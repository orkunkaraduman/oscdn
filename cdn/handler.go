package cdn

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/goinsane/logng"
	"github.com/goinsane/xcontext"

	"github.com/orkunkaraduman/oscdn/httputil"
	"github.com/orkunkaraduman/oscdn/ioutil"
	"github.com/orkunkaraduman/oscdn/store"
)

type Handler struct {
	Logger        *logng.Logger
	Store         *store.Store
	ServerHeader  string
	GetHostConfig func(scheme, host string) *HostConfig
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var err error

	ctx := context.Background()

	if req.TLS == nil {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimSuffix(req.Host, ":80")
	} else {
		req.URL.Scheme = "https"
		req.URL.Host = strings.TrimSuffix(req.Host, ":443")
	}

	domain, _, _ := httputil.SplitHostPort(req.URL.Host)
	remoteIP, _, _ := httputil.SplitHostPort(req.RemoteAddr)

	logger := h.Logger.WithFieldKeyVals("requestScheme", req.URL.Scheme, "requestHost", req.URL.Host,
		"requestURI", req.RequestURI, "remoteAddr", req.RemoteAddr, "remoteIP", remoteIP)
	ctx = context.WithValue(ctx, "logger", logger)

	err = ctx.Err()
	if err != nil {
		logger.V(1).Error(err)
		return
	}

	w.Header().Set("Server", "oscdn")
	if h.ServerHeader != "" {
		w.Header().Set("Server", h.ServerHeader)
	}

	if (req.URL.Scheme != "http" && req.URL.Scheme != "https") ||
		req.URL.Opaque != "" ||
		req.URL.User != nil ||
		req.URL.Host == "" ||
		req.URL.Fragment != "" {
		err = errors.New("invalid cdn url")
		logger.V(1).Error(err)
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}

	switch req.Method {
	case http.MethodHead:
	case http.MethodGet:
	default:
		err = fmt.Errorf("method %s not allowed", req.Method)
		logger.V(1).Error(err)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	contentRange, err := getContentRange(req.Header)
	if err != nil {
		logger.V(1).Error(err)
		http.Error(w, "invalid content range", http.StatusBadRequest)
		return
	}

	var hostConfig *HostConfig
	if h.GetHostConfig != nil {
		hostConfig = h.GetHostConfig(req.URL.Scheme, req.URL.Host)
		if hostConfig == nil {
			err = errors.New("not allowed host")
			logger.V(1).Error(err)
			http.Error(w, "not allowed host", http.StatusForbidden)
			return
		}
	}

	storeURL := &url.URL{
		Scheme:   req.URL.Scheme,
		Host:     req.URL.Host,
		Path:     req.URL.Path,
		RawQuery: req.URL.RawQuery,
	}
	storeHost := ""

	if hostConfig != nil {
		_, originPort, _ := httputil.SplitHostPort(hostConfig.Origin.Host)

		if req.URL.Scheme == "http" && hostConfig.HttpsRedirect {
			storeURL.Scheme = "https"
			storeURL.Host = domain
			if hostConfig.HttpsRedirectPort > 0 && hostConfig.HttpsRedirectPort != 443 {
				storeURL.Host = fmt.Sprintf("%s:%d", domain, hostConfig.HttpsRedirectPort)
			}
			w.Header().Set("Location", storeURL.String())
			http.Error(w, http.StatusText(http.StatusFound), http.StatusFound)
			return
		}

		storeURL.Scheme = hostConfig.Origin.Scheme
		storeURL.Host = hostConfig.Origin.Host

		if hostConfig.DomainOverride {
			storeHost = domain
			if originPort > 0 {
				storeHost = fmt.Sprintf("%s:%d", domain, originPort)
			}
		}

		if hostConfig.IgnoreQuery {
			storeURL.RawQuery = ""
		}
	}

	getResult, err := h.Store.Get(ctx, storeURL.String(), storeHost, contentRange)
	if err != nil {
		if xcontext.IsContextError(err) {
			logger.V(1).Error(err)
			return
		}
		switch err.(type) {
		case *store.RequestError:
			http.Error(w, "origin not responding", http.StatusBadGateway)
		case *store.DynamicContentError:
			http.Error(w, "dynamic content", http.StatusBadGateway)
		case *store.SizeExceededError:
			http.Error(w, "content size exceeded", http.StatusBadGateway)
		default:
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}
	defer func(getResult store.GetResult) {
		_ = getResult.Close()
	}(getResult)

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
