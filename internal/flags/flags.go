package flags

import "time"

var Flags = &_Flags{}

type _Flags struct {
	Verbose          int           `name:"v"`
	Debug            bool          `name:"d"`
	TerminateTimeout time.Duration `default:"2m"`
	QuitTimeout      time.Duration `default:"3m"`
	Http             string        `default:":8080"`
	Https            string        `default:":8443"`
	Mgmt             string        `default:":9080"`
}

func (f *_Flags) Validate() error {
	return nil
}
