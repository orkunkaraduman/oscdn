package config

import (
	"crypto/tls"
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Origins map[string]struct {
		UseHttps       bool          `yaml:"useHttps"`
		MaxSize        int64         `yaml:"maxSize"`
		MaxAge         time.Duration `yaml:"maxAge"`
		MaxAge404      time.Duration `yaml:"maxAge404"`
		MaxAgeOverride bool          `yaml:"maxAgeOverride"`
		DownloadBurst  int64         `yaml:"downloadBurst"`
		DownloadRate   int64         `yaml:"downloadRate"`
	} `yaml:"origins"`
	Hosts map[string]struct {
		TLS *struct {
			Cert string `yaml:"cert"`
			Key  string `yaml:"key"`
		} `yaml:"tls"`
		Origin             string `yaml:"origin"`
		HttpsRedirect      bool   `yaml:"httpsRedirect"`
		HttpsRedirectPort  int    `yaml:"httpsRedirectPort"`
		HostOverride       bool   `yaml:"hostOverride"`
		IgnoreQuery        bool   `yaml:"ignoreQuery"`
		CompressionMaxSize int64  `yaml:"compressionMaxSize"`
		UploadBurst        int64  `yaml:"uploadBurst"`
		UploadRate         int64  `yaml:"uploadRate"`
	} `yaml:"hosts"`
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
	for k, d := range c.Hosts {
		if _, ok := c.Origins[d.Origin]; !ok {
			return fmt.Errorf("unknown origin %q for %q", d.Origin, k)
		}
	}
	return nil
}

func (c *Config) TLSCertificates() (certs map[string]*tls.Certificate, err error) {
	certs = make(map[string]*tls.Certificate, len(c.Hosts))
	for k, d := range c.Hosts {
		if d.TLS == nil {
			continue
		}
		var cert tls.Certificate
		cert, err = tls.X509KeyPair([]byte(d.TLS.Cert), []byte(d.TLS.Key))
		if err != nil {
			err = fmt.Errorf("unable to load certificate for %q: %w", k, err)
			return nil, err
		}
		certs[k] = &cert
	}
	return certs, nil
}
