package store

import "errors"

var (
	ErrReleased       = errors.New("store released")
	ErrDynamicContent = errors.New("dynamic content")
)
