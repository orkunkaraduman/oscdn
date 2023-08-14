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

	a.ctx = xcontext.WithCancelable2(context.WithValue(context.Background(), "logger", a.Logger))

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
	a.httpServeMux.Handle("/cdn/", http.StripPrefix("/cdn", http.HandlerFunc(a.cdnHandler)))

	a.httpSrv = &http.Server{
		Handler:           http.HandlerFunc(a.httpHandler),
		TLSConfig:         nil,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
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
		_ = a.httpSrv.Close()
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
	ctx := a.ctx
	logger := a.Logger

	defer func() {
		if p := recover(); p != nil {
			logger.Fatal(p)
		}
	}()

	a.wg.Add(1)
	defer a.wg.Done()
	if ctx.Err() != nil {
		return
	}

	a.httpServeMux.ServeHTTP(w, req)
}

func (a *MgmtApp) cdnHandler(w http.ResponseWriter, req *http.Request) {
	var err error

	ctx := a.ctx

	values, _ := url.ParseQuery(req.URL.RawQuery)

	switch {

	case req.URL.Path == "/":
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)

	case req.URL.Path == "/purge":
		if req.Method != http.MethodPost {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			break
		}
		err = a.Store.Purge(ctx, values.Get("url"), values.Get("host"))
		switch err {
		case store.ErrNotExists:
			http.Error(w, "content not exists", http.StatusNotFound)
		default:
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		case nil:
			_, _ = fmt.Fprintln(w, "content purged")
		}

	case req.URL.Path == "/purge_host":
		if req.Method != http.MethodPost {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			break
		}
		err = a.Store.PurgeHost(ctx, values.Get("host"))
		switch err {
		case store.ErrNotExists:
			http.Error(w, "host not exists", http.StatusNotFound)
		default:
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		case nil:
			_, _ = fmt.Fprintln(w, "host purged")
		}

	case req.URL.Path == "/purge_all":
		if req.Method != http.MethodPost {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			break
		}
		err = a.Store.PurgeAll(ctx)
		switch err {
		default:
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		case nil:
			_, _ = fmt.Fprintln(w, "all purged")
		}

	default:
		http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)

	}
}

var mgmtDebugMux = new(http.ServeMux)
