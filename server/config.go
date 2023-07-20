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
}
