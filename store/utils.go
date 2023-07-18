package store

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"syscall"
)

func isNotEmpty(err error) bool {
	if e := new(os.PathError); errors.As(err, &e) {
		if e.Err == syscall.ENOTEMPTY {
			return true
		}
	}
	return false
}

func walkDir(root string, fn func(p string, d fs.DirEntry) bool) error {
	return fs.WalkDir(os.DirFS(root), ".", func(p string, d fs.DirEntry, e error) error {
		if p == "." {
			return e
		}
		if e != nil {
			if d.IsDir() && os.IsNotExist(e) {
				return fs.SkipDir
			}
			return e
		}
		if fn(path.Join(root, p), d) {
			return nil
		}
		return fs.SkipAll
	})
}
