package apps

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goinsane/logng"
	"github.com/valyala/tcplisten"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/orkunkaraduman/oscdn/cdn"
)

type HttpApp struct {
	Logger        *logng.Logger
	Listen        string
	ListenBacklog int
	MaxConns      int32
	HandleH2C     bool
	TLSConfig     *tls.Config
	Handler       *cdn.Handler

	wg sync.WaitGroup

	listener net.Listener
	httpSrv  *http.Server

	connCount int32
}

func (a *HttpApp) Start(ctx context.Context, cancel context.CancelFunc) {
	var err error

	logger := a.Logger
	ctx = context.WithValue(ctx, "logger", a.Logger)

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
		cancel()
		return
	}
	logger.Infof("listening %q.", a.Listen)

	var httpHandler http.Handler
	httpHandler = http.HandlerFunc(a.httpHandler)
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
		ConnState: func(conn net.Conn, state http.ConnState) {
			switch state {
			case http.StateNew:
				atomic.AddInt32(&a.connCount, 1)

				if a.MaxConns > 0 && a.MaxConns < a.connCount {
					_ = conn.Close()
					logger.Error("max conns exceeded")
					break
				}

				var tcpConn *net.TCPConn
				switch conn := conn.(type) {
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
			case http.StateClosed:
				atomic.AddInt32(&a.connCount, -1)
			}
		},
		ErrorLog: log.New(io.Discard, "", log.LstdFlags),
	}
}

func (a *HttpApp) Run(ctx context.Context, cancel context.CancelFunc) {
	logger := a.Logger
	ctx = context.WithValue(ctx, "logger", a.Logger)

	logger.Info("started.")

	if a.httpSrv.TLSConfig == nil {
		if e := a.httpSrv.Serve(a.listener); e != nil && e != http.ErrServerClosed {
			logger.Errorf("http serve error: %w", e)
			cancel()
			return
		}
	} else {
		if e := a.httpSrv.ServeTLS(a.listener, "", ""); e != nil && e != http.ErrServerClosed {
			logger.Errorf("https serve error: %w", e)
			cancel()
			return
		}
	}
}

func (a *HttpApp) Terminate(ctx context.Context) {
	logger := a.Logger

	if e := a.httpSrv.Shutdown(ctx); e != nil {
		logger.Errorf("http server shutdown error: %w", e)
		_ = a.httpSrv.Close()
	}

	logger.Info("terminated.")
}

func (a *HttpApp) Stop() {
	logger := a.Logger

	if a.listener != nil {
		_ = a.listener.Close()
	}

	a.wg.Wait()
	logger.Info("stopped.")
}

func (a *HttpApp) httpHandler(w http.ResponseWriter, req *http.Request) {
	logger := a.Logger

	defer func() {
		if p := recover(); p != nil {
			logger.Fatal(p)
		}
	}()

	a.wg.Add(1)
	defer a.wg.Done()

	a.Handler.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), "logger", logger)))
}
