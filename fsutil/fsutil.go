package fsutil

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

func ToOSPath(p string) string {
	return filepath.FromSlash(path.Clean(p))
}

func IsExists(name string) (exists bool, err error) {
	f, err := os.Open(ToOSPath(name))
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
	f, err := os.Open(ToOSPath(name))
	if err != nil {
		return false, err
	}
	//goland:noinspection GoUnhandledErrorResult
	defer f.Close()
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

func walkDir(fsys fs.FS, name string, fn WalkDirFunc) (cont bool, err error) {
	dirs, err := fs.ReadDir(fsys, name)
	if err != nil {
		return false, err
	}

	for _, dir := range dirs {
		newName := path.Join(name, dir.Name())
		if !dir.IsDir() {
			if !fn(newName, dir) {
				break
			}
			continue
		}
		if !fn(newName, dir) {
			break
		}
		var cont1 bool
		cont1, err = walkDir(fsys, newName, fn)
		if err != nil {
			return false, err
		}
		if !cont1 {
			break
		}
	}

	return false, nil
}

func WalkDir(fsys fs.FS, root string, fn WalkDirFunc) (err error) {
	info, err := fs.Stat(fsys, root)
	if err != nil {
		return err
	}
	d := fs.FileInfoToDirEntry(info)
	if !fn(root, d) || !d.IsDir() {
		return nil
	}
	_, err = walkDir(fsys, d.Name(), fn)
	return err
}

type WalkDirFunc func(name string, d fs.DirEntry) (cont bool)
