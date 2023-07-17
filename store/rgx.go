package store

import "regexp"

var (
	DomainRgx = regexp.MustCompile(`^([A-Za-z0-9-]{1,63}\.)+[A-Za-z]{2,6}$`)
	HostRgx   = regexp.MustCompile(`^([A-Za-z0-9-]{1,63}\.)+[A-Za-z]{2,6}(:[0-9]{1,5})?$`)
)
