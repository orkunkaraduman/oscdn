package server

import "github.com/orkunkaraduman/oscdn/store"

type Config struct {
	HttpListen  string
	HttpsListen string
	Store       *store.Store
}
