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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/goinsane/filelock"
	"github.com/goinsane/logng"
	"github.com/google/uuid"

	"github.com/orkunkaraduman/oscdn/fileutil"
	"github.com/orkunkaraduman/oscdn/ioutil"
	"github.com/orkunkaraduman/oscdn/namedlock"
)

type Store struct {
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	releaseOnce sync.Once

	config      Config
	httpClient  *http.Client
	lockPath    string
	contentPath string
	trashPath   string
	hostLock    *namedlock.NamedLock
	dataLock    *namedlock.NamedLock
	downloads   map[string]chan struct{}
	downloadsMu sync.RWMutex

	lockFile *filelock.File
}

func New(config Config) (result *Store, err error) {
	if config.Path == "" {
		config.Path = "."
	}
	config.Path, err = filepath.Abs(config.Path)
	if err != nil {
		return nil, fmt.Errorf("unable to get absolute store path: %w", err)
	}

	if config.UserAgent == "" {
		config.UserAgent = "oscdn"
	}

	ctx, cancel := context.WithCancel(context.Background())
	ctx = context.WithValue(ctx, "logger", config.Logger)
	s := &Store{
		ctx:    ctx,
		cancel: cancel,
		config: config,
		httpClient: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: func(ctx context.Context, network, addr string) (conn net.Conn, err error) {
					conn, err = (&net.Dialer{
						Timeout: 3 * time.Second,
					}).DialContext(ctx, network, addr)
					if err != nil {
						return nil, err
					}
					tcpConn := conn.(*net.TCPConn)
					_ = tcpConn.SetLinger(-1)
					_ = tcpConn.SetReadBuffer(128 * 1024)
					_ = tcpConn.SetWriteBuffer(128 * 1024)
					return
				},
				TLSClientConfig:        config.TLSConfig.Clone(),
				TLSHandshakeTimeout:    3 * time.Second,
				MaxIdleConns:           config.MaxIdleConns,
				IdleConnTimeout:        65 * time.Second,
				ResponseHeaderTimeout:  5 * time.Second,
				ExpectContinueTimeout:  1 * time.Second,
				MaxResponseHeaderBytes: 1024 * 1024,
				WriteBufferSize:        128 * 1024,
				ReadBufferSize:         128 * 1024,
				ForceAttemptHTTP2:      true,
			},
		},
		lockPath:    filepath.Join(config.Path, "lock"),
		contentPath: filepath.Join(config.Path, "content"),
		trashPath:   filepath.Join(config.Path, "trash"),
		hostLock:    namedlock.New(),
		dataLock:    namedlock.New(),
		downloads:   make(map[string]chan struct{}, 4096),
	}

	s.lockFile, err = filelock.Create(s.lockPath, 0666)
	if err != nil {
		return nil, fmt.Errorf("unable to get store lock: %w", err)
	}
	defer func(lockFile *filelock.File) {
		if err != nil {
			_ = lockFile.Release()
		}
	}(s.lockFile)
	err = s.lockFile.Truncate(0)
	if err != nil {
		return nil, fmt.Errorf("unable to truncate store lock: %w", err)
	}

	err = os.Mkdir(s.contentPath, 0777)
	if err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("unable to create content directory: %w", err)
	}
	err = os.Mkdir(s.trashPath, 0777)
	if err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("unable to create trash directory: %w", err)
	}

	s.wg.Add(1)
	go s.contentCleaner()

	s.wg.Add(1)
	go s.trashCleaner()

	return s, nil
}

