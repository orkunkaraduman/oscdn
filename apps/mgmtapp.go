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
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/orkunkaraduman/oscdn/store"
)

type MgmtApp struct {
	Logger *logng.Logger
	Listen string
	Store  *store.Store

	wg sync.WaitGroup

	listener     net.Listener
	httpServeMux *http.ServeMux
	httpSrv      *http.Server
}

func (a *MgmtApp) Start(ctx context.Context, cancel context.CancelFunc) {
	var err error

	logger := a.Logger
	ctx = context.WithValue(ctx, "logger", a.Logger)

	a.listener, err = net.Listen("tcp4", a.Listen)
	if err != nil {
		logger.Errorf("listen error: %w", err)
		cancel()
		return
	}
	logger.Infof("listening %q.", a.Listen)

	a.httpServeMux = new(http.ServeMux)
	a.httpServeMux.Handle("/debug/", mgmtDebugMux)
	a.httpServeMux.Handle("/metrics/", promhttp.Handler())
	a.httpServeMux.Handle("/mgmt/", http.StripPrefix("/mgmt", http.HandlerFunc(a.mgmtHandler)))

	a.httpSrv = &http.Server{
		Handler:           http.HandlerFunc(a.httpHandler),
		TLSConfig:         nil,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       65 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          log.New(io.Discard, "", log.LstdFlags),
	}
}

func (a *MgmtApp) Run(ctx context.Context, cancel context.CancelFunc) {
	logger := a.Logger
	ctx = context.WithValue(ctx, "logger", a.Logger)

	logger.Info("started.")

	if e := a.httpSrv.Serve(a.listener); e != nil && e != http.ErrServerClosed {
		logger.Errorf("http serve error: %w", e)
		cancel()
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

	if a.listener != nil {
		_ = a.listener.Close()
	}

	a.wg.Wait()
	logger.Info("stopped.")
}

func (a *MgmtApp) httpHandler(w http.ResponseWriter, req *http.Request) {
	logger := a.Logger.WithFieldKeyVals("requestHash", generateRequestHash())

	defer func() {
		if p := recover(); p != nil {
			logger.Fatal(p)
		}
	}()

	a.wg.Add(1)
	defer a.wg.Done()

	a.httpServeMux.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), "logger", logger)))
}

func (a *MgmtApp) mgmtHandler(w http.ResponseWriter, req *http.Request) {
	var err error

	ctx := req.Context()

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
