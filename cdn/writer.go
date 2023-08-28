package cdn

import (
	"compress/flate"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/goinsane/logng"

	"github.com/orkunkaraduman/oscdn/httputil"
	"github.com/orkunkaraduman/oscdn/ioutil"
	"github.com/orkunkaraduman/oscdn/store"
)

type _Writer struct {
	io.WriteCloser
	http.ResponseWriter
	Request         *http.Request
	HostConfig      *HostConfig
	StoreURL        *url.URL
	StoreHost       string
	ContentRange    *store.ContentRange
	ContentEncoding string
}

func (w *_Writer) Write(p []byte) (n int, err error) {
	return w.WriteCloser.Write(p)
}

func (w *_Writer) Prepare(ctx context.Context) bool {
	w.WriteCloser = ioutil.NopWriteCloser(w.ResponseWriter)
	if !w.validate(ctx) {
		return false
	}
	if !w.setStoreURL(ctx) {
		return false
	}
	if !w.setContentRange(ctx) {
		return false
	}
	return true
}

func (w *_Writer) validate(ctx context.Context) bool {
	var err error

	logger, _ := ctx.Value("logger").(*logng.Logger)

	if (w.Request.URL.Scheme != "http" && w.Request.URL.Scheme != "https") ||
		w.Request.URL.Opaque != "" ||
		w.Request.URL.User != nil ||
		w.Request.URL.Host == "" ||
		w.Request.URL.Fragment != "" {
		err = errors.New("invalid url")
		logger.V(1).Error(err)
		http.Error(w, "invalid url", http.StatusBadRequest)
		return false
	}

	switch w.Request.Method {
	case http.MethodHead:
	case http.MethodGet:
	default:
		err = fmt.Errorf("method %s not allowed", w.Request.Method)
		logger.V(1).Error(err)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return false
	}

	if w.HostConfig == nil {
		err = errors.New("not allowed host")
		logger.V(1).Error(err)
		http.Error(w, "not allowed host", http.StatusForbidden)
		return false
	}

	return true
}

func (w *_Writer) setStoreURL(ctx context.Context) bool {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	w.StoreURL = &url.URL{
		Scheme:   w.Request.URL.Scheme,
		Host:     w.Request.URL.Host,
		Path:     w.Request.URL.Path,
		RawQuery: w.Request.URL.RawQuery,
	}

	host, _, _ := httputil.SplitHost(w.Request.URL.Host)
	_, originPort, _ := httputil.SplitHost(w.HostConfig.Origin.Host)

	if w.Request.URL.Scheme == "http" && w.HostConfig.HttpsRedirect {
		w.StoreURL.Scheme = "https"
		w.StoreURL.Host = host
		if w.HostConfig.HttpsRedirectPort > 0 && w.HostConfig.HttpsRedirectPort != 443 {
			w.StoreURL.Host = fmt.Sprintf("%s:%d", host, w.HostConfig.HttpsRedirectPort)
		}
		logger.V(1).Info("redirecting to https")
		w.Header().Set("Location", w.StoreURL.String())
		http.Error(w, http.StatusText(http.StatusFound), http.StatusFound)
		return false
	}

	w.StoreURL.Scheme = w.HostConfig.Origin.Scheme
	w.StoreURL.Host = w.HostConfig.Origin.Host

	if w.HostConfig.HostOverride {
		w.StoreHost = host
		if originPort > 0 {
			w.StoreHost = fmt.Sprintf("%s:%d", host, originPort)
		}
	}

	if w.HostConfig.IgnoreQuery {
		w.StoreURL.RawQuery = ""
	}

	return true
}

func (w *_Writer) setContentRange(ctx context.Context) bool {
	var err error

	logger, _ := ctx.Value("logger").(*logng.Logger)

	opts := httputil.ParseOptions(w.Request.Header.Get("Range"))
	if len(opts) <= 0 {
		return true
	}
	b := opts[0].Map["bytes"]
	if b == "" {
		return true
	}
	ranges := strings.SplitN(b, "-", 2)
	w.ContentRange = &store.ContentRange{
		Start: 0,
		End:   -1,
	}
	if len(ranges) > 0 && ranges[0] != "" {
		w.ContentRange.Start, err = strconv.ParseInt(ranges[0], 10, 64)
		if err != nil {
			err = fmt.Errorf("unable to parse content range start: %w", err)
			logger.V(1).Error(err)
			http.Error(w, "invalid content range", http.StatusBadRequest)
			return false
		}
	}
	if len(ranges) > 1 && ranges[1] != "" {
		w.ContentRange.End, err = strconv.ParseInt(ranges[1], 10, 64)
		if err != nil {
			err = fmt.Errorf("unable to parse content range end: %w", err)
			logger.V(1).Error(err)
			http.Error(w, "invalid content range", http.StatusBadRequest)
			return false
		}
	}

	return true
}

func (w *_Writer) SetContentEncoder(ctx context.Context) bool {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	w.WriteCloser = ioutil.NopWriteCloser(w.ResponseWriter)

	for _, opt := range httputil.ParseOptions(w.Request.Header.Get("Accept-Encoding")) {
		var q *float64
		if f, e := strconv.ParseFloat(opt.Map["q"], 64); e == nil {
			q = &f
		} else {
			logger.V(1).Warningf("quality level parse error: %w", e)
		}
		switch key := opt.KeyVals[0].Key; key {
		case "gzip":
			level := gzip.DefaultCompression
			if q != nil {
				newLevel := int(*q)
				if gzip.NoCompression <= newLevel && newLevel <= gzip.BestCompression {
					level = newLevel
				} else {
					logger.V(1).Warningf("invalid quality level %d", newLevel)
				}
			}
			w.WriteCloser, _ = gzip.NewWriterLevel(w.ResponseWriter, level)
			w.ContentEncoding = key
			w.Header().Set("Content-Encoding", w.ContentEncoding)
			return true
		case "deflate":
			level := flate.DefaultCompression
			if q != nil {
				newLevel := int(*q)
				if flate.NoCompression <= newLevel && newLevel <= flate.BestCompression {
					level = newLevel
				} else {
					logger.V(1).Warningf("invalid quality level %d", newLevel)
				}
			}
			w.WriteCloser, _ = flate.NewWriter(w.ResponseWriter, level)
			w.ContentEncoding = key
			w.Header().Set("Content-Encoding", w.ContentEncoding)
			return true
		}
	}

	return false
}
