package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/goinsane/filelock"
	"github.com/goinsane/logng"
	"github.com/goinsane/xcontext"
	"github.com/google/uuid"

	"github.com/orkunkaraduman/oscdn/fsutil"
	"github.com/orkunkaraduman/oscdn/httphdr"
	"github.com/orkunkaraduman/oscdn/ioutil"
	"github.com/orkunkaraduman/oscdn/namedlock"
)

type Store struct {
	ctx         xcontext.CancelableContext
	wg          sync.WaitGroup
	releaseOnce sync.Once

	config      Config
	httpClient  *http.Client
	dataLock    *namedlock.NamedLock
	downloads   map[string]chan struct{}
	downloadsMu sync.RWMutex

	lockFile *filelock.File
}

func New(config Config) (result *Store, err error) {
	s := &Store{
		ctx:    xcontext.WithCancelable2(context.Background()),
		config: config,
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
		dataLock:  namedlock.New(),
		downloads: make(map[string]chan struct{}, 4096),
	}

	if s.config.Path == "" {
		s.config.Path = "."
	}
	if s.config.UserAgent == "" {
		s.config.UserAgent = "oscdn"
	}
	s.lockFile, err = filelock.Create(fsutil.ToOSPath(s.config.Path+"/lock"), 0666)
	if err != nil {
		return nil, fmt.Errorf("unable to get store lock: %w", err)
	}
	defer func() {
		if err != nil {
			_ = s.lockFile.Release()
		}
	}()

	err = os.Mkdir(fsutil.ToOSPath(fmt.Sprintf("%s/purged", s.config.Path)), 0777)
	if err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("unable to create purged directory: %w", err)
	}

	return s, nil
}

func (s *Store) Release() (err error) {
	s.releaseOnce.Do(func() {
		s.ctx.Cancel()
		s.wg.Wait()
		if e := s.lockFile.Release(); e != nil && err == nil {
			err = fmt.Errorf("unable to release store lock: %w", e)
		}
	})
	return
}

func (s *Store) Get(ctx context.Context, rawURL string, host string) (result GetResult, err error) {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	select {
	case <-s.ctx.Done():
		err = ErrReleased
		return
	default:
	}

	logger = logger.WithFieldKeyVals("rawURL", rawURL, "host", host)
	ctx = context.WithValue(ctx, "logger", logger)

	result.ReadCloser = io.NopCloser(&ioutil.NopReader{Err: io.EOF})

	baseURL, keyURL, err := s.getURLs(rawURL, host)
	if err != nil {
		logger.Error(err)
		return
	}
	keyRawURL := keyURL.String()

	data := &Data{
		Path: s.getDataPath(baseURL, keyURL),
	}

	logger = logger.WithFieldKeyVals("dataPath", data.Path)
	ctx = context.WithValue(ctx, "logger", logger)

	locker := s.dataLock.Locker(data.Path)
	locker.Lock()
	defer locker.Unlock()

	s.downloadsMu.RLock()
	download := s.downloads[keyRawURL]
	s.downloadsMu.RUnlock()

	now := time.Now()

	if download != nil {
		err = data.Open()
		if err != nil {
			logger.Error(err)
			return
		}
		return GetResult{
			ReadCloser: s.pipeData(ctx, data, download),
			BaseURL:    baseURL,
			KeyURL:     keyURL,
			StatusCode: data.Info.StatusCode,
			Header:     data.Header.Clone(),
		}, nil
	}

	ok, err := fsutil.IsExists(data.Path)
	if err != nil {
		err = fmt.Errorf("unable to check data path is exists: %w", err)
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
			return GetResult{
				ReadCloser: s.pipeData(ctx, data, nil),
				BaseURL:    baseURL,
				KeyURL:     keyURL,
				StatusCode: data.Info.StatusCode,
				Header:     data.Header.Clone(),
			}, nil
		}
		_ = data.Close()
		_ = os.RemoveAll(fsutil.ToOSPath(data.Path))
	}

	download, err = s.startDownload(ctx, baseURL, keyURL)
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
	return GetResult{
		ReadCloser: s.pipeData(ctx, data, download),
		BaseURL:    baseURL,
		KeyURL:     keyURL,
		StatusCode: data.Info.StatusCode,
		Header:     data.Header.Clone(),
	}, nil
}

func (s *Store) Purge(ctx context.Context, rawURL string, host string) (err error) {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	select {
	case <-s.ctx.Done():
		err = ErrReleased
		return
	default:
	}

	logger = logger.WithFieldKeyVals("rawURL", rawURL, "host", host)
	ctx = context.WithValue(ctx, "logger", logger)

	baseURL, keyURL, err := s.getURLs(rawURL, host)
	if err != nil {
		logger.Error(err)
		return
	}

	dataPath := s.getDataPath(baseURL, keyURL)

	logger = logger.WithFieldKeyVals("dataPath", dataPath)
	ctx = context.WithValue(ctx, "logger", logger)

	return s.purge(ctx, dataPath)
}

func (s *Store) PurgeHost(ctx context.Context, host string) (err error) {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	select {
	case <-s.ctx.Done():
		err = ErrReleased
		return
	default:
	}

	logger = logger.WithFieldKeyVals("host", host)
	ctx = context.WithValue(ctx, "logger", logger)

	if !HostRgx.MatchString(host) {
		err = errors.New("invalid host")
		return
	}

	basePath := fmt.Sprintf("%s/content", s.config.Path)
	if e := fs.WalkDir(os.DirFS(basePath),
		host, func(p string, d fs.DirEntry, e error) error {
			if os.IsNotExist(e) || !d.IsDir() || !strings.HasSuffix(p, "/data") {
				return nil
			}

			err = ctx.Err()
			if err != nil {
				return fs.SkipAll
			}

			dataPath := fmt.Sprintf("%s/%s", basePath, p)

			logger := logger.WithFieldKeyVals("dataPath", dataPath)
			ctx := context.WithValue(ctx, "logger", logger)

			err = s.purge(ctx, dataPath)
			if err != nil {
				if err == ErrNotExists {
					logger.Error("data path not exists")
					return nil
				}
				return fs.SkipAll
			}

			return nil
		}); e != nil {
		err = fmt.Errorf("unable to walk content directories: %w", e)
		logger.Error(err)
		return err
	}
	if err != nil {
		return err
	}

	return nil
}

