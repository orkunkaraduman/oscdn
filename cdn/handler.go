package cdn

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/goinsane/logng"

	"github.com/orkunkaraduman/oscdn/httputil"
	"github.com/orkunkaraduman/oscdn/ioutil"
	"github.com/orkunkaraduman/oscdn/store"
)

type Handler struct {
	Store         *store.Store
	ServerHeader  string
	GetHostConfig func(scheme, host string) *HostConfig
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var err error

	ctx := req.Context()
	logger, _ := ctx.Value("logger").(*logng.Logger)

	if req.TLS == nil {
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimSuffix(req.Host, ":80")
	} else {
		req.URL.Scheme = "https"
		req.URL.Host = strings.TrimSuffix(req.Host, ":443")
	}

	remoteIP, _, _ := httputil.SplitHost(req.RemoteAddr)
	realIP := httputil.GetRealIP(req)

	logger = logger.WithFieldKeyVals("requestScheme", req.URL.Scheme, "requestHost", req.URL.Host,
		"requestMethod", req.Method, "requestURI", req.RequestURI, "remoteAddr", req.RemoteAddr, "remoteIP", remoteIP,
		"realIP", realIP)
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
	w.Header().Set("Accept-Ranges", "bytes")

	writer := &_Writer{
		ResponseWriter: w,
		Request:        req,
		HostConfig:     h.GetHostConfig(req.URL.Scheme, req.URL.Host),
	}

	if !writer.Prepare(ctx) {
		return
	}

	getResult, err := h.Store.Get(ctx, writer.StoreURL.String(), writer.StoreHost, writer.ContentRange)
	if err != nil {
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

	if getResult.ContentRange == nil {
		if getResult.Size <= writer.HostConfig.CompressionMaxSize {
			writer.SetContentEncoder(ctx)
		}
		if writer.ContentEncoding == "" {
			w.Header().Set("Content-Length", strconv.FormatInt(getResult.Size, 10))
		}
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
		_, err = ioutil.CopyRate(writer, getResult, writer.HostConfig.UploadBurst, writer.HostConfig.UploadRate)
		if err == nil {
			err = writer.Close()
		}
	}
	if err != nil {
		err = fmt.Errorf("content upload error: %w", err)
		logger.V(1).Error(err)
		return
	}
}
