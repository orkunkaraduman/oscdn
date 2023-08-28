package cdn

import (
	"net/http"
	"strconv"
)

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
