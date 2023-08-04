package apps

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
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
	HandleH2C     bool
	TLSConfig     *tls.Config
	Handler       *cdn.Handler

	ctx xcontext.CancelableContext
	wg  sync.WaitGroup

	listener net.Listener
	httpSrv  *http.Server
}

func (a *HttpApp) Start(ctx xcontext.CancelableContext) {
	var err error

	a.ctx = xcontext.WithCancelable2(context.Background())

	logger := a.Logger

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
		logger.Errorf("listen error: %w", err)
		ctx.Cancel()
		return
	}
	logger.Infof("listening %q.", a.Listen)

	var httpHandler http.Handler
	httpHandler = a.Handler
	if a.HandleH2C {
		httpHandler = h2c.NewHandler(http.HandlerFunc(a.httpHandler), &http2.Server{
			MaxHandlers:                  0,
			MaxConcurrentStreams:         0,
			MaxReadFrameSize:             0,
			PermitProhibitedCipherSuites: false,
			IdleTimeout:                  65 * time.Second,
			MaxUploadBufferPerConnection: 0,
			MaxUploadBufferPerStream:     0,
			NewWriteScheduler:            nil,
		})
	}
	a.httpSrv = &http.Server{
		Handler:           httpHandler,
		TLSConfig:         a.TLSConfig.Clone(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       65 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          log.New(io.Discard, "", log.LstdFlags),
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			var tcpConn *net.TCPConn
			switch conn := c.(type) {
			case *net.TCPConn:
				tcpConn = conn
			case *tls.Conn:
				tcpConn = conn.NetConn().(*net.TCPConn)
			default:
				panic("unknown conn type")
			}
			_ = tcpConn.SetLinger(-1)
			_ = tcpConn.SetReadBuffer(128 * 1024)
			_ = tcpConn.SetWriteBuffer(128 * 1024)
			return ctx
		},
	}
}

func (a *HttpApp) Run(ctx xcontext.CancelableContext) {
	logger := a.Logger

	logger.Info("started.")

	if a.httpSrv.TLSConfig == nil {
		if e := a.httpSrv.Serve(a.listener); e != nil && e != http.ErrServerClosed {
			logger.Errorf("http serve error: %w", e)
			ctx.Cancel()
			return
		}
	} else {
		if e := a.httpSrv.ServeTLS(a.listener, "", ""); e != nil && e != http.ErrServerClosed {
			logger.Errorf("https serve error: %w", e)
			ctx.Cancel()
			return
		}
	}
}

func (a *HttpApp) Terminate(ctx context.Context) {
	logger := a.Logger

	if e := a.httpSrv.Shutdown(ctx); e != nil {
		logger.Errorf("http server shutdown error: %w", e)
	}

	logger.Info("terminated.")
}

func (a *HttpApp) Stop() {
	logger := a.Logger

	a.ctx.Cancel()

	if a.listener != nil {
		_ = a.listener.Close()
	}

	a.wg.Wait()
	logger.Info("stopped.")
}

func (a *HttpApp) httpHandler(w http.ResponseWriter, req *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logng.Fatal(p)
		}
	}()

	a.wg.Add(1)
	defer a.wg.Done()
	if a.ctx.Err() != nil {
		return
	}

	a.Handler.ServeHTTP(w, req)
}

type MgmtApp struct {
	Logger *logng.Logger
	Listen string

	ctx xcontext.CancelableContext
	wg  sync.WaitGroup

	listener     net.Listener
	httpServeMux *http.ServeMux
	httpSrv      *http.Server
}

func (a *MgmtApp) Start(ctx xcontext.CancelableContext) {
	var err error

	a.ctx = xcontext.WithCancelable2(context.Background())

	logger := a.Logger

	a.listener, err = net.Listen("tcp4", a.Listen)
	if err != nil {
		logger.Errorf("listen error: %w", err)
		ctx.Cancel()
		return
	}
	logger.Infof("listening %q.", a.Listen)

	a.httpServeMux = new(http.ServeMux)
	a.httpServeMux.Handle("/debug/", mgmtDebugMux)
	a.httpServeMux.Handle("/metrics/", promhttp.Handler())

	a.httpSrv = &http.Server{
		Handler:           http.HandlerFunc(a.httpHandler),
		TLSConfig:         nil,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       65 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          log.New(io.Discard, "", log.LstdFlags),
	}
}

func (a *MgmtApp) Run(ctx xcontext.CancelableContext) {
	logger := a.Logger

	logger.Info("started.")

	if e := a.httpSrv.Serve(a.listener); e != nil && e != http.ErrServerClosed {
		logger.Errorf("http serve error: %w", e)
		ctx.Cancel()
		return
	}
}

func (a *MgmtApp) Terminate(ctx context.Context) {
	logger := a.Logger

	if e := a.httpSrv.Shutdown(ctx); e != nil {
		logger.Errorf("http server shutdown error: %w", e)
	}

	logger.Info("terminated.")
}

func (a *MgmtApp) Stop() {
	logger := a.Logger

	a.ctx.Cancel()

	if a.listener != nil {
		_ = a.listener.Close()
	}

	a.wg.Wait()
	logger.Info("stopped.")
}

func (a *MgmtApp) httpHandler(w http.ResponseWriter, req *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logng.Fatal(p)
		}
	}()

	a.wg.Add(1)
	defer a.wg.Done()
	if a.ctx.Err() != nil {
		return
	}

	a.httpServeMux.ServeHTTP(w, req)
}

var mgmtDebugMux = new(http.ServeMux)
