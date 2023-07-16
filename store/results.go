package store

import (
	"io"
	"net/http"
	"net/url"
)

type GetResult struct {
	io.ReadCloser
	BaseURL    *url.URL
	KeyURL     *url.URL
	StatusCode int
	Header     http.Header
}
