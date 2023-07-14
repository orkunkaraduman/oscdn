package store

import (
	"crypto/tls"
	"time"
)

type Config struct {
	Path          string
	MaxAge        time.Duration
	TLSConfig     *tls.Config
	UserAgent     string
	MaxIdleConns  int
	DownloadBurst int64
	DownloadRate  int64
}
