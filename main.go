package main

import (
	"context"
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
			logng.Fatal(err)
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
		logng.Fatal("unable to validate flags")
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

	c, err := config.FromFile(flags.Flags.Config)
	if err != nil {
		err = fmt.Errorf("config load error: %w", err)
		logng.Error(err)
		return
	}
	err = c.Validate()
	if err != nil {
		err = fmt.Errorf("config validate error: %w", err)
		logng.Error(err)
		return
	}

	s, err := store.New(store.Config{
		Logger:       logng.WithFieldKeyVals("logger", "store"),
		Path:         flags.Flags.StorePath,
		TLSConfig:    nil,
		MaxIdleConns: 100,
		UserAgent:    "oscdn",
		DefaultHostConfig: &store.HostConfig{
			MaxSize:       1024 * 1024 * 1024,
			MaxAge:        24 * time.Hour,
			DownloadBurst: 0,
			DownloadRate:  0,
		},
		GetHostConfig: func(scheme, host string) *store.HostConfig {
			o, ok := c.Origins[host]
			if !ok {
				return nil
			}
			return &store.HostConfig{
				MaxSize:       o.MaxSize,
				MaxAge:        o.MaxAge,
				DownloadBurst: o.DownloadBurst,
				DownloadRate:  o.DownloadRate,
			}
		},
	})
	if err != nil {
		err = fmt.Errorf("store create error: %w", err)
		logng.Error(err)
		return
	}
	defer func(s *store.Store) {
		_ = s.Release()
	}(s)

	h := &cdn.Handler{
		Logger:  logng.WithFieldKeyVals("logger", "handler"),
		Context: nil,
		Store:   s,
		GetHostConfig: func(scheme, host string) *cdn.HostConfig {
			h, ok := c.Hosts[host]
			if !ok {
				return nil
			}
			o, ok := c.Origins[h.Origin]
			if !ok {
				return nil
			}
			result := &cdn.HostConfig{
				HostOverride:  h.HostOverride,
				IgnoreQuery:   h.IgnoreQuery,
				HttpsRedirect: h.HttpsRedirect,
				UploadBurst:   h.UploadBurst,
				UploadRate:    h.UploadRate,
			}
			result.Origin.Scheme = "http"
			if o.UseHttps {
				result.Origin.Scheme = "https"
			}
			result.Origin.Host = h.Origin
			return result
		},
	}

	httpApp := &apps.HttpApp{
		Logger:        logng.WithFieldKeyVals("logger", "http app"),
		Listen:        flags.Flags.Http,
		ListenBacklog: flags.Flags.ListenBacklog,
		TLSConfig:     nil,
		Handler:       h,
	}

	mgmtApp := &apps.MgmtApp{
		Logger:  logng.WithFieldKeyVals("logger", "mgmt app"),
		Listen:  flags.Flags.Mgmt,
		Handler: h,
	}

	if !application.RunAll(appCtx, []application.Application{httpApp, mgmtApp}, flags.Flags.TerminateTimeout, flags.Flags.QuitTimeout) {
		logng.Error("quit timeout")
	}
	logng.Info("stopped.")
}
