package flags

import (
	"errors"
	"fmt"
	"time"
)

var Flags = &_Flags{}

type _Flags struct {
	Verbose           int           `default:"0"`
	Debug             bool          `default:"false"`
	TerminateTimeout  time.Duration `default:"30s"`
	QuitTimeout       time.Duration `default:"15s"`
	Config            string        `default:"config.yaml"`
	StorePath         string        `default:""`
	StoreMaxIdleConns int           `default:"100"`
	StoreUserAgent    string        `default:"oscdn"`
	Http              string        `default:":8080"`
	Https             string        `default:":8443"`
	Mgmt              string        `default:":8000"`
	ListenBacklog     int           `default:"128"`
	MinTlsVersion     string        `default:"1.2"`
	MaxConns          int32         `default:"1000"`
	HandleH2c         bool          `default:"false"`
	ServerHeader      string        `default:"oscdn"`
}

func (f *_Flags) Validate() error {
	if f.StorePath == "" {
		return errors.New("empty store path")
	}
	switch f.MinTlsVersion {
	case "1.0":
	case "1.1":
	case "1.2":
	case "1.3":
	default:
		return fmt.Errorf("unknown minimum tls version %q", f.MinTlsVersion)
	}
	return nil
}
