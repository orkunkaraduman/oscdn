package store

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/goinsane/logng"
)

var (
	hostRgx = regexp.MustCompile(`^([A-Za-z0-9-]{1,63}\.)*([A-Za-z0-9-]{1,63})(:[0-9]{1,5})?$`)
)

type Config struct {
	Logger            *logng.Logger
	Path              string
	TLSConfig         *tls.Config
	MaxIdleConns      int
	UserAgent         string
	DefaultHostConfig *HostConfig
	GetHostConfig     func(scheme, host string) *HostConfig
}

type HostConfig struct {
	MaxSize       int64
	MaxAge        time.Duration
	DownloadBurst int64
	DownloadRate  int64
}

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
	BaseURL      *url.URL
	KeyURL       *url.URL
	CacheStatus  CacheStatus
	StatusCode   int
	Header       http.Header
	CreatedAt    time.Time
	ExpiresAt    time.Time
	Size         int64
	ContentRange *ContentRange
}
