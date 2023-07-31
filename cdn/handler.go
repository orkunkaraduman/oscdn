package cdn

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goinsane/logng"
	"github.com/goinsane/xcontext"

	"github.com/orkunkaraduman/oscdn/ioutil"
	"github.com/orkunkaraduman/oscdn/store"
)

type Handler struct {
	Logger    *logng.Logger
	Context   context.Context
	Store     *store.Store
	GetOrigin func(scheme, host string) *Origin

	wg sync.WaitGroup
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var err error

	defer func() {
		if p := recover(); p != nil {
			go func() {
				panic(p)
			}()
		}
	}()

	h.wg.Add(1)
	defer h.wg.Done()

	ctx := h.Context
	if ctx == nil {
		ctx = context.Background()
	}

	logger := h.Logger.WithFieldKeyVals("scheme", req.URL.Scheme,
		"host", req.URL.Host, "requestURI", req.RequestURI, "remoteAddr", req.RemoteAddr)
	ctx = context.WithValue(ctx, "logger", logger)

	err = ctx.Err()
	if err != nil {
		logger.Error(err)
		return
	}

	switch req.URL.Scheme {
	case "http":
		req.URL.Host = strings.TrimSuffix(req.URL.Host, ":80")
	case "https":
		req.URL.Host = strings.TrimSuffix(req.URL.Host, ":443")
	default:
		err = fmt.Errorf("unknown scheme %s", req.URL.Scheme)
		logger.V(2).Error(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	logger = logger.WithFieldKeyVals("normalizedHost", req.URL.Host)
	ctx = context.WithValue(context.Background(), "logger", logger)

	switch req.Method {
	case http.MethodHead:
	case http.MethodGet:
	default:
		err = fmt.Errorf("method %s not allowed", req.Method)
		logger.V(2).Error(err)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	contentRange, err := getContentRange(req.Header)
	if err != nil {
		logger.V(2).Error(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var origin *Origin
	if h.GetOrigin != nil {
		origin = h.GetOrigin(req.URL.Scheme, req.URL.Host)
		if origin == nil {
			err = errors.New("unknown origin")
			logger.V(2).Error(err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
	}

	rawURL := req.URL.String()
	host := ""

	if origin != nil {
		rawURL = (&url.URL{
			Scheme:   origin.Scheme,
			Host:     origin.Host,
			Path:     req.URL.Path,
			RawQuery: req.URL.RawQuery,
		}).String()
		if origin.HostOverride {
			host = req.URL.Host
		}
	}

	getResult, err := h.Store.Get(ctx, rawURL, host, contentRange)
	if err != nil {
		if xcontext.IsContextError(err) {
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}
		switch err.(type) {
		case *store.RequestError:
			w.WriteHeader(http.StatusBadGateway)
		case *store.DynamicContentError:
			w.WriteHeader(http.StatusBadGateway)
		case *store.SizeExceededError:
			w.WriteHeader(http.StatusBadGateway)
		}
		return
	}

	w.Header().Set("X-Cache-Status", getResult.CacheStatus.String())
	for _, key := range []string{
		"Content-Type",
		"Date",
		"Etag",
		"Last-Modified",
	} {
		for _, val := range getResult.Header.Values(key) {
			w.Header().Add(key, val)
		}
	}
	w.Header().Set("Expires", getResult.ExpiresAt.Format(time.RFC1123))
	if getResult.StatusCode != http.StatusOK || getResult.ContentRange == nil {
		w.Header().Set("Content-Length", strconv.FormatInt(getResult.Size, 10))
		w.WriteHeader(getResult.StatusCode)
	} else {
		contentLength := getResult.ContentRange.End - getResult.ContentRange.Start
		w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d",
			getResult.ContentRange.Start, getResult.ContentRange.End, contentLength))
		w.WriteHeader(http.StatusPartialContent)
	}

	switch req.Method {
	case http.MethodHead:
		err = nil
	case http.MethodGet:
		var uploadBurst int64
		var uploadRate int64
		if origin != nil {
			uploadBurst = origin.UploadBurst
			uploadRate = origin.UploadRate
		}
		_, err = ioutil.CopyRate(w, getResult, uploadBurst, uploadRate)
	}
	if err != nil {
		err = fmt.Errorf("content upload error: %w", err)
		logger.V(2).Error(err)
		return
	}
}

func (h *Handler) Wait() {
	h.wg.Wait()
}
