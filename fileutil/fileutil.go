package fileutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

func IsExists(name string) (exists bool, err error) {
	f, err := os.Open(name)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return false, err
	}
	_ = f.Close()
	return true, nil
}

func IsExists2(name string) (exists bool) {
	exists, _ = IsExists(name)
	return
}

func IsDir(name string) (ok bool, err error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	fi, err := f.Stat()
	if err != nil {
		return false, err
	}
	return fi.IsDir(), nil
}

func IsDir2(name string) (ok bool) {
	ok, _ = IsDir(name)
	return
}

func IsNotEmpty(err error) bool {
	if e := new(os.PathError); errors.As(err, &e) {
		if e.Err == syscall.ENOTEMPTY {
			return true
		}
	}
	return false
}

func WalkDir(root string, fn func(p string, d fs.DirEntry) bool) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if p == root {
			return e
		}
		if e != nil {
			if d != nil && d.IsDir() && os.IsNotExist(e) {
				return fs.SkipDir
			}
			return e
		}
		if fn(p, d) {
			return nil
		}
		return fs.SkipAll
	})
}