func (s *Store) Release() (err error) {
	s.releaseOnce.Do(func() {
		s.cancel()
		s.wg.Wait()
		if e := s.lockFile.Release(); e != nil && err == nil {
			err = fmt.Errorf("unable to release store lock: %w", e)
		}
	})
	return
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
	if !hostRgx.MatchString(baseHost) {
		err = fmt.Errorf("invalid base host %q", baseHost)
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
	if !hostRgx.MatchString(keyHost) {
		err = fmt.Errorf("invalid key host %q", keyHost)
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
	result := filepath.Join(s.contentPath, baseURL.Host)
	h := sha256.Sum256([]byte((keyURL.String())))
	for i, j := 0, len(h); i < j; i = i + 2 {
		result += fmt.Sprintf("%c%04x", os.PathSeparator, h[i:i+2])
	}
	result = filepath.Join(result, "data")
	return result
}

func (s *Store) Get(ctx context.Context, rawURL string, host string, contentRange *ContentRange) (result GetResult, err error) {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	logger = logger.WithFieldKeyVals("rawURL", rawURL, "host", host)
	ctx = context.WithValue(ctx, "logger", logger)

	select {
	case <-s.ctx.Done():
		err = ErrStoreReleased
		logger.Error(err)
		return
	default:
	}

	var contentRangeNew *ContentRange
	if contentRange != nil {
		contentRangeNew = &(*contentRange)
	}

	result.ReadCloser = io.NopCloser(&ioutil.ErrorReader{Err: io.EOF})

	baseURL, keyURL, err := s.getURLs(rawURL, host)
	if err != nil {
		logger.Error(err)
		return
	}
	keyRawURL := keyURL.String()

	result.BaseURL = baseURL
	result.KeyURL = keyURL

	data := &Data{
		Path: s.getDataPath(baseURL, keyURL),
	}

	logger = logger.WithFieldKeyVals("dataPath", data.Path)
	ctx = context.WithValue(ctx, "logger", logger)

	hostLocker := s.hostLock.Locker(baseURL.Host)
	hostLocker.RLock()
	defer hostLocker.RUnlock()
	dataLocker := s.dataLock.Locker(data.Path)
	dataLocker.Lock()
	defer dataLocker.Unlock()

	s.downloadsMu.RLock()
	download := s.downloads[keyRawURL]
	s.downloadsMu.RUnlock()

	now := time.Now()

	exists, err := fileutil.IsExists(data.Path)
	if err != nil {
		err = fmt.Errorf("unable to check data is exists: %w", err)
		logger.Error(err)
		return
	}

	if download != nil {
		if !exists {
			s.downloadsMu.Lock()
			delete(s.downloads, keyRawURL)
			s.downloadsMu.Unlock()
		} else {
			err = data.Open()
			if err != nil {
				logger.Error(err)
				return
			}
			result.ReadCloser = s.pipeData(ctx, data, nil, download)
			result.CacheStatus = CacheStatusUpdating
			result.StatusCode = data.Info.StatusCode
			result.Header = data.Header.Clone()
			result.CreatedAt = data.Info.CreatedAt
			result.ExpiresAt = data.Info.ExpiresAt
			result.Size = data.Info.Size
			return
		}
	}

	if exists {
		var ok bool
		ok, err = fileutil.IsDir(data.Path)
		if err != nil {
			err = fmt.Errorf("unable to check data is directory: %w", err)
			logger.Error(err)
			return
		}
		if !ok {
			err = errors.New("data is not directory")
			logger.Error(err)
			return
		}
		err = data.Open()
		if err != nil {
			logger.Error(err)
			return
		}
		if data.Info.ExpiresAt.After(now) {
			result.ReadCloser = s.pipeData(ctx, data, contentRangeNew, nil)
			result.CacheStatus = CacheStatusHit
			result.StatusCode = data.Info.StatusCode
			result.Header = data.Header.Clone()
			result.CreatedAt = data.Info.CreatedAt
			result.ExpiresAt = data.Info.ExpiresAt
			result.Size = data.Info.Size
			result.ContentRange = contentRangeNew
			return
		}
		_ = data.Close()
	}

	download, err = s.startDownload(ctx, baseURL, keyURL)
	if err != nil {
		switch e := err.(type) {
		case *RequestError:
			if !exists {
				break
			}
			err = data.Open()
			if err != nil {
				logger.Error(err)
				break
			}
			result.ReadCloser = s.pipeData(ctx, data, contentRangeNew, nil)
			result.CacheStatus = CacheStatusStale
			result.StatusCode = data.Info.StatusCode
			result.Header = data.Header.Clone()
			result.CreatedAt = data.Info.CreatedAt
			result.ExpiresAt = data.Info.ExpiresAt
			result.Size = data.Info.Size
			result.ContentRange = contentRangeNew
			err = nil
		case *DynamicContentError:
			result.CacheStatus = CacheStatusDynamic
			result.StatusCode = e.resp.StatusCode
			result.Header = e.resp.Header.Clone()
			result.CreatedAt = data.Info.CreatedAt
			result.ExpiresAt = data.Info.ExpiresAt
			result.Size = -1
		case *SizeExceededError:
			result.StatusCode = data.Info.StatusCode
			result.Header = data.Header.Clone()
			result.CreatedAt = data.Info.CreatedAt
			result.ExpiresAt = data.Info.ExpiresAt
			result.Size = data.Info.Size
		}
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
	result.ReadCloser = s.pipeData(ctx, data, nil, download)
	if !exists {
		result.CacheStatus = CacheStatusMiss
	} else {
		result.CacheStatus = CacheStatusExpired
	}
	result.StatusCode = data.Info.StatusCode
	result.Header = data.Header.Clone()
	result.CreatedAt = data.Info.CreatedAt
	result.ExpiresAt = data.Info.ExpiresAt
	result.Size = data.Info.Size
	return
}

func (s *Store) pipeData(ctx context.Context, data *Data, contentRange *ContentRange, download chan struct{}) io.ReadCloser {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	if contentRange != nil {
		if download != nil {
			panic("download non-nil")
		}
		if contentRange.Start < 0 {
			contentRange.Start = 0
		}
		if contentRange.Start > data.Info.Size {
			contentRange.Start = data.Info.Size
		}
		if contentRange.End < 0 {
			contentRange.End = data.Info.Size
		} else {
			if contentRange.End > data.Info.Size {
				contentRange.End = data.Info.Size
			}
			if contentRange.End < contentRange.Start {
				contentRange.End = contentRange.Start
			}
		}
	}

	pr, pw := io.Pipe()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		var err error

		defer func(data *Data) {
			_ = data.Close()
		}(data)

		end := make(chan struct{})
		defer close(end)
		go func() {
			select {
			case <-s.ctx.Done():
				_ = pw.CloseWithError(ErrStoreReleased)
			case <-end:
				_ = pw.Close()
			}
		}()

		f := data.Body()
		r := io.Reader(f)
		if contentRange != nil {
			if contentRange.Start > 0 {
				_, err = f.Seek(contentRange.Start, io.SeekStart)
				if err != nil {
					logger.Errorf("seek error: %w", err)
					return
				}
			}
			if contentRange.End >= 0 {
				r = io.LimitReader(f, contentRange.End-contentRange.Start)
			}
		}

		for {
			_, err = io.Copy(pw, r)
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
			case <-s.ctx.Done():
				return
			case <-download:
				_, err = io.Copy(pw, r)
				if err != nil {
					switch err {
					case io.ErrClosedPipe:
					default:
						logger.Errorf("copy error: %w", err)
					}
					return
				}
				return
			case <-time.After(time.Second / 16):
			}
		}
	}()

	return &struct{ io.ReadCloser }{pr}
}

func (s *Store) startDownload(ctx context.Context, baseURL, keyURL *url.URL) (download chan struct{}, err error) {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	baseRawURL := baseURL.String()
	keyRawURL := keyURL.String()

	reqCtx, reqCtxCancel := context.WithCancel(s.ctx)
	defer func(reqCtxCancel context.CancelFunc) {
		if err != nil {
			reqCtxCancel()
		}
	}(reqCtxCancel)
	req := (&http.Request{
		Method: http.MethodGet,
		URL:    baseURL,
		Header: http.Header{
			"User-Agent": []string{s.config.UserAgent},
		},
		Host: keyURL.Host,
	}).WithContext(reqCtx)

	requested := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			reqCtxCancel()
		case <-requested:
		}
	}()
	resp, err := s.httpClient.Do(req)
	close(requested)
	if err != nil {
		err = &RequestError{error: err}
		logger.V(1).Error(err)
		return nil, err
	}
	defer func(body io.ReadCloser) {
		if err != nil {
			_ = body.Close()
		}
	}(resp.Body)

	now := time.Now()

	hostConfig := s.config.DefaultHostConfig
	if s.config.GetHostConfig != nil {
		hostConfig1 := s.config.GetHostConfig(baseURL.Scheme, baseURL.Host)
		if hostConfig1 != nil {
			hostConfig = hostConfig1
		}
	}
	if hostConfig == nil {
		hostConfig = new(HostConfig)
	}

	data := &Data{
		Path: s.getDataPath(baseURL, keyURL),
	}
	data.Header = resp.Header.Clone()
	data.Info.BaseURL = baseRawURL
	data.Info.KeyURL = keyRawURL
	data.Info.StatusCode = resp.StatusCode
	data.Info.Size = resp.ContentLength
	data.Info.CreatedAt = now
	data.Info.ExpiresAt = now

	err = s.moveToTrash(data.Path)
	if err != nil && !os.IsNotExist(err) {
		err = fmt.Errorf("unable to move expired data to trash: %w", err)
		logger.Error(err)
		return nil, err
	}

	if hostConfig.MaxSize > 0 && data.Info.Size > hostConfig.MaxSize {
		err = &SizeExceededError{Size: data.Info.Size}
		logger.V(1).Error(err)
		return nil, err
	}

	if hostConfig.MaxAge > 0 {
		data.Info.ExpiresAt = now.Add(hostConfig.MaxAge)
	}
	if hostConfig.MaxAge404 > 0 && resp.StatusCode == http.StatusNotFound {
		data.Info.ExpiresAt = now.Add(hostConfig.MaxAge404)
	}

	if !hostConfig.MaxAgeOverride {
		if expires := httpExpires(resp.Header, now); !expires.IsZero() && !expires.After(data.Info.ExpiresAt) {
			data.Info.ExpiresAt = expires
		}
	}

	if s.config.MinContentAge > 0 {
		minExpires := now.Add(s.config.MinContentAge)
		if data.Info.ExpiresAt.Before(minExpires) {
			data.Info.ExpiresAt = minExpires
		}
	}

	dynamic := ((resp.StatusCode != http.StatusOK || resp.ContentLength < 0) && resp.StatusCode != http.StatusNotFound) ||
		!data.Info.ExpiresAt.After(now)
	if dynamic {
		err = &DynamicContentError{resp: resp}
		logger.V(1).Error(err)
		return nil, err
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

		var err error

		defer reqCtxCancel()

		defer func(body io.ReadCloser) {
			_ = body.Close()
		}(resp.Body)

		hash := sha256.New()

		written, err := ioutil.CopyRate(io.MultiWriter(data.Body(), hash), io.LimitReader(resp.Body, data.Info.Size), hostConfig.DownloadBurst, hostConfig.DownloadRate)
		if err != nil {
			err = fmt.Errorf("content download error: %w", err)
			logger.V(1).Error(err)
		}

		if e := data.Close(); e != nil {
			logger.Error(e)
			if err == nil {
				err = e
			}
		}
		close(download)

		hostLocker := s.hostLock.Locker(baseURL.Host)
		hostLocker.RLock()
		defer hostLocker.RUnlock()
		dataLocker := s.dataLock.Locker(data.Path)
		dataLocker.Lock()
		defer dataLocker.Unlock()

		s.downloadsMu.RLock()
		downloadNew := s.downloads[keyRawURL]
		s.downloadsMu.RUnlock()

		if download == downloadNew {
			s.downloadsMu.Lock()
			delete(s.downloads, keyRawURL)
			s.downloadsMu.Unlock()
		}

		if data.Info.Size >= 0 && written != data.Info.Size {
			e := errors.New("different content size")
			logger.V(1).Error(e)
			if err == nil {
				err = e
			}
		}

		if err != nil && download == downloadNew {
			err = s.moveToTrash(data.Path)
			if err != nil {
				if !os.IsNotExist(err) {
					err = fmt.Errorf("unable to move incomplete data to trash: %w", err)
					logger.Error(err)
				}
			}
		}
	}()

	return download, nil
}

