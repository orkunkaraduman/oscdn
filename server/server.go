package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/goinsane/xcontext"
	"github.com/valyala/tcplisten"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type Server struct {
	ctx       xcontext.CancelableContext
	wg        sync.WaitGroup
	closeOnce sync.Once

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
	s.closeOnce.Do(func() {
		if e := s.httpSrv.Shutdown(ctx); e != nil && err == nil {
			err = fmt.Errorf("http server shutdown error: %w", e)
		}
		s.ctx.Cancel()
		s.wg.Wait()
	})
	return
}

func (s *Server) httpHandler(rw http.ResponseWriter, req *http.Request) {
	s.wg.Add(1)
	defer s.wg.Done()
	if s.ctx.Err() != nil {
		return
	}

}
