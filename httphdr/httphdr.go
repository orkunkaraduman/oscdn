package httphdr

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
)

func PutHTTPHeaders(dst http.Header, src http.Header, keys ...string) {
	src = src.Clone()
	if len(keys) <= 0 {
		for key := range src {
			dst[key] = src[key]
		}
		return
	}
	for _, key := range keys {
		dst[key] = src[key]
	}
}

func SizeOfHTTPHeader(header http.Header) int {
	buf := bytes.NewBuffer(nil)
	_ = header.Write(buf)
	return buf.Len()
}

type Option struct {
	Name       string
	Parameters map[string]string
}

func ParseOptions(directive string) []*Option {
	result := make([]*Option, 0, 128)
	var option *Option
	matches := optionRgx.FindAllString(directive, -1)
	for _, match := range matches {
		if strings.HasSuffix(match, ";") {
			if option != nil {
				result = append(result, option)
			}
			option = &Option{
				Name:       strings.ToLower(strings.TrimSpace(strings.TrimSuffix(match, ";"))),
				Parameters: make(map[string]string),
			}
			continue
		}
		if option == nil {
			option = &Option{
				Name:       "",
				Parameters: make(map[string]string),
			}
		}
		var key, value string
		key = match
		if index := strings.Index(match, "="); index != -1 {
			key, value = match[:index], match[index+1:]
		}
		option.Parameters[strings.ToLower(key)] = strings.TrimSpace(value)
	}
	if option != nil {
		result = append(result, option)
	}
	return result
}

var optionRgx = regexp.MustCompile(`([a-zA-Z][a-zA-Z_\-]*(?:\s*;)?)(?:=(?:"([^"]*)"|([^ \t",;]*)))?`)
