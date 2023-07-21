package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/goinsane/xcontext"
	"github.com/valyala/tcplisten"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/orkunkaraduman/oscdn/store"
)

type Server struct {
	ctx      xcontext.CancelableContext
	wg       sync.WaitGroup
	stopOnce sync.Once

	config   Config
	listener net.Listener
	httpSrv  *http.Server
}

func NewServer(config Config) (result *Server, err error) {
	s := &Server{
		ctx:    xcontext.WithCancelable2(context.WithValue(context.Background(), "logger", config.Logger)),
		config: config,
	}

	logger := config.Logger

	if config.ListenBacklog > 0 {
		s.listener, err = (&tcplisten.Config{
			ReusePort:   true,
			DeferAccept: false,
			FastOpen:    true,
			Backlog:     config.ListenBacklog,
		}).NewListener("tcp4", config.Listen)
	} else {
		s.listener, err = net.Listen("tcp4", config.Listen)
	}
	if err != nil {
		logger.Errorf("listen error: %w", err)
		return
	}
	defer func() {
		if err != nil {
			_ = s.listener.Close()
		}
	}()
	logger.Infof("listening %q", config.Listen)

	s.httpSrv = &http.Server{
		Handler: h2c.NewHandler(http.HandlerFunc(s.httpHandler), &http2.Server{
			MaxHandlers:                  0,
			MaxConcurrentStreams:         0,
			MaxReadFrameSize:             0,
			PermitProhibitedCipherSuites: false,
			IdleTimeout:                  65 * time.Second,
			MaxUploadBufferPerConnection: 0,
			MaxUploadBufferPerStream:     0,
			NewWriteScheduler:            nil,
		}),
		TLSConfig:         config.TLSConfig,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       65 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          log.New(io.Discard, "", log.LstdFlags),
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if s.httpSrv.TLSConfig == nil {
			if e := s.httpSrv.Serve(s.listener); e != nil && e != http.ErrServerClosed {
				logger.Errorf("http serve error: %w", e)
			}
		} else {
			if e := s.httpSrv.ServeTLS(s.listener, "", ""); e != nil && e != http.ErrServerClosed {
				logger.Errorf("https serve error: %w", e)
			}
		}
	}()

	return s, nil
}

func (s *Server) Stop(ctx context.Context) (err error) {
	s.stopOnce.Do(func() {
		if e := s.httpSrv.Shutdown(ctx); e != nil && err == nil {
			err = fmt.Errorf("http server shutdown error: %w", e)
		}
		s.ctx.Cancel()
		s.wg.Wait()
	})
	return
}

func (s *Server) httpHandler(w http.ResponseWriter, req *http.Request) {
	var err error

	s.wg.Add(1)
	defer s.wg.Done()
	if s.ctx.Err() != nil {
		return
	}

	defer func() {
		if p := recover(); p != nil {
			go func() {
				panic(p)
			}()
		}
	}()

	logger := s.config.Logger.WithFieldKeyVals("scheme", req.URL.Scheme,
		"host", req.URL.Host, "requestURI", req.RequestURI, "remoteAddr", req.RemoteAddr)
	ctx := context.WithValue(context.Background(), "logger", logger)

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

	getResult, err := s.config.Store.Get(ctx, req.URL.String(), "", contentRange)
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
		_, err = io.Copy(w, getResult)
	}
	if err != nil {
		err = fmt.Errorf("content upload error: %w", err)
		logger.V(2).Error(err)
		return
	}
}