func (s *Store) purge(ctx context.Context, dataPath string) (err error) {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	locker := s.dataLock.Locker(dataPath)
	locker.Lock()
	defer locker.Unlock()

	ok, err := fsutil.IsExists(dataPath)
	if err != nil {
		err = fmt.Errorf("unable to check data path is exists: %w", err)
		logger.Error(err)
		return
	}
	if !ok {
		return ErrNotExists
	}

	purgedPath := fmt.Sprintf("%s/purged/%s", s.config.Path, uuid.NewString())
	err = os.Rename(fsutil.ToOSPath(dataPath), fsutil.ToOSPath(purgedPath))
	if err != nil {
		err = fmt.Errorf("unable to rename data path: %w", err)
		logger.Error(err)
		return
	}

	return nil
}

func (s *Store) getURLs(rawURL string, host string) (baseURL, keyURL *url.URL, err error) {
	baseURL, err = url.Parse(rawURL)
	if err != nil {
		err = fmt.Errorf("unable to parse raw url: %w", err)
		return
	}
	if (baseURL.Scheme != "http" && baseURL.Scheme != "https") ||
		baseURL.Opaque != "" ||
		baseURL.User != nil ||
		baseURL.Host == "" ||
		baseURL.Fragment != "" {
		err = errors.New("invalid raw url")
		return
	}

	firstScheme := baseURL.Scheme
	firstHost := baseURL.Host

	baseHost := firstHost
	if !HostRgx.MatchString(baseHost) {
		err = errors.New("invalid base host")
		return
	}
	switch firstScheme {
	case "http":
		baseHost = strings.TrimSuffix(baseHost, ":80")
	case "https":
		baseHost = strings.TrimSuffix(baseHost, ":443")
	}
	baseURL = &url.URL{
		Scheme:   firstScheme,
		Host:     baseHost,
		Path:     baseURL.Path,
		RawQuery: baseURL.RawQuery,
	}

	keyHost := firstHost
	if host != "" {
		keyHost = host
	}
	if !HostRgx.MatchString(keyHost) {
		err = errors.New("invalid key host")
		return
	}
	switch firstScheme {
	case "http":
		keyHost = strings.TrimSuffix(keyHost, ":80")
	case "https":
		keyHost = strings.TrimSuffix(keyHost, ":443")
	}
	keyURL = &url.URL{
		Scheme:   firstScheme,
		Host:     keyHost,
		Path:     baseURL.Path,
		RawQuery: baseURL.RawQuery,
	}

	return
}

func (s *Store) getDataPath(baseURL, keyURL *url.URL) string {
	result := fmt.Sprintf("%s/content/%s", s.config.Path, baseURL.Host)
	h := sha256.Sum256([]byte((keyURL.String())))
	for i, j := 0, len(h); i < j; i = i + 2 {
		result += fmt.Sprintf("%c%04x", '/', h[i:i+2])
	}
	result += "/data"
	return result
}

func (s *Store) pipeData(ctx context.Context, data *Data, download chan struct{}) io.ReadCloser {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	pr, pw := io.Pipe()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		var err error
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
			if download == nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-s.ctx.Done():
				_ = pw.CloseWithError(ErrReleased)
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

func (s *Store) startDownload(ctx context.Context, baseURL, keyURL *url.URL) (download chan struct{}, err error) {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	baseRawURL := baseURL.String()
	keyRawURL := keyURL.String()

	req := (&http.Request{
		Method: http.MethodGet,
		URL:    baseURL,
		Header: http.Header{
			"User-Agent": []string{s.config.UserAgent},
		},
		Host: keyURL.Host,
	}).WithContext(s.ctx)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request error: %w", err)
	}
	//goland:noinspection GoUnhandledErrorResult
	defer func() {
		if err != nil {
			_ = resp.Body.Close()
		}
	}()

	now := time.Now()

	data := &Data{
		Path: s.getDataPath(baseURL, keyURL),
	}
	data.Header = resp.Header.Clone()
	data.Info.BaseURL = baseRawURL
	data.Info.KeyURL = keyRawURL
	data.Info.StatusCode = resp.StatusCode
	data.Info.ContentLength = resp.ContentLength
	data.Info.CreatedAt = now
	data.Info.ExpiresAt = now.Add(s.config.MaxAge)

	expires := httphdr.Expires(resp.Header, now)
	if expires.Sub(data.Info.ExpiresAt) < 0 {
		data.Info.ExpiresAt = expires
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
	s.downloads[keyRawURL] = download
	s.downloadsMu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		_, copyErr := ioutil.CopyRate(data.Body(), resp.Body, s.config.DownloadBurst, s.config.DownloadRate)

		_ = data.Close()
		close(download)

		locker := s.dataLock.Locker(data.Path)
		locker.Lock()
		defer locker.Unlock()

		s.downloadsMu.Lock()
		delete(s.downloads, keyRawURL)
		s.downloadsMu.Unlock()

		if copyErr != nil {
			logger.V(2).Warning(err)
			_ = os.RemoveAll(fsutil.ToOSPath(data.Path))
		}
	}()

	return download, nil
}
