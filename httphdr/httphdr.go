package httphdr

import (
	"bytes"
	"net/http"
	"time"

	"gitlab.com/orkunkaraduman/narvi/httphdr"
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

func SizeOfHTTPHeader(header http.Header) int {
	buf := bytes.NewBuffer(nil)
	_ = header.Write(buf)
	return buf.Len()
}

func Expires(header http.Header, now time.Time) (expires time.Time) {
	hasCacheControl := false
	if h := header.Get("Cache-Control"); h != "" {
		hasCacheControl = true
		cc := httphdr.ParseCacheControl(h)
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