func (s *Store) moveToTrash(sourcePath string) (err error) {
	targetPath := filepath.Join(s.trashPath, uuid.NewString())
	err = os.Rename(sourcePath, targetPath)
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) Purge(ctx context.Context, rawURL string, host string) (err error) {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	logger = logger.WithFieldKeyVals("rawURL", rawURL, "host", host)
	ctx = context.WithValue(ctx, "logger", logger)

	select {
	case <-s.ctx.Done():
		err = ErrStoreReleased
		logger.Error(err)
		return
	default:
	}

	baseURL, keyURL, err := s.getURLs(rawURL, host)
	if err != nil {
		logger.Error(err)
		return
	}

	dataPath := s.getDataPath(baseURL, keyURL)

	logger = logger.WithFieldKeyVals("dataPath", dataPath)
	ctx = context.WithValue(ctx, "logger", logger)

	hostLocker := s.hostLock.Locker(baseURL.Host)
	hostLocker.RLock()
	defer hostLocker.RUnlock()
	dataLocker := s.dataLock.Locker(dataPath)
	dataLocker.Lock()
	defer dataLocker.Unlock()

	ok, err := fileutil.IsExists(dataPath)
	if err != nil {
		err = fmt.Errorf("unable to check data is exists: %w", err)
		logger.Error(err)
		return
	}
	if !ok {
		return ErrNotExists
	}

	err = s.moveToTrash(dataPath)
	if err != nil {
		err = fmt.Errorf("unable to move purging data to trash: %w", err)
		logger.Error(err)
		return
	}

	return nil
}

