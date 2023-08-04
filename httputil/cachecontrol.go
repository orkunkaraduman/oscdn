package httputil

import (
	"math"
	"strconv"
	"time"
)

type CacheControl map[string]string

func ParseCacheControl(directive string) CacheControl {
	opts := ParseOptions(directive)
	if len(opts) <= 0 {
		return nil
	}
	return opts[0].Parameters
}

func (c CacheControl) MaxAge() time.Duration {
	return c.timedDirective("max-age", -1)
}

func (c CacheControl) MaxStale() time.Duration {
	return c.timedDirective("max-stale", math.MaxInt64)
}

func (c CacheControl) MinFresh() time.Duration {
	return c.timedDirective("min-fresh", -1)
}

func (c CacheControl) SMaxAge() time.Duration {
	return c.timedDirective("s-maxage", -1)
}

func (c CacheControl) NoCache() bool {
	_, ok := c["no-cache"]
	return ok
}

func (c CacheControl) NoStore() bool {
	_, ok := c["no-store"]
	return ok
}

func (c CacheControl) NoTransform() bool {
	_, ok := c["no-transform"]
	return ok
}

func (c CacheControl) OnlyIfCached() bool {
	_, ok := c["only-if-cached"]
	return ok
}

func (c CacheControl) MustRevalidate() bool {
	_, ok := c["must-revalidate"]
	return ok
}

func (c CacheControl) ProxyRevalidate() bool {
	_, ok := c["proxy-revalidate"]
	return ok
}

func (c CacheControl) Private() bool {
	_, ok := c["private"]
	return ok
}

func (c CacheControl) Public() bool {
	_, ok := c["public"]
	return ok
}

func (c CacheControl) timedDirective(key string, def time.Duration) time.Duration {
	s, ok := c[key]
	if !ok {
		return -1
	}
	if s == "" {
		return def
	}

	x, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return time.Duration(x) * time.Second
}
