package store

import (
	"crypto/tls"
	"time"

	"github.com/goinsane/logng"
)

type Config struct {
	Logger        *logng.Logger
	Path          string
	MaxSize       int64
	MaxAge        time.Duration
	DefAge        time.Duration
	TLSConfig     *tls.Config
	MaxIdleConns  int
	UserAgent     string
	DownloadBurst int64
	DownloadRate  int64
}
