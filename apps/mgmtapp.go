package apps

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/goinsane/logng"
	"github.com/goinsane/xcontext"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/orkunkaraduman/oscdn/store"
)

type MgmtApp struct {
	Logger *logng.Logger
	Listen string
	Store  *store.Store

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
	a.httpServeMux.HandleFunc("/cdn/", a.cdnHandler)

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

func (a *MgmtApp) cdnHandler(w http.ResponseWriter, req *http.Request) {
	var err error

	ctx := context.WithValue(a.ctx, "logger", a.Logger)

	values, _ := url.ParseQuery(req.URL.RawQuery)

	switch {

	case req.RequestURI == "/cdn/purge":
		if req.Method != http.MethodPost {
			_, _ = w.Write([]byte(fmt.Sprintf("%s\n", http.StatusText(http.StatusMethodNotAllowed))))
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		err = a.Store.Purge(ctx, values.Get("url"), values.Get("host"))
		switch err {
		case store.ErrNotExists:
			_, _ = w.Write([]byte("content not exists"))
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = w.Write([]byte("internal error"))
			w.WriteHeader(http.StatusInternalServerError)
		case nil:
			_, _ = w.Write([]byte("content purged"))
			w.WriteHeader(http.StatusOK)
		}

	case req.RequestURI == "/cdn/purge_host":
		if req.Method != http.MethodPost {
			_, _ = w.Write([]byte(fmt.Sprintf("%s\n", http.StatusText(http.StatusMethodNotAllowed))))
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		err = a.Store.PurgeHost(ctx, values.Get("host"))
		switch err {
		case store.ErrNotExists:
			_, _ = w.Write([]byte("host not exists"))
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = w.Write([]byte("internal error"))
			w.WriteHeader(http.StatusInternalServerError)
		case nil:
			_, _ = w.Write([]byte("host purged"))
			w.WriteHeader(http.StatusOK)
		}

	}
}

var mgmtDebugMux = new(http.ServeMux)
