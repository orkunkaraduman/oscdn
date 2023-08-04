//go:build debug
// +build debug

package apps

import "net/http/pprof"

func init() {
	mgmtDebugMux.HandleFunc("/debug/pprof/", pprof.Index)
	mgmtDebugMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mgmtDebugMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mgmtDebugMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mgmtDebugMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}
