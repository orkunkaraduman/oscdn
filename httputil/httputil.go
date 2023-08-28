package httputil

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
