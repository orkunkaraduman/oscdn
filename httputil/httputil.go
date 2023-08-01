package httputil

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func SplitHostPort(host string) (domain string, port int, err error) {
	domain = host
	if idx := strings.LastIndex(host, ":"); idx >= 0 {
		domain = host[:idx]
		sPort := host[idx+1:]
		var uPort uint64
		uPort, err = strconv.ParseUint(sPort, 10, 16)
		if err != nil {
			err = fmt.Errorf("unable to parse port %q: %w", sPort, err)
			return
		}
		port = int(uPort)
	}
	return
}

func Expires(header http.Header, now time.Time) (expires time.Time) {
	if !now.After(*new(time.Time)) {
		panic("invalid now")
	}

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
		if expires.Before(now) {
			expires = now
		}
	}

	return
}
