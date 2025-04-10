package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jellydator/ttlcache/v2"
)

type Authorizer struct {
	cache *ttlcache.Cache
}

type CacheMetrics struct {
	Keys    int64
	Hits    int64
	Misses  int64
	Inserts int64
	Removes int64
}

func NewAuthorizer(enableCache bool) *Authorizer {
	if !enableCache {
		return &Authorizer{cache: nil}
	}

	cache := ttlcache.NewCache()
	_ = cache.SetTTL(15 * time.Minute)
	cache.SkipTTLExtensionOnHit(true)
	cache.SetCacheSizeLimit(1000 * 1000)
	return &Authorizer{cache: cache}
}

func (a *Authorizer) Close() {
	if a.cache != nil {
		a.cache.Close()
	}
}

func (a *Authorizer) RequestAuthorizer(req *http.Request) error {
	username, token, ok := req.BasicAuth()
	if !ok {
		return errors.New("request not authenticated")
	}

	// 从请求路径中获取仓库信息
	pathParts := strings.Split(strings.TrimPrefix(req.URL.Path, "/"), "/")
	if len(pathParts) < 2 {
		return errors.New("malformed request")
	}

	repoOwner, repoName := pathParts[0], pathParts[1]
	repoURL := fmt.Sprintf("https://github.com/%s/%s", repoOwner, repoName)

	authorized, err := a.isAuthorized(username, token, repoURL)
	if !authorized {
		if err != nil {
			return fmt.Errorf("authentication error: %v", err)
		}
		return errors.New("access denied")
	}

	// 验证成功后删除认证头，防止泄露
	req.Header.Del("Authorization")

	return nil
}

func (a *Authorizer) isAuthorized(username, token string, repoURL string) (bool, error) {
	if a.cache == nil {
		authorized, _, err := isTokenValid(username, token, repoURL)
		return authorized, err
	}

	cacheKey := fmt.Sprintf("%s:%s@%s", username, token, repoURL)
	if authorized, err := a.cache.Get(cacheKey); err == nil {
		return authorized.(bool), nil
	}

	authorized, shouldCache, err := isTokenValid(username, token, repoURL)
	if shouldCache {
		_ = a.cache.Set(cacheKey, authorized)
	}

	return authorized, err
}

func isTokenValid(username, token string, repoURL string) (authorized bool, shouldCache bool, err error) {
	infoRefsURL := fmt.Sprintf("%s/info/refs?service=git-upload-pack", repoURL)

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

		// 对于服务器错误，不缓存结果以便下次重试
		if res.StatusCode >= 500 && res.StatusCode < 600 && res.StatusCode != 501 {
			return false, false, err
		}

		return false, true, err
	}

	return true, true, nil
}

func (a *Authorizer) CacheMetrics() CacheMetrics {
	var metrics CacheMetrics
	if a.cache != nil {
		internalMetrics := a.cache.GetMetrics()
		metrics.Keys = int64(a.cache.Count())
		metrics.Hits = internalMetrics.Retrievals
		metrics.Misses = internalMetrics.Misses
		metrics.Inserts = internalMetrics.Inserted
		metrics.Removes = internalMetrics.Evicted
	}
	return metrics
}

func (a *Authorizer) CacheMetricsHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if b, err := json.Marshal(a.CacheMetrics()); err == nil {
		_, _ = w.Write(b)
	}
}
