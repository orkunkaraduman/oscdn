package store

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/goinsane/filelock"
	"github.com/goinsane/logng"

	"github.com/orkunkaraduman/oscdn/namedlock"
)

type Store struct {
	config      Config
	orjConfig   Config
	httpClient  *http.Client
	namedLock   *namedlock.NamedLock
	downloads   map[string]*_Download
	downloadsMu sync.RWMutex

	lockFile *filelock.File

	releaseOnce sync.Once
}

func New(config Config) (s *Store, err error) {
	s = &Store{
		config:    config,
		orjConfig: config,
		httpClient: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   3 * time.Second,
					KeepAlive: time.Second,
				}).DialContext,
				TLSClientConfig:        config.TLSConfig.Clone(),
				TLSHandshakeTimeout:    3 * time.Second,
				MaxIdleConns:           config.DownloadMaxIdle,
				IdleConnTimeout:        65 * time.Second,
				ResponseHeaderTimeout:  5 * time.Second,
				ExpectContinueTimeout:  1 * time.Second,
				MaxResponseHeaderBytes: 1024 * 1024,
				ForceAttemptHTTP2:      true,
			},
		},
		namedLock: namedlock.New(),
		downloads: make(map[string]*_Download, 4096),
	}

	if s.config.Path == "" {
		s.config.Path = "."
	}
	s.lockFile, err = filelock.Create(filepath.FromSlash(path.Clean(s.config.Path+"/lock")), 0666)
	if err != nil {
		return nil, fmt.Errorf("unable to get store lock: %w", err)
	}
	defer func() {
		if err != nil {
			_ = s.lockFile.Release()
		}
	}()

	return s, nil
}

func (s *Store) Release() (err error) {
	s.releaseOnce.Do(func() {
		if e := s.lockFile.Release(); e != nil && err == nil {
			err = fmt.Errorf("unable to release store lock: %w", e)
		}
	})
	return
}

func (s *Store) Get(ctx context.Context, rawURL string, host string) (h http.Header, r io.ReadCloser, err error) {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to parse raw url: %w", err)
	}

	keyHost := u.Host
	if host != "" {
		keyHost = host
	}
	keyURL := (&url.URL{
		Scheme:   u.Scheme,
		Host:     keyHost,
		Path:     u.Path,
		RawQuery: u.RawQuery,
	}).String()

	dataPath := s.getDataPath(keyURL, u.Host)

	locker := s.namedLock.Locker(keyURL)
	locker.Lock()
	defer locker.Unlock()

	s.downloadsMu.RLock()
	download := s.downloads[keyURL]
	s.downloadsMu.RUnlock()

	if download != nil {
		pr, pw := io.Pipe()

	}

	return
}

func (s *Store) getDataPath(rawURL string, subDir string) string {
	h := sha256.Sum256([]byte((rawURL)))
	result := s.config.Path + "/" + subDir
	for i, j := 0, len(h); i < j; i = i + 2 {
		result += fmt.Sprintf("%c%04x", '/', h[i:i+2])
	}
	return result
}
