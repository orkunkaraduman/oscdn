package store

import (
	"errors"
	"fmt"
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

func (e *RequestError) Error() string {
	return fmt.Errorf("request error: %w", e.error).Error()
}

func (e *RequestError) Unwrap() error {
	return e.error
}

type SizeExceededError struct {
	Size int64
}

func (e *SizeExceededError) Error() string {
	return fmt.Errorf("size exceeded %d", e.Size).Error()
}

type DynamicContentError struct {
	resp *http.Response
}

func (e *DynamicContentError) Error() string {
	return fmt.Errorf("dynamic content").Error()
}
