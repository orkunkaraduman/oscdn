package config

import (
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Hosts map[string]struct {
		TLS struct {
			Key  string
			Cert string
		}
		Origin        string
		HostOverride  bool
		IgnoreQuery   bool
		HttpsRedirect bool
		UploadBurst   int64
		UploadRate    int64
	}
	Origins map[string]struct {
		UseHttps      bool
		MaxSize       int64
		MaxAge        time.Duration
		DownloadBurst int64
		DownloadRate  int64
	}
}

func New(r io.Reader) (c *Config, err error) {
	c = new(Config)
	d := yaml.NewDecoder(r)
	err = d.Decode(c)
	if err != nil {
		err = fmt.Errorf("unable to decode yaml: %w", err)
		return nil, err
	}
	return
}

func FromFile(name string) (c *Config, err error) {
	f, err := os.Open(name)
	if err != nil {
		err = fmt.Errorf("unable to open file: %w", err)
		return nil, err
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	return New(f)
}

func (c *Config) Validate() error {
	for k, h := range c.Hosts {
		if _, ok := c.Origins[h.Origin]; !ok {
			return fmt.Errorf("unknown origin %q for %q", h.Origin, k)
		}
	}
	return nil
}
