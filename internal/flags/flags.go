package flags

import (
	"errors"
	"fmt"
	"time"
)

var Flags = &_Flags{}

type _Flags struct {
	Verbose          int           `default:"0"`
	Debug            bool          `default:"false"`
	TerminateTimeout time.Duration `default:"30s"`
	QuitTimeout      time.Duration `default:"15s"`
	Config           string        `default:"config.yaml"`
	StorePath        string        `default:""`
	MaxIdleConns     int           `default:"100"`
	UserAgent        string        `default:"oscdn"`
	ServerHeader     string        `default:"oscdn"`
	Http             string        `default:":8080"`
	Https            string        `default:":8443"`
	Mgmt             string        `default:":8000"`
	ListenBacklog    int           `default:"128"`
	MaxConns         int32         `default:"1000"`
	HandleH2c        bool          `default:"false"`
	MinTlsVersion    string        `default:"1.2"`
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
