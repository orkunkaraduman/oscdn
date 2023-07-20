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
type DynamicContentError struct {
	error
	resp *http.Response
}
