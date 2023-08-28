package httputil

import (
	"strings"
)

type Option struct {
	KeyVals []OptionKeyVal
	Map     map[string]string
}

type OptionKeyVal struct {
	Key string
	Val string
}

func ParseOptions(directive string) (options []Option) {
	options = []Option{}

	for _, o := range strings.Split(directive, ",") {
		o = strings.TrimSpace(o)
		option := &Option{
			KeyVals: []OptionKeyVal{},
			Map:     map[string]string{},
		}
		for _, kv := range strings.Split(o, ";") {
			kv = strings.TrimSpace(kv)
			kvs := strings.SplitN(kv, "=", 2)
			optionKeyVal := &OptionKeyVal{
				Key: strings.TrimSpace(kvs[0]),
			}
			if optionKeyVal.Key == "" {
				continue
			}
			if len(kvs) > 1 {
				optionKeyVal.Val = strings.TrimSpace(kvs[1])
			}
			option.KeyVals = append(option.KeyVals, *optionKeyVal)
			if _, ok := option.Map[optionKeyVal.Key]; !ok {
				option.Map[optionKeyVal.Key] = optionKeyVal.Val
			}
		}
		if len(option.KeyVals) <= 0 {
			continue
		}
		options = append(options, *option)
	}

	return
}
