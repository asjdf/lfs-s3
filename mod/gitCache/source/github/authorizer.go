package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/jellydator/ttlcache/v2"
)

type CacheableAuthorizer struct {
	cache        *ttlcache.Cache // Set cache to nil to disable caching
	statsdClient *statsd.Client
}

type CacheMetrics struct {
	Keys    int64 // the number of keys currently in the cache
	Hits    int64 // the total number of cache hits
	Misses  int64 // the total number of cache misses
	Inserts int64 // the total number of keys ever inserted into the cache
	Removes int64 // the total number of keys ever removed from the cache, either expired or evicted
}

func NewAuthorizer(enableCache bool, statsdClient *statsd.Client) CacheableAuthorizer {
	if enableCache {
		cache := ttlcache.NewCache()
		_ = cache.SetTTL(15 * time.Minute)
		cache.SkipTTLExtensionOnHit(true) // set this to true so that TTL won't get extended on cache hit
		cache.SetCacheSizeLimit(1000 * 1000)
		return CacheableAuthorizer{
			cache:        cache,
			statsdClient: statsdClient,
		}
	}
	return CacheableAuthorizer{
		cache:        nil,
		statsdClient: statsdClient,
	}
}

func (authorizer CacheableAuthorizer) Close() {
	if authorizer.cache != nil {
		authorizer.cache.Close()
	}
}

func (authorizer CacheableAuthorizer) RequestAuthorizer(req *http.Request) error {
	username, token, ok := req.BasicAuth()
	if !ok {
		// Reject any requests without an "Authorization" header.
		return grpcstatus.Error(grpccodes.Unauthenticated, "request not authenticated")
	}

	// The HTTP basic-auth username expected for any authenticated request issued by a Git client.
	// https://docs.github.com/en/developers/apps/authenticating-with-github-apps#http-based-git-access-by-an-installation
	//if username != "x-access-token" {
	//	// Reject any requests with unexpected username values.
	//	_ = authorizer.statsdClient.Incr("goblet.authentication.failed", []string{"reason:wrong_username"}, 1)
	//	return grpcstatus.Error(grpccodes.InvalidArgument, "malformed request")
	//}

	pathParts := strings.Split(strings.TrimPrefix(req.URL.Path, "/"), "/")
	if len(pathParts) < 2 {
		// All GitHub repository URLs will have, at least, two parts. Thus, reject
		// any request that does not adhere to the expected format.
		_ = authorizer.statsdClient.Incr("goblet.authentication.failed", []string{"reason:malformed_url"}, 1)
		return grpcstatus.Error(grpccodes.InvalidArgument, "malformed request")
	}

	repoOwner, repoName := url.PathEscape(pathParts[0]), url.PathEscape(pathParts[1])
	repoURL := fmt.Sprintf("https://github.com/%s/%s", repoOwner, repoName)

	authorized, err := authorizer.isAuthorized(username, token, repoURL)
	if !authorized {
		if err != nil {
			log.Printf("Authentication error: %v\n", err)
		}
		_ = authorizer.statsdClient.Incr("goblet.authentication.failed", []string{"reason:access_denied"}, 1)
		return grpcstatus.Error(grpccodes.PermissionDenied, "access denied")
	}

	// Ensures that the authorization token isn't leaked further down the chain after
	// the request has been successfully validated.
	req.Header.Del("Authorization")

	return nil
}

func (authorizer CacheableAuthorizer) isAuthorized(username, token string, repoURL string) (bool, error) {
	if authorizer.cache == nil {
		authorized, _, err := isTokenValid(username, token, repoURL)
		_ = authorizer.statsdClient.Incr("goblet.operation.count", []string{"op:token_validation"}, 1)
		return authorized, err
	}

	cacheKey := fmt.Sprintf("%s:%s@%s", username, token, repoURL)
	if authorized, err := authorizer.cache.Get(cacheKey); err == nil {
		// return the cached result. We will lose the original error if any
		return authorized.(bool), nil
	}

	authorized, shouldCache, err := isTokenValid(username, token, repoURL)
	_ = authorizer.statsdClient.Incr("goblet.operation.count", []string{"op:token_validation"}, 1)
	if shouldCache {
		_ = authorizer.cache.Set(cacheKey, authorized)
	}

	return authorized, err
}

// This function queries Github to validate if the `token` has access to the `repoURL`.
// It returns 3 values:
// 1. authorized: whether the token is valid
// 2. shouldCache: whether the result should be cached
// 3. err: Associated error if the token is not valid
func isTokenValid(username, token string, repoURL string) (authorized bool, shouldCache bool, err error) {
	infoRefsURL := fmt.Sprintf("%s/info/refs?service=git-upload-pack", repoURL)

	log.Printf("Validating token against %s\n", infoRefsURL)

	req, err := http.NewRequest(http.MethodGet, infoRefsURL, nil)
	if err != nil {
		return false, true, err
	}

	req.Header.Add("Git-Protocol", "version=2")
	req.SetBasicAuth(username, token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, true, err
	}
	defer res.Body.Close()

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return false, true, err
	}

	if res.StatusCode != http.StatusOK {
		err = errors.New(string(resBytes))

		// should not cache result if statusCode matches these, so next authorization attempt will retry
		if res.StatusCode >= 500 && res.StatusCode < 600 && res.StatusCode != 501 {
			return false, false, err
		}

		return false, true, err
	}

	return true, true, nil
}

func (authorizer CacheableAuthorizer) CacheMetrics() CacheMetrics {
	var metrics CacheMetrics
	if authorizer.cache != nil {
		internalMetrics := authorizer.cache.GetMetrics()
		// the keys in cache.GetMetrics() are a bit misleading, so we will map them to our own keys
		metrics.Keys = int64(authorizer.cache.Count())
		metrics.Hits = internalMetrics.Retrievals
		metrics.Misses = internalMetrics.Misses
		metrics.Inserts = internalMetrics.Inserted
		metrics.Removes = internalMetrics.Evicted
	}
	return metrics
}

func (authorizer CacheableAuthorizer) CacheMetricsHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if b, err := json.Marshal(authorizer.CacheMetrics()); err == nil {
		_, _ = w.Write(b)
	}
}
