package store

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
)

var (
	DomainRgx = regexp.MustCompile(`^([A-Za-z0-9-]{1,63}\.)+[A-Za-z]{2,6}$`)
	HostRgx   = regexp.MustCompile(`^([A-Za-z0-9-]{1,63}\.)+[A-Za-z]{2,6}(:[0-9]{1,5})?$`)
)

type CacheStatus int

const (
	CacheStatusUnknown CacheStatus = iota
	CacheStatusHit
	CacheStatusMiss
	CacheStatusExpired
	CacheStatusUpdating
	CacheStatusStale
)

type GetResult struct {
	io.ReadCloser
	BaseURL     *url.URL
	KeyURL      *url.URL
	CacheStatus CacheStatus
	StatusCode  int
	Header      http.Header
}
