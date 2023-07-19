package store

import "errors"

var (
	ErrReleased       = errors.New("store released")
	ErrDynamicContent = errors.New("dynamic content")
	ErrDownloadError  = errors.New("download error")
	ErrNotExists      = errors.New("not exists")
)
