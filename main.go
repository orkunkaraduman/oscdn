package main

import (
	"context"
	"fmt"
	"io"
	"sync"
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

	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()
		statusCode, header, r, err := s.Get(context.Background(), "https://speed.hetzner.de/100MB.bin", "")
		if err != nil {
			panic(err)
		}
		fmt.Println(statusCode)
		fmt.Printf("%+v\n", header)
		written, err := io.Copy(io.Discard, r)
		if err != nil {
			panic(err)
		}
		fmt.Println(written)
	}()
	go func() {
		defer wg.Done()
		statusCode, header, r, err := s.Get(context.Background(), "https://speed.hetzner.de/100MB.bin", "")
		if err != nil {
			panic(err)
		}
		fmt.Println(statusCode)
		fmt.Printf("%+v\n", header)
		written, err := io.Copy(io.Discard, r)
		if err != nil {
			panic(err)
		}
		fmt.Println(written)
	}()

	wg.Wait()
}
