package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/orkunkaraduman/oscdn/fsutil"
)

type Data struct {
	Path string
	Info struct {
		BaseURL       string
		KeyURL        string
		StatusCode    int
		ContentLength int64
		CreatedAt     time.Time
		ExpiresAt     time.Time
	}
	Header http.Header

	initialized bool
	closeOnce   sync.Once

	infoFile   *os.File
	headerFile *os.File
	bodyFile   *os.File
}

func (d *Data) Create() (err error) {
	if d.initialized {
		panic("already initialized")
	}

	err = os.MkdirAll(fsutil.ToOSPath(d.Path), 0777)
	if err != nil {
		return fmt.Errorf("unable to create data directory: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(fsutil.ToOSPath(d.Path))
		}
	}()

	err = d.openFiles(true)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = d.closeFiles()
		}
	}()

	err = json.NewEncoder(d.infoFile).Encode(d.Info)
	if err != nil {
		return fmt.Errorf("unable to marshal to info file: %w", err)
	}
	err = d.infoFile.Sync()
	if err != nil {
		return fmt.Errorf("unable to sync info file: %w", err)
	}

	err = d.Header.Write(d.headerFile)
	if err != nil {
		return fmt.Errorf("unable to serialize to header file: %w", err)
	}
	err = d.headerFile.Sync()
	if err != nil {
		return fmt.Errorf("unable to sync header file: %w", err)
	}

	d.initialized = true
	return nil
}

func (d *Data) Open() (err error) {
	if d.initialized {
		panic("already initialized")
	}

	err = d.openFiles(false)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = d.closeFiles()
		}
	}()

	d.Info = (&Data{}).Info
	err = json.NewDecoder(d.infoFile).Decode(&d.Info)
	if err != nil {
		return fmt.Errorf("unable to unmarshal from info file: %w", err)
	}

	d.Header = make(http.Header, 4096)
	s := bufio.NewScanner(d.headerFile)
	for s.Scan() {
		text := s.Text()
		if text == "" {
			continue
		}
		idx := strings.Index(text, ":")
		if idx < 0 {
			d.Header.Add(text, "")
			continue
		}
		d.Header.Add(text[:idx], strings.TrimSpace(text[idx+1:]))
	}
	if err = s.Err(); err != nil {
		return fmt.Errorf("unable to scan header file: %w", err)
	}

	d.initialized = true
	return nil
}

func (d *Data) Close() (err error) {
	if !d.initialized {
		panic("not initialized")
	}
	d.closeOnce.Do(func() {
		err = d.closeFiles()
	})
	return
}

func (d *Data) Body() *os.File {
	return d.bodyFile
}

func (d *Data) openFiles(create bool) (err error) {
	defer func() {
		if err != nil {
			_ = d.closeFiles()
		}
	}()

	flag := os.O_RDONLY
	if create {
		flag = os.O_RDWR | os.O_CREATE | os.O_TRUNC
	}
	perm := os.FileMode(0666)

	d.infoFile, err = os.OpenFile(fsutil.ToOSPath(d.Path+"/info"), flag, perm)
	if err != nil {
		return fmt.Errorf("unable to open info file: %w", err)
	}

	d.headerFile, err = os.OpenFile(fsutil.ToOSPath(d.Path+"/header"), flag, perm)
	if err != nil {
		return fmt.Errorf("unable to open header file: %w", err)
	}

	d.bodyFile, err = os.OpenFile(fsutil.ToOSPath(d.Path+"/body"), flag, perm)
	if err != nil {
		return fmt.Errorf("unable to open body file: %w", err)
	}

	return nil
}

func (d *Data) closeFiles() (err error) {
	if d.infoFile != nil {
		if e := d.infoFile.Close(); e != nil && err == nil {
			err = fmt.Errorf("unable to close info file: %w", e)
		}
	}

	if d.headerFile != nil {
		if e := d.headerFile.Close(); e != nil && err == nil {
			err = fmt.Errorf("unable to close header file: %w", e)
		}
	}

	if d.bodyFile != nil {
		if e := d.bodyFile.Close(); e != nil && err == nil {
			err = fmt.Errorf("unable to close body file: %w", e)
		}
	}

	return
}
