package httputil

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func SplitHost(addr string) (host string, port int, err error) {
	host = addr
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		host = addr[:idx]
		sPort := addr[idx+1:]
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

func GetRealIP(req *http.Request) string {
	var v string

	v = req.Header.Get("X-Real-IP")
	if v != "" {
		return v
	}

	v = strings.TrimSpace(strings.Split(req.Header.Get("X-Forwarded-For"), ",")[0])
	if v != "" {
		return v
	}

	host, _, _ := SplitHost(req.Host)

	return host
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
