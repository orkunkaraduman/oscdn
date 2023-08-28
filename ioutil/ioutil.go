package ioutil

import (
	"io"
	"time"

	"github.com/goinsane/ioshape"
)

func CopyRate(dst io.Writer, src io.Reader, burst, rate int64) (written int64, err error) {
	// burst
	if burst > 0 {
		n, e := io.CopyN(dst, src, burst*1024)
		written += n
		if e != nil {
			return written, e
		}
	}
	// shaping
	if rate > 0 {
		const k = 64
		period := time.Second / k
		leakSize := rate * 1024 / k
		rd := ioshape.NewLeakyBucket(src, period, leakSize, 128*1024)
		defer rd.Stop()
		n, e := io.Copy(dst, rd)
		written += n
		if e != nil {
			return written, e
		}
		return written, nil
	}
	// without shaping
	n, e := io.Copy(dst, src)
	written += n
	if e != nil {
		return written, e
	}
	return written, nil
}

func NopWriteCloser(w io.Writer) io.WriteCloser {
	if _, ok := w.(io.ReaderFrom); ok {
		return nopWriteCloserReaderFrom{w}
	}
	return nopWriteCloser{w}
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

type nopWriteCloserReaderFrom struct {
	io.Writer
}

func (nopWriteCloserReaderFrom) Close() error { return nil }

func (c nopWriteCloserReaderFrom) WriteTo(r io.Reader) (n int64, err error) {
	return c.Writer.(io.ReaderFrom).ReadFrom(r)
}

type ErrorReader struct {
	Err error
}

func (r *ErrorReader) Read(p []byte) (n int, err error) {
	return 0, r.Err
}
