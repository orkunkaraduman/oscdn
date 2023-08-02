package apps

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	"github.com/goinsane/logng"
	"github.com/goinsane/xcontext"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/tcplisten"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/orkunkaraduman/oscdn/cdn"
)

type HttpApp struct {
	Logger        *logng.Logger
	Listen        string
	ListenBacklog int
	TLSConfig     *tls.Config
	Handler       *cdn.Handler

	logger *logng.Logger
	wg     sync.WaitGroup

	listener net.Listener
	httpSrv  *http.Server
}

func (a *HttpApp) Start(ctx xcontext.CancelableContext) {
	var err error

	a.logger = a.Logger.WithFieldKeyVals("listen", a.Listen)

	if a.ListenBacklog > 0 {
		a.listener, err = (&tcplisten.Config{
			ReusePort:   true,
			DeferAccept: false,
			FastOpen:    true,
			Backlog:     a.ListenBacklog,
		}).NewListener("tcp4", a.Listen)
	} else {
		a.listener, err = net.Listen("tcp4", a.Listen)
	}
	if err != nil {
		a.logger.Errorf("listen error: %w", err)
		ctx.Cancel()
		return
	}
	a.logger.Info("listening.")

	a.httpSrv = &http.Server{
		Handler: h2c.NewHandler(a.Handler, &http2.Server{
			MaxHandlers:                  0,
			MaxConcurrentStreams:         0,
			MaxReadFrameSize:             0,
			PermitProhibitedCipherSuites: false,
			IdleTimeout:                  65 * time.Second,
			MaxUploadBufferPerConnection: 0,
			MaxUploadBufferPerStream:     0,
			NewWriteScheduler:            nil,
		}),
		TLSConfig:         a.TLSConfig.Clone(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       65 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          log.New(io.Discard, "", log.LstdFlags),
	}
}

func (a *HttpApp) Run(ctx xcontext.CancelableContext) {
	a.logger.Info("started.")

	if a.httpSrv.TLSConfig == nil {
		if e := a.httpSrv.Serve(a.listener); e != nil && e != http.ErrServerClosed {
			a.logger.Errorf("http serve error: %w", e)
			ctx.Cancel()
			return
		}
	} else {
		if e := a.httpSrv.ServeTLS(a.listener, "", ""); e != nil && e != http.ErrServerClosed {
			a.logger.Errorf("https serve error: %w", e)
			ctx.Cancel()
			return
		}
	}
}

func (a *HttpApp) Terminate(ctx context.Context) {
	if e := a.httpSrv.Shutdown(ctx); e != nil {
		a.logger.Errorf("http server shutdown error: %w", e)
	}

	a.logger.Info("terminated.")
}

func (a *HttpApp) Stop() {
	if a.listener != nil {
		_ = a.listener.Close()
	}
	a.Handler.Wait()

	a.wg.Wait()
	a.logger.Info("stopped.")
}

type MgmtApp struct {
	Logger  *logng.Logger
	Listen  string
	Handler *cdn.Handler

	logger *logng.Logger
	wg     sync.WaitGroup

	listener     net.Listener
	httpServeMux *http.ServeMux
	httpSrv      *http.Server
}

func (a *MgmtApp) Start(ctx xcontext.CancelableContext) {
	var err error

	a.logger = a.Logger.WithFieldKeyVals("listen", a.Listen)

	a.listener, err = net.Listen("tcp4", a.Listen)
	if err != nil {
		a.logger.Errorf("listen error: %w", err)
		ctx.Cancel()
		return
	}
	a.logger.Info("listening.")

	a.httpServeMux = new(http.ServeMux)
	a.httpServeMux.HandleFunc("/debug/pprof/", pprof.Index)
	a.httpServeMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	a.httpServeMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	a.httpServeMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	a.httpServeMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	a.httpServeMux.Handle("/metrics/", promhttp.Handler())

	a.httpSrv = &http.Server{
		Handler:           a.httpServeMux,
		TLSConfig:         nil,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       65 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          log.New(io.Discard, "", log.LstdFlags),
	}
}

func (a *MgmtApp) Run(ctx xcontext.CancelableContext) {
	a.logger.Info("started.")

	if e := a.httpSrv.Serve(a.listener); e != nil && e != http.ErrServerClosed {
		a.logger.Errorf("http serve error: %w", e)
		ctx.Cancel()
		return
	}
}

func (a *MgmtApp) Terminate(ctx context.Context) {
	if e := a.httpSrv.Shutdown(ctx); e != nil {
		a.logger.Errorf("http server shutdown error: %w", e)
	}

	a.logger.Info("terminated.")
}

func (a *MgmtApp) Stop() {
	if a.listener != nil {
		_ = a.listener.Close()
	}

	a.wg.Wait()
	a.logger.Info("stopped.")
}
