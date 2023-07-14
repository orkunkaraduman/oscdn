package store

import "crypto/tls"

type Config struct {
	Path            string
	TLSConfig       *tls.Config
	DownloadMaxIdle int
	DownloadBurst   int64
	DownloadRate    int64
}