func (s *Store) PurgeHost(ctx context.Context, host string) (err error) {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	logger = logger.WithFieldKeyVals("host", host)
	ctx = context.WithValue(ctx, "logger", logger)

	select {
	case <-s.ctx.Done():
		err = ErrStoreReleased
		logger.Error(err)
		return
	default:
	}

	if host == "" || !hostRgx.MatchString(host) {
		err = errors.New("invalid host")
		return
	}

	hostPath := filepath.Join(s.contentPath, host)

	logger = logger.WithFieldKeyVals("hostPath", hostPath)
	ctx = context.WithValue(ctx, "logger", logger)

	hostLocker := s.hostLock.Locker(host)
	hostLocker.Lock()
	defer hostLocker.Unlock()

	ok, err := fileutil.IsExists(hostPath)
	if err != nil {
		err = fmt.Errorf("unable to check host is exists: %w", err)
		logger.Error(err)
		return
	}
	if !ok {
		return ErrNotExists
	}

	err = s.moveToTrash(hostPath)
	if err != nil {
		err = fmt.Errorf("unable to move host to trash: %w", err)
		logger.Error(err)
		return
	}

	return nil
}

func (s *Store) PurgeAll(ctx context.Context) (err error) {
	logger, _ := ctx.Value("logger").(*logng.Logger)

	select {
	case <-s.ctx.Done():
		err = ErrStoreReleased
		logger.Error(err)
		return
	default:
	}

	dirs, err := os.ReadDir(s.contentPath)
	if err != nil {
		err = fmt.Errorf("unable to read content directory: %w", err)
		logger.Error(err)
		return
	}

	for _, dir := range dirs {
		err = ctx.Err()
		if err != nil {
			break
		}

		if !dir.IsDir() {
			continue
		}

		host := dir.Name()

		if host == "" || !hostRgx.MatchString(host) {
			continue
		}

		err = s.PurgeHost(ctx, host)
		if err != nil {
			if err == ErrNotExists {
				continue
			}
			break
		}
	}

	return
}

