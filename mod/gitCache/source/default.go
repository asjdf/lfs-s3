package source

import (
	"net/url"
	"strings"
)

// DefaultURLCanonicalizer is a URLCanonicalizer implementation that agnostic to any Git hosting provider.
func DefaultURLCanonicalizer(u *url.URL) (*url.URL, error) {
	ret := url.URL{}
	ret.Scheme = "https"
	ret.Host = strings.ToLower(u.Host)
	ret.Path = u.Path

	// Git endpoint suffixes.
	if strings.HasSuffix(ret.Path, "/info/refs") {
		ret.Path = strings.TrimSuffix(ret.Path, "/info/refs")
	} else if strings.HasSuffix(ret.Path, "/git-upload-pack") {
		ret.Path = strings.TrimSuffix(ret.Path, "/git-upload-pack")
	} else if strings.HasSuffix(ret.Path, "/git-receive-pack") {
		ret.Path = strings.TrimSuffix(ret.Path, "/git-receive-pack")
	}
	ret.Path = strings.TrimSuffix(ret.Path, ".git")
	return &ret, nil
}
