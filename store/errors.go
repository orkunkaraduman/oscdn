package store

import (
	"errors"
	"net/http"
)

var (
	ErrReleased  = errors.New("store released")
	ErrNotExists = errors.New("not exists")
)

type RequestError struct {
	error
}
type SizeExceededError struct {
	error
	Size int64
}
type DynamicContentError struct {
	error
	resp *http.Response
}
