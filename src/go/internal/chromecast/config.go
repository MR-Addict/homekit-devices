package chromecast

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v3"
	"homekit-devices/internal/hapserver"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type AppConfig struct {
	HomeKit    hapserver.Config `yaml:"homekit"`
	Chromecast Config           `yaml:"chromecast"`
	Control    ControlConfig    `yaml:"control"`
	Apps       []AppInput       `yaml:"apps"`
}
type AppInput struct {
	ID      int    `yaml:"id"`
	Name    string `yaml:"name"`
	Launch  string `yaml:"launch"`
	Package string `yaml:"package"`
}

type ControlConfig struct {
	Interval          string `yaml:"interval"`
	Timeout           string `yaml:"timeout"`
	TransitionTimeout string `yaml:"transition_timeout"`
}

func Load(path string) (AppConfig, error) {
	var c AppConfig
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err = d.Decode(&c); err != nil {
		return c, err
	}
	var trailing any
	if err = d.Decode(&trailing); err != io.EOF {
		return c, fmt.Errorf("config must contain exactly one YAML document")
	}
	c.HomeKit.Normalize()
	c.HomeKit.ApplyDefaults(hapserver.Config{Name: "Chromecast", Pin: "00102003", StoragePath: "./chromecast-db", Manufacturer: "homekit-devices", Model: "chromecast-remote", Firmware: "1.0.0"})
	if c.HomeKit.ListenAddress == "" {
		c.HomeKit.ListenAddress = ":32044"
	}
	if problems := c.HomeKit.Problems(); len(problems) > 0 {
		return c, fmt.Errorf("HomeKit config: %s", strings.Join(problems, "; "))
	}
	c.Chromecast.Host = strings.TrimSpace(c.Chromecast.Host)
	c.Chromecast.KeyPath = strings.TrimSpace(c.Chromecast.KeyPath)
	c.Chromecast.CertPath = strings.TrimSpace(c.Chromecast.CertPath)
	c.Chromecast.PowerMode = strings.TrimSpace(c.Chromecast.PowerMode)
	host := c.Chromecast.Host
	if host == "" || strings.ContainsAny(host, " /\t\r\n") || strings.Contains(host, ":") && net.ParseIP(host) == nil {
		return c, fmt.Errorf("chromecast.host must be an IP or hostname without a port")
	}
	if c.Chromecast.APIPort < 0 || c.Chromecast.APIPort > 65535 {
		return c, fmt.Errorf("chromecast.api_port must be 1..65535 or omitted")
	}
	if c.Chromecast.KeyPath == "" {
		c.Chromecast.KeyPath = "./chromecast-credentials/key.pem"
	}
	if c.Chromecast.CertPath == "" {
		c.Chromecast.CertPath = "./chromecast-credentials/cert.pem"
	}
	if c.Chromecast.KeyPath == c.Chromecast.CertPath {
		return c, fmt.Errorf("certificate and private key paths must differ")
	}
	if c.Chromecast.PowerMode == "" {
		c.Chromecast.PowerMode = "toggle"
	}
	if c.Chromecast.PowerMode != "toggle" && c.Chromecast.PowerMode != "discrete" {
		return c, fmt.Errorf("chromecast.power_mode must be toggle or discrete")
	}
	for field, def := range map[*string]string{&c.Control.Interval: "5s", &c.Control.Timeout: "5s", &c.Control.TransitionTimeout: "15s"} {
		if *field == "" {
			*field = def
		}
		value, e := time.ParseDuration(*field)
		if e != nil || value <= 0 {
			return c, fmt.Errorf("invalid positive duration %q", *field)
		}
	}
	ids := make(map[int]bool)
	packages := make(map[string]bool)
	for i := range c.Apps {
		a := &c.Apps[i]
		a.Name, a.Launch, a.Package = strings.TrimSpace(a.Name), strings.TrimSpace(a.Launch), strings.TrimSpace(a.Package)
		if a.ID <= 0 || uint64(a.ID) > uint64(1<<32-1) || a.Name == "" || a.Launch == "" || a.Package == "" {
			return c, fmt.Errorf("apps[%d] requires id in 1..4294967295 and nonempty name, launch, package", i)
		}
		if ids[a.ID] || packages[a.Package] {
			return c, fmt.Errorf("apps[%d] has duplicate id or package", i)
		}
		ids[a.ID], packages[a.Package] = true, true
	}
	base := filepath.Dir(path)
	for _, p := range []*string{&c.HomeKit.StoragePath, &c.Chromecast.KeyPath, &c.Chromecast.CertPath} {
		if !filepath.IsAbs(*p) {
			*p = filepath.Join(base, *p)
		}
		*p = filepath.Clean(*p)
	}
	if c.Chromecast.KeyPath == c.Chromecast.CertPath {
		return c, fmt.Errorf("certificate and private key paths must differ")
	}
	return c, nil
}
func Duration(s string) time.Duration { d, _ := time.ParseDuration(s); return d }
