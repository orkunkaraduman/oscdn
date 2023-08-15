package cdn

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/orkunkaraduman/oscdn/httputil"
	"github.com/orkunkaraduman/oscdn/store"
)

func getContentRange(h http.Header) (result *store.ContentRange, err error) {
	r := h.Get("Range")
	if r == "" {
		return nil, nil
	}
	opts := httputil.ParseOptions(r)
	if len(opts) <= 0 {
		return nil, errors.New("no option")
	}
	b := opts[0].Map["bytes"]
	if b == "" {
		return nil, nil
	}
	ranges := strings.SplitN(b, "-", 2)
	result = &store.ContentRange{
		Start: 0,
		End:   -1,
	}
	if len(ranges) > 0 && ranges[0] != "" {
		result.Start, err = strconv.ParseInt(ranges[0], 10, 64)
		if err != nil {
			err = fmt.Errorf("unable to parse content range start: %w", err)
			return nil, err
		}
	}
	if len(ranges) > 1 && ranges[1] != "" {
		result.End, err = strconv.ParseInt(ranges[1], 10, 64)
		if err != nil {
			err = fmt.Errorf("unable to parse content range end: %w", err)
			return nil, err
		}
	}
	return result, nil
}

func maskStatusCode(code int) string {
	switch {
	case 100 <= code && code <= 199:
		switch code {
		default:
			return "1xx"
		}
	case 200 <= code && code <= 299:
		switch code {
		case http.StatusOK:
		case http.StatusPartialContent:
		default:
			return "2xx"
		}
	case 300 <= code && code <= 399:
		switch code {
		case http.StatusMovedPermanently:
		case http.StatusFound:
		default:
			return "3xx"
		}
	case 400 <= code && code <= 499:
		switch code {
		case http.StatusForbidden:
		case http.StatusNotFound:
		default:
			return "4xx"
		}
	case 500 <= code && code <= 599:
		switch code {
		case http.StatusInternalServerError:
		case http.StatusNotImplemented:
		case http.StatusBadGateway:
		case http.StatusServiceUnavailable:
		case http.StatusGatewayTimeout:
		default:
			return "5xx"
		}
	default:
		return "xxx"
	}
	return strconv.Itoa(code)
}
