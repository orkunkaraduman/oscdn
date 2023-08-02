package cdn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goinsane/logng"
	"github.com/goinsane/xcontext"

	"github.com/orkunkaraduman/oscdn/httputil"
	"github.com/orkunkaraduman/oscdn/ioutil"
	"github.com/orkunkaraduman/oscdn/store"
)

type Handler struct {
	Logger        *logng.Logger
	Context       context.Context
	Store         *store.Store
	GetHostConfig func(scheme, host string) *HostConfig

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

	if req.TLS == nil {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimSuffix(req.Host, ":80")
	} else {
		req.URL.Scheme = "https"
		req.URL.Host = strings.TrimSuffix(req.Host, ":443")
	}

	logger := h.Logger.WithFieldKeyVals("scheme", req.URL.Scheme, "host", req.URL.Host,
		"requestURI", req.RequestURI, "remoteAddr", req.RemoteAddr)
	ctx = context.WithValue(ctx, "logger", logger)

	err = ctx.Err()
	if err != nil {
		logger.V(1).Error(err)
		return
	}

	if (req.URL.Scheme != "http" && req.URL.Scheme != "https") ||
		req.URL.Opaque != "" ||
		req.URL.User != nil ||
		req.URL.Host == "" ||
		req.URL.Fragment != "" {
		err = errors.New("invalid cdn url")
		logger.V(1).Error(err)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.Copy(w, strings.NewReader(BodyInvalidCdnUrl))
		return
	}

	switch req.Method {
	case http.MethodHead:
	case http.MethodGet:
	default:
		err = fmt.Errorf("method %s not allowed", req.Method)
		logger.V(1).Error(err)
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.Copy(w, strings.NewReader(BodyMethodNotAllowed))
		return
	}

	contentRange, err := getContentRange(req.Header)
	if err != nil {
		logger.V(1).Error(err)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.Copy(w, strings.NewReader(BodyInvalidContentRange))
		return
	}

	var hostConfig *HostConfig
	if h.GetHostConfig != nil {
		hostConfig = h.GetHostConfig(req.URL.Scheme, req.URL.Host)
		if hostConfig == nil {
			err = errors.New("not allowed host")
			logger.V(1).Error(err)
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.Copy(w, strings.NewReader(BodyNotAllowedHost))
			return
		}
	}

	_url := &url.URL{
		Scheme:   req.URL.Scheme,
		Host:     req.URL.Host,
		Path:     req.URL.Path,
		RawQuery: req.URL.RawQuery,
	}
	host := ""

	if hostConfig != nil {
		domain, _, _ := httputil.SplitHostPort(req.URL.Host)
		_, originPort, _ := httputil.SplitHostPort(hostConfig.Origin.Host)

		if req.URL.Scheme == "http" && hostConfig.HttpsRedirect {
			_url.Scheme = "https"
			_url.Host = domain
			if hostConfig.HttpsRedirectPort > 0 && hostConfig.HttpsRedirectPort != 443 {
				_url.Host = fmt.Sprintf("%s:%d", domain, hostConfig.HttpsRedirectPort)
			}
			w.Header().Set("Location", _url.String())
			w.WriteHeader(http.StatusFound)
			_, _ = io.Copy(w, strings.NewReader(BodyHttpsRedirect))
			return
		}

		_url.Scheme = hostConfig.Origin.Scheme
		_url.Host = hostConfig.Origin.Host

		if hostConfig.DomainOverride {
			host = domain
			if originPort > 0 {
				host = fmt.Sprintf("%s:%d", domain, originPort)
			}
		}

		if hostConfig.IgnoreQuery {
			_url.RawQuery = ""
		}
	}

	getResult, err := h.Store.Get(ctx, _url.String(), host, contentRange)
	if err != nil {
		if xcontext.IsContextError(err) {
			logger.V(1).Error(err)
			return
		}
		switch err.(type) {
		case *store.RequestError:
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.Copy(w, strings.NewReader(BodyOriginNotResponding))
		case *store.DynamicContentError:
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.Copy(w, strings.NewReader(BodyDynamicContent))
		case *store.SizeExceededError:
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.Copy(w, strings.NewReader(BodyContentSizeExceeded))
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.Copy(w, strings.NewReader(BodyInternalServerError))
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
		if hostConfig != nil {
			uploadBurst = hostConfig.UploadBurst
			uploadRate = hostConfig.UploadRate
		}
		_, err = ioutil.CopyRate(w, getResult, uploadBurst, uploadRate)
	}
	if err != nil {
		err = fmt.Errorf("content upload error: %w", err)
		logger.V(1).Error(err)
		return
	}
}

func (h *Handler) Wait() {
	h.wg.Wait()
}
