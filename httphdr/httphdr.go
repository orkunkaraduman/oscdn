package httphdr

import (
	"net/http"
	"time"
)

func PutHTTPHeaders(dst http.Header, src http.Header, keys ...string) {
	src = src.Clone()
	if len(keys) <= 0 {
		for key := range src {
			dst[key] = src[key]
		}
		return
	}
	for _, key := range keys {
		dst[key] = src[key]
	}
}

func Expires(header http.Header, now time.Time) (expires time.Time) {
	hasCacheControl := false
	if h := header.Get("Cache-Control"); h != "" {
		hasCacheControl = true
		cc := ParseCacheControl(h)
		maxAge := cc.MaxAge()
		if maxAge < 0 {
			maxAge = cc.SMaxAge()
		}
		switch {
		case cc.NoCache():
		case maxAge >= 0:
			expires = now.Add(maxAge)
		}
	}

	if h := header.Get("Expires"); h != "" && !hasCacheControl {
		expires, _ = time.Parse(time.RFC1123, h)
	}

	return
}
