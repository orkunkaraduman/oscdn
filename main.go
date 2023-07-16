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
	//goland:noinspection GoUnhandledErrorResult
	defer s.Release()

	ctx, _ := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()
		result, err := s.Get(ctx, "https://speed.hetzner.de/100MB.bin", "")
		if err != nil {
			panic(err)
		}
		//goland:noinspection GoUnhandledErrorResult
		defer result.Close()
		fmt.Println(result.StatusCode)
		fmt.Printf("%+v\n", result.Header)
		written, err := io.Copy(io.Discard, result)
		if err != nil {
			panic(err)
		}
		fmt.Println(written)
	}()
	go func() {
		defer wg.Done()
		result, err := s.Get(ctx, "https://speed.hetzner.de/100MB.bin", "")
		if err != nil {
			panic(err)
		}
		//goland:noinspection GoUnhandledErrorResult
		defer result.Close()
		fmt.Println(result.StatusCode)
		fmt.Printf("%+v\n", result.Header)
		written, err := io.Copy(io.Discard, result)
		if err != nil {
			panic(err)
		}
		fmt.Println(written)
	}()

	wg.Wait()
}
