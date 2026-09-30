package hapserver

import (
	"net"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/brutella/hap"
)

type Config struct {
	Name          string   `yaml:"name"`
	Pin           string   `yaml:"pin"`
	StoragePath   string   `yaml:"storage_path"`
	ListenAddress string   `yaml:"listen_address,omitempty"`
	Interfaces    []string `yaml:"interfaces,omitempty"`
	SerialNumber  string   `yaml:"serial_number,omitempty"`
	Manufacturer  string   `yaml:"manufacturer,omitempty"`
	Model         string   `yaml:"model,omitempty"`
	Firmware      string   `yaml:"firmware,omitempty"`
}

func (c *Config) Normalize() {
	c.Name = strings.TrimSpace(c.Name)
	var pin strings.Builder
	for _, r := range strings.TrimSpace(c.Pin) {
		if r >= '0' && r <= '9' {
			pin.WriteRune(r)
		}
	}
	c.Pin = pin.String()
	c.StoragePath = strings.TrimSpace(c.StoragePath)
	if c.StoragePath != "" {
		c.StoragePath = filepath.Clean(c.StoragePath)
	}
	c.ListenAddress = strings.TrimSpace(c.ListenAddress)
	c.SerialNumber = strings.TrimSpace(c.SerialNumber)
	c.Manufacturer = strings.TrimSpace(c.Manufacturer)
	c.Model = strings.TrimSpace(c.Model)
	c.Firmware = strings.TrimSpace(c.Firmware)
	for i := range c.Interfaces {
		c.Interfaces[i] = strings.TrimSpace(c.Interfaces[i])
	}
}
func (c *Config) ApplyDefaults(d Config) {
	if c.Name == "" {
		c.Name = d.Name
	}
	if c.Pin == "" {
		c.Pin = d.Pin
	}
	if c.StoragePath == "" {
		c.StoragePath = d.StoragePath
	}
	if c.Manufacturer == "" {
		c.Manufacturer = d.Manufacturer
	}
	if c.Model == "" {
		c.Model = d.Model
	}
	if c.Firmware == "" {
		c.Firmware = d.Firmware
	}
}
func (c Config) Problems() []string {
	var p []string
	if len(c.Pin) != 8 || strings.Trim(c.Pin, "0123456789") != "" {
		p = append(p, "homekit.pin must be 8 digits or the Apple Home style 3-2-3 format")
	} else if hap.InvalidPins[c.Pin] {
		p = append(p, "homekit.pin uses a HomeKit-reserved invalid pin")
	}
	if c.StoragePath == "" {
		p = append(p, "homekit.storage_path is required")
	}
	if c.ListenAddress != "" {
		_, port, err := net.SplitHostPort(c.ListenAddress)
		n, e := strconv.Atoi(port)
		if err != nil || e != nil || n < 1 || n > 65535 {
			p = append(p, "homekit.listen_address must be in host:port form with a valid TCP port")
		}
	}
	for _, iface := range c.Interfaces {
		if iface == "" {
			p = append(p, "homekit.interfaces cannot contain empty values")
			break
		}
	}
	return p
}
