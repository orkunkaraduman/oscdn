package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/goinsane/application"
	"github.com/goinsane/flagbind"
	"github.com/goinsane/flagconf"
	"github.com/goinsane/logng"

	"github.com/orkunkaraduman/oscdn/apps"
	"github.com/orkunkaraduman/oscdn/cdn"
	"github.com/orkunkaraduman/oscdn/httputil"
	"github.com/orkunkaraduman/oscdn/internal/config"
	"github.com/orkunkaraduman/oscdn/internal/flags"
	"github.com/orkunkaraduman/oscdn/store"
)

func main() {
	var err error

	appName := "oscdn"

	logng.SetSeverity(logng.SeverityInfo)
	logng.SetVerbose(0)
	logng.SetPrintSeverity(logng.SeverityInfo)
	logng.SetStackTraceSeverity(logng.SeverityError)
	logng.SetTextOutputWriter(os.Stdout)
	logng.SetTextOutputFlags(logng.TextOutputFlagDefault | logng.TextOutputFlagLongFunc)
	logng.SetOutput(logng.NewJSONOutput(os.Stdout, logng.JSONOutputFlagDefault))
	flagSet := flag.NewFlagSet(appName, flag.ExitOnError)
	flagbind.Bind(flagSet, flags.Flags)
	if confPath := os.Getenv("OSCDN_CONF"); confPath != "" {
		err = flagconf.ParseFile(flagSet, confPath, os.Args[1:])
		if err != nil {
			logng.Errorf("flags config file parse error: %w", err)
			return
		}
	} else {
		err = flagSet.Parse(os.Args[1:])
		if err != nil {
			return
		}
	}
	err = flags.Flags.Validate()
	if err != nil {
		logng.Errorf("flags validate error: %w", err)
		return
	}

	logng.SetVerbose(logng.Verbose(flags.Flags.Verbose))
	if flags.Flags.Verbose > 0 {
		logng.SetStackTraceSeverity(logng.SeverityWarning)
	}
	if flags.Flags.Debug {
		logng.SetSeverity(logng.SeverityDebug)
		logng.SetStackTraceSeverity(logng.SeverityDebug)
	}

	appCtx, appCtxCancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill, syscall.SIGTERM)
	defer appCtxCancel()

	logng.Info("starting.")

	_config, err := config.FromFile(flags.Flags.Config)
	if err != nil {
		err = fmt.Errorf("config load error: %w", err)
		logng.Error(err)
		return
	}
	err = _config.Validate()
	if err != nil {
		err = fmt.Errorf("config validate error: %w", err)
		logng.Error(err)
		return
	}
	certs, err := _config.TLSCertificates()
	if err != nil {
		err = fmt.Errorf("config get tls certificates error: %w", err)
		logng.Error(err)
		return
	}

	_store, err := store.New(store.Config{
		Logger:       logng.WithFieldKeyVals("logger", "store"),
		Path:         flags.Flags.StorePath,
		TLSConfig:    nil,
		MaxIdleConns: flags.Flags.StoreMaxIdleConns,
		UserAgent:    flags.Flags.StoreUserAgent,
		DefaultHostConfig: &store.HostConfig{
			MaxSize:       1024 * 1024 * 1024,
			MaxAge:        24 * time.Hour,
			DownloadBurst: 0,
			DownloadRate:  0,
		},
		GetHostConfig: func(scheme, host string) *store.HostConfig {
			o, ok := _config.Origins[host]
			if !ok {
				return nil
			}
			return &store.HostConfig{
				MaxSize:        o.MaxSize,
				MaxAge:         o.MaxAge,
				MaxAge404:      o.MaxAge404,
				MaxAgeOverride: o.MaxAgeOverride,
				DownloadBurst:  o.DownloadBurst,
				DownloadRate:   o.DownloadRate,
			}
		},
	})
	if err != nil {
		err = fmt.Errorf("store create error: %w", err)
		logng.Error(err)
		return
	}
	defer func(s *store.Store) {
		if err != nil {
			_ = s.Release()
		}
	}(_store)

	handler := &cdn.Handler{
		Store:        _store,
		ServerHeader: flags.Flags.ServerHeader,
		GetHostConfig: func(scheme, host string) *cdn.HostConfig {
			domain, _, _ := httputil.SplitHost(host)
			d, ok := _config.Domains[domain]
			if !ok {
				return nil
			}
			o, ok := _config.Origins[d.Origin]
			if !ok {
				return nil
			}
			result := &cdn.HostConfig{
				HttpsRedirect:     d.HttpsRedirect,
				HttpsRedirectPort: d.HttpsRedirectPort,
				DomainOverride:    d.DomainOverride,
				IgnoreQuery:       d.IgnoreQuery,
				UploadBurst:       d.UploadBurst,
				UploadRate:        d.UploadRate,
			}
			result.Origin.Scheme = "http"
			if o.UseHttps {
				result.Origin.Scheme = "https"
			}
			result.Origin.Host = d.Origin
			return result
		},
	}

	mainApp := new(application.Instance)
	mainApp.StartFunc = func(ctx context.Context, cancel context.CancelFunc) {
	}
	mainApp.RunFunc = func(ctx context.Context, cancel context.CancelFunc) {
	}
	mainApp.TerminateFunc = func(ctx context.Context) {
	}
	mainApp.StopFunc = func() {
		err := _store.Release()
		if err != nil {
			err = fmt.Errorf("store release error: %w", err)
			logng.Error(err)
		}
	}

	httpApp := &apps.HttpApp{
		Logger:        logng.WithFieldKeyVals("logger", "http app"),
		Listen:        flags.Flags.Http,
		ListenBacklog: flags.Flags.ListenBacklog,
		MaxConns:      flags.Flags.MaxConns,
		HandleH2C:     flags.Flags.HandleH2c,
		TLSConfig:     nil,
		Handler:       handler,
	}

	httpsApp := &apps.HttpApp{
		Logger:        logng.WithFieldKeyVals("logger", "https app"),
		Listen:        flags.Flags.Https,
		ListenBacklog: flags.Flags.ListenBacklog,
		MaxConns:      flags.Flags.MaxConns,
		HandleH2C:     false,
		TLSConfig: &tls.Config{
			GetCertificate: func(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
				return certs[info.ServerName], nil
			},
		},
		Handler: handler,
	}
	switch flags.Flags.MinTlsVersion {
	case "1.0":
		httpsApp.TLSConfig.MinVersion = tls.VersionTLS10
	case "1.1":
		httpsApp.TLSConfig.MinVersion = tls.VersionTLS11
	case "1.2":
		httpsApp.TLSConfig.MinVersion = tls.VersionTLS12
	case "1.3":
		httpsApp.TLSConfig.MinVersion = tls.VersionTLS13
	default:
		err = errors.New("unknown minimum tls version")
		panic(err)
	}

	mgmtApp := &apps.MgmtApp{
		Logger: logng.WithFieldKeyVals("logger", "mgmt app"),
		Listen: flags.Flags.Mgmt,
		Store:  _store,
	}

	if !application.RunAll(appCtx, []application.Application{mainApp, httpApp, httpsApp, mgmtApp}, flags.Flags.TerminateTimeout, flags.Flags.QuitTimeout) {
		logng.Error("quit timeout")
	}
	logng.Info("stopped.")
}
