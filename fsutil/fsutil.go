package fsutil

import "os"

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