func (s *Store) contentCleaner() {
	defer s.wg.Done()

	var err error
	ctx := s.ctx
	logger, _ := ctx.Value("logger").(*logng.Logger)

	for ctx.Err() == nil {
		if e := fileutil.WalkDir(s.contentPath, func(subContentPath string, dirEntry fs.DirEntry) bool {
			err = ctx.Err()
			if err != nil {
				return false
			}

			if !dirEntry.IsDir() {
				return true
			}

			if !strings.HasSuffix(subContentPath, fmt.Sprintf("%cdata", os.PathSeparator)) {
				logger := logger.WithFieldKeyVals("subContentPath", subContentPath)
				err = os.Remove(subContentPath)
				if err != nil {
					if fileutil.IsNotEmpty(err) {
						err = nil
						return true
					}
					err = fmt.Errorf("unable to remove sub content directory: %w", err)
					logger.Error(err)
					return false
				}
				return true
			}

			data := &Data{
				Path: subContentPath,
			}

			logger := logger.WithFieldKeyVals("dataPath", data.Path)

			host := strings.TrimPrefix(subContentPath, s.contentPath)
			if idx := strings.Index(host, string(os.PathSeparator)); idx >= 0 {
				host = host[:idx]
			}

			hostLocker := s.hostLock.Locker(host)
			hostLocker.RLock()
			defer hostLocker.RUnlock()
			dataLocker := s.dataLock.Locker(data.Path)
			dataLocker.Lock()
			defer dataLocker.Unlock()

			var ok bool
			ok, err = fileutil.IsExists(data.Path)
			if err != nil {
				err = fmt.Errorf("unable to check data is exists: %w", err)
				logger.Error(err)
				return false
			}
			if !ok {
				return true
			}

			ok, err = fileutil.IsDir(data.Path)
			if err != nil {
				err = fmt.Errorf("unable to check data is directory: %w", err)
				logger.Error(err)
				return false
			}
			if !ok {
				err = errors.New("data is not directory")
				logger.Error(err)
				err = os.Remove(data.Path)
				if err != nil {
					err = fmt.Errorf("unable to remove data: %w", err)
					logger.Error(err)
					return false
				}
				return true
			}
			err = data.Open()
			if err != nil {
				logger.Error(err)
				return false
			}
			defer func(data *Data) {
				_ = data.Close()
			}(data)
			if !data.Info.ExpiresAt.After(time.Now()) {
				err = s.moveToTrash(data.Path)
				if err != nil {
					err = fmt.Errorf("unable to move data to trash: %w", err)
					logger.Error(err)
					return false
				}
			}
			return true
		}); e != nil {
			e = fmt.Errorf("unable to walk content directories: %w", e)
			logger.Error(e)
		}
		if err != nil {
			err = nil
		}

		select {
		case <-ctx.Done():
		case <-time.After(time.Second / 4):
		}
	}
}

func (s *Store) trashCleaner() {
	defer s.wg.Done()

	var err error
	ctx := s.ctx
	logger, _ := ctx.Value("logger").(*logng.Logger)

	for ctx.Err() == nil {
		if e := fileutil.WalkDir(s.trashPath, func(subTrashPath string, dirEntry fs.DirEntry) bool {
			err = ctx.Err()
			if err != nil {
				return false
			}

			logger := logger.WithFieldKeyVals("subTrashPath", subTrashPath)

			err = os.Remove(subTrashPath)
			if err != nil {
				if dirEntry.IsDir() && fileutil.IsNotEmpty(err) {
					err = nil
					return true
				}
				err = fmt.Errorf("unable to remove sub trash directory: %w", err)
				logger.Error(err)
				return false
			}
			return true
		}); e != nil {
			e = fmt.Errorf("unable to walk trash directories: %w", e)
			logger.Error(e)
		}
		if err != nil {
			err = nil
		}

		select {
		case <-ctx.Done():
		case <-time.After(time.Second / 4):
		}
	}
}
