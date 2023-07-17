package httphdr

import (
	"regexp"
	"strings"
)

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
