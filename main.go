package main

import (
	"context"
	"fmt"
	"io"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/goinsane/logng"

	"github.com/orkunkaraduman/oscdn/store"
)

func main() {
	logng.Info("aa")
	s, err := store.New(store.Config{
		Path:          "/Users/orkun/narvi",
		MaxAge:        48 * time.Hour,
		TLSConfig:     nil,
		UserAgent:     "oscdn",
		MaxIdleConns:  100,
		DownloadBurst: 4,
		DownloadRate:  2048,
	})
	if err != nil {
		panic(err)
	}

	ctx, _ := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	//fmt.Println(s.Purge(context.Background(), "https://speed.hetzner.de/100MB.bin", ""))
	//fmt.Println(s.PurgeHost(context.Background(), "speed.hetzner.de"))
	//return

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		<-ctx.Done()
		_ = s.Release()
	}()

	go func() {
		defer wg.Done()
		result, err := s.Get(context.Background(), "https://speed.hetzner.de/100MB.bin", "")
		if err != nil {
			logng.Error(err)
			return
		}
		//goland:noinspection GoUnhandledErrorResult
		defer result.Close()
		fmt.Println(result.StatusCode)
		fmt.Printf("%+v\n", result.Header)
		written, err := io.Copy(io.Discard, result)
		if err != nil {
			logng.Error(err)
			return
		}
		fmt.Println(written)
	}()
	go func() {
		defer wg.Done()
		result, err := s.Get(context.Background(), "https://speed.hetzner.de/100MB.bin", "")
		if err != nil {
			logng.Error(err)
			return
		}
		//goland:noinspection GoUnhandledErrorResult
		defer result.Close()
		fmt.Println(result.StatusCode)
		fmt.Printf("%+v\n", result.Header)
		written, err := io.Copy(io.Discard, result)
		if err != nil {
			logng.Error(err)
			return
		}
		fmt.Println(written)
	}()

	wg.Wait()
}
