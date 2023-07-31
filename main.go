package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/goinsane/application"
	"github.com/goinsane/flagbind"
	"github.com/goinsane/flagconf"
	"github.com/goinsane/logng"

	"github.com/orkunkaraduman/oscdn/internal/flags"
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

	if !application.RunAll(appCtx, []application.Application{
		&MgmtApp{
			Listen: flags.Flags.Mgmt,
		},
	}, flags.Flags.TerminateTimeout, flags.Flags.QuitTimeout) {
		logng.Error("quit timeout")
	}
	logng.Info("stopped.")

}
