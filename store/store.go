package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/goinsane/filelock"
	"github.com/goinsane/logng"

	"github.com/orkunkaraduman/oscdn/fsutil"
	"github.com/orkunkaraduman/oscdn/httphdr"
	"github.com/orkunkaraduman/oscdn/ioutil"
	"github.com/orkunkaraduman/oscdn/namedlock"
)

type Store struct {
	config      Config
	orjConfig   Config
	httpClient  *http.Client
	namedLock   *namedlock.NamedLock
	downloads   map[string]chan struct{}
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
				MaxIdleConns:           config.MaxIdleConns,
				IdleConnTimeout:        65 * time.Second,
				ResponseHeaderTimeout:  5 * time.Second,
				ExpectContinueTimeout:  1 * time.Second,
				MaxResponseHeaderBytes: 1024 * 1024,
				ForceAttemptHTTP2:      true,
			},
		},
		namedLock: namedlock.New(),
		downloads: make(map[string]chan struct{}, 4096),
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

func (s *Store) Get(ctx context.Context, rawURL string, host string) (statusCode int, header http.Header, r io.Reader, err error) {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	logger = logger.WithFieldKeyVals("rawURL", rawURL, "host", host)
	ctx = context.WithValue(ctx, "logger", logger)

	u, err := url.Parse(rawURL)
	if err != nil {
		err = fmt.Errorf("unable to parse raw url: %w", err)
		logger.Error(err)
		return
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

	data := &Data{
		Path: s.getDataPath(keyURL, u.Host),
	}

	logger = logger.WithFieldKeyVals("dataPath", data.Path)
	ctx = context.WithValue(ctx, "logger", logger)

	locker := s.namedLock.Locker(keyURL)
	locker.Lock()
	defer locker.Unlock()

	now := time.Now()

	s.downloadsMu.RLock()
	download := s.downloads[keyURL]
	s.downloadsMu.RUnlock()

	if download != nil {
		err = data.Open()
		if err != nil {
			logger.Error(err)
			return
		}
		return data.Info.StatusCode, data.Header.Clone(), s.pipeData(ctx, data, download), nil
	}

	ok, err := fsutil.IsExists(data.Path)
	if err != nil {
		err = fmt.Errorf("unable to check data path exists: %w", err)
		logger.Error(err)
		return
	}

	if ok {
		ok, err = fsutil.IsDir(data.Path)
		if err != nil {
			err = fmt.Errorf("unable to check data path is directory: %w", err)
			logger.Error(err)
			return
		}
		if !ok {
			err = errors.New("data path is not directory")
			logger.Error(err)
			return
		}
		err = data.Open()
		if err != nil {
			logger.Error(err)
			return
		}
		if data.Info.ExpiresAt.After(now) {
			return data.Info.StatusCode, data.Header.Clone(), s.pipeData(ctx, data, nil), nil
		}
		_ = data.Close()
		_ = os.RemoveAll(data.Path)
	}

	download, err = s.startDownload(ctx, u, host, keyURL, data.Path)
	if err != nil {
		return
	}
	data = &Data{
		Path: data.Path,
	}
	err = data.Open()
	if err != nil {
		logger.Error(err)
		return
	}
	return data.Info.StatusCode, data.Header.Clone(), s.pipeData(ctx, data, nil), nil
}

func (s *Store) getDataPath(rawURL string, subDir string) string {
	h := sha256.Sum256([]byte((rawURL)))
	result := s.config.Path + "/" + subDir
	for i, j := 0, len(h); i < j; i = i + 2 {
		result += fmt.Sprintf("%c%04x", '/', h[i:i+2])
	}
	return result
}

func (s *Store) pipeData(ctx context.Context, data *Data, download chan struct{}) io.Reader {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	pr, pw := io.Pipe()
	end := make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
			_ = pr.Close()
		case <-end:
		}
	}()

	go func() {
		var err error
		defer close(end)
		//goland:noinspection GoUnhandledErrorResult
		defer data.Close()
		//goland:noinspection GoUnhandledErrorResult
		defer pw.Close()
		for {
			_, err = io.Copy(pw, data.Body())
			if err != nil {
				switch err {
				case io.ErrClosedPipe:
				default:
					logger.Errorf("copy error: %w", err)
				}
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-download:
				_, err = io.Copy(pw, data.Body())
				if err != nil {
					switch err {
					case io.ErrClosedPipe:
					default:
						logger.Errorf("copy error: %w", err)
					}
					return
				}
				return
			case <-time.After(25 * time.Millisecond):
			}
		}
	}()

	return pr
}

func (s *Store) startDownload(ctx context.Context, u *url.URL, host string, keyURL string, dataPath string) (download chan struct{}, err error) {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	req := (&http.Request{
		Method: http.MethodGet,
		URL:    u,
		Header: http.Header{},
		Host:   host,
	}).WithContext(ctx)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request error: %w", err)
	}
	//goland:noinspection GoUnhandledErrorResult
	defer resp.Body.Close()

	now := time.Now()

	data := &Data{
		Path: dataPath,
	}
	data.Header = resp.Header.Clone()
	data.Info.StatusCode = resp.StatusCode
	data.Info.ContentLength = resp.ContentLength
	data.Info.CreatedAt = now
	data.Info.ExpiresAt = now.Add(s.config.MaxAge)

	hasCacheControl := false
	if h := resp.Header.Get("Cache-Control"); h != "" {
		hasCacheControl = true
		cc := httphdr.ParseCacheControl(h)
		maxAge := cc.MaxAge()
		if maxAge < 0 {
			maxAge = cc.SMaxAge()
		}
		switch {
		case cc.NoCache():
			data.Info.ExpiresAt = now
		case maxAge >= 0:
			expiresAt := now.Add(maxAge)
			if expiresAt.Sub(data.Info.ExpiresAt) < 0 {
				data.Info.ExpiresAt = expiresAt
			}
		}
	}
	if h := resp.Header.Get("Expires"); h != "" && !hasCacheControl {
		expiresAt, _ := time.Parse(time.RFC1123, h)
		if expiresAt.Sub(data.Info.ExpiresAt) < 0 {
			data.Info.ExpiresAt = expiresAt
		}
	}

	dynamic := (resp.StatusCode != http.StatusOK || resp.ContentLength < 0) && resp.StatusCode != http.StatusNotFound
	if dynamic {
		return nil, ErrDynamicContent
	}

	err = data.Create()
	if err != nil {
		logger.Error(err)
		return nil, err
	}

	download = make(chan struct{})

	s.downloadsMu.Lock()
	s.downloads[keyURL] = download
	s.downloadsMu.Unlock()

	go func() {
		//goland:noinspection GoUnhandledErrorResult
		defer data.Close()

		written, copyErr := ioutil.CopyRate(data.Body(), resp.Body, s.config.DownloadBurst, s.config.DownloadRate)
		_ = written

		_ = data.Close()
		close(download)

		locker := s.namedLock.Locker(keyURL)
		locker.Lock()
		defer locker.Unlock()

		s.downloadsMu.Lock()
		delete(s.downloads, keyURL)
		s.downloadsMu.Unlock()

		if copyErr != nil {
			_ = os.RemoveAll(data.Path)
		}
	}()

	return download, nil
}
