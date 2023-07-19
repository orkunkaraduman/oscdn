package internal

import (
	"context"
	"sync"

	"github.com/goinsane/logng"
	"github.com/goinsane/xcontext"
)

var App = new(_App)

type _App struct {
	ctx context.Context
	wg  sync.WaitGroup
}

func (a *_App) Start(ctx xcontext.CancelableContext) {
	a.ctx = context.WithValue(ctx, "logger", logng.WithFieldKeyVals("logger", "application"))
}

func (a *_App) Run(ctx xcontext.CancelableContext) {

	// jobs

	logng.Info("application started.")
}

func (a *_App) Terminate(ctx context.Context) {

	logng.Info("application terminated.")
}

func (a *_App) Stop() {
	a.wg.Wait()
	logng.Info("application stopped.")
}
