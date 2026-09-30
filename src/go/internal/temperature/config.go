package temperature

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"homekit-devices/internal/hapserver"
)

type Config struct {
	HomeKit     hapserver.Config `yaml:"homekit"`
	Temperature SensorConfig     `yaml:"temperature"`
}
type SensorConfig struct {
	Name     string `yaml:"name"`
	Path     string `yaml:"path"`
	Interval string `yaml:"interval"`
}

func Load(path string) (Config, error) {
	var c Config
	data, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("read config: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err = dec.Decode(&c); err != nil {
		return c, fmt.Errorf("decode config: %w", err)
	}
	c.HomeKit.Normalize()
	c.HomeKit.ApplyDefaults(hapserver.Config{Name: "路由器温度", Pin: "00102003", StoragePath: "./temperature-db", Manufacturer: "homekit-devices", Model: "router-temperature", Firmware: "1.0.0"})
	c.Temperature.Name = strings.TrimSpace(c.Temperature.Name)
	if c.Temperature.Name == "" {
		c.Temperature.Name = "路由器温度"
	}
	c.Temperature.Path = strings.TrimSpace(c.Temperature.Path)
	c.Temperature.Interval = strings.TrimSpace(c.Temperature.Interval)
	if c.Temperature.Interval == "" {
		c.Temperature.Interval = "30s"
	}
	if problems := c.HomeKit.Problems(); len(problems) > 0 {
		return c, fmt.Errorf("HomeKit config: %s", strings.Join(problems, "; "))
	}
	if c.Temperature.Path == "" {
		return c, fmt.Errorf("temperature.path is required; use -list-sensors to discover sensors")
	}
	if _, err := c.Temperature.Duration(); err != nil {
		return c, err
	}
	return c, nil
}
func (c SensorConfig) Duration() (time.Duration, error) {
	d, err := time.ParseDuration(c.Interval)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("temperature.interval must be a positive duration, e.g. 30s")
	}
	return d, nil
}
