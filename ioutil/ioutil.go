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
		const k = 16
		period := time.Second / k
		leakSize := rate * 1024 / k
		rd := ioshape.NewLeakyBucket(src, period, leakSize, 4*leakSize)
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
