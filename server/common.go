package server

import (
	"crypto/tls"

	"github.com/goinsane/logng"

	"github.com/orkunkaraduman/oscdn/store"
)

type Config struct {
	Logger        *logng.Logger
	Listen        string
	ListenBacklog int
	TLSConfig     *tls.Config
	Store         *store.Store
	GetOrigin     func(scheme, host string) *Origin
}

type Origin struct {
	Scheme        string
	Host          string
	HostOverride  bool
	HttpsRedirect bool
	UploadBurst   int64
	UploadRate    int64
}
