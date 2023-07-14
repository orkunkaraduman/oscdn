package store

import "net/http"

type _Download struct {
	Header        http.Header
	ContentLength int64
	Done          chan struct{}
}
