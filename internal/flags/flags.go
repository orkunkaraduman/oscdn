package flags

import "time"

var Flags = &_Flags{}

type _Flags struct {
	Verbose          int           `default:"0"`
	Debug            bool          `default:"false"`
	TerminateTimeout time.Duration `default:"2m"`
	QuitTimeout      time.Duration `default:"3m"`
	Config           string        `default:"config.yaml"`
	StorePath        string        `default:"."`
	ListenBacklog    int           `default:"0"`
	Http             string        `default:":8080"`
	Https            string        `default:":8443"`
	Mgmt             string        `default:":9080"`
}

func (f *_Flags) Validate() error {
	return nil
}
