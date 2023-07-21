package store

import (
	"errors"
	"net/http"
)

var (
	ErrNotExists = errors.New("not exists")
)
var (
	ErrStoreReleased = errors.New("store released")
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
