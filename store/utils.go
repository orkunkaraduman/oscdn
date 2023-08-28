package store

import (
	"net/http"
	"time"

	"github.com/orkunkaraduman/oscdn/httputil"
)

func httpExpires(header http.Header, now time.Time) (expires time.Time) {
	if !now.After(*new(time.Time)) {
		panic("invalid now")
	}

	hasCacheControl := false
	if h := header.Get("Cache-Control"); h != "" {
		hasCacheControl = true
		cc := httputil.ParseCacheControl(h)
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
		if expires.Before(now) {
			expires = now
		}
	}

	return
}
