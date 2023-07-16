package store

import (
	"crypto/tls"
	"time"

	"github.com/goinsane/logng"
)

type Config struct {
	Logger        *logng.Logger
	Path          string
	MaxAge        time.Duration
	TLSConfig     *tls.Config
	MaxIdleConns  int
	UserAgent     string
	DownloadBurst int64
	DownloadRate  int64
}
