package httphdr

import (
	"bytes"
	"net/http"
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
