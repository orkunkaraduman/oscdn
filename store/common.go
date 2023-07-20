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

func (s CacheStatus) String() string {
	switch s {
	case CacheStatusUnspecified:
		return "UNSPECIFIED"
	case CacheStatusHit:
		return "HIT"
	case CacheStatusMiss:
		return "MISS"
	case CacheStatusExpired:
		return "EXPIRED"
	case CacheStatusUpdating:
		return "UPDATING"
	case CacheStatusStale:
		return "STALE"
	case CacheStatusDynamic:
		return "DYNAMIC"
	}
	return "UNKNOWN"
}

const (
	CacheStatusUnspecified CacheStatus = iota
	CacheStatusHit
	CacheStatusMiss
	CacheStatusExpired
	CacheStatusUpdating
	CacheStatusStale
	CacheStatusDynamic
)

type ContentRange struct {
	Start int64
	End   int64
}

type GetResult struct {
	io.ReadCloser
	BaseURL     *url.URL
	KeyURL      *url.URL
	CacheStatus CacheStatus
	StatusCode  int
	Header      http.Header
}
