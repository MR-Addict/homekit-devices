package wol

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"homekit-devices/internal/hapserver"
)

const (
	defaultBridgeName   = "Wake Targets"
	defaultPin          = "00102003"
	defaultStoragePath  = "./db"
	defaultBroadcastIP  = "255.255.255.255"
	defaultPort         = 9
	defaultManufacturer = "homekit-wol"
	defaultModel        = "wake-switch"
	defaultFirmware     = "1.0.0"
)

type Config struct {
	HomeKit HomeKitConfig  `yaml:"homekit"`
	WOL     WOLConfig      `yaml:"wol,omitempty"`
	Devices []DeviceConfig `yaml:"devices"`
}

type HomeKitConfig = hapserver.Config

type WOLConfig struct {
	BroadcastIP string `yaml:"broadcast_ip,omitempty"`
	Port        int    `yaml:"port,omitempty"`
}

type DeviceConfig struct {
	Options     *PowerOptions `yaml:"options,omitempty"`
	Name        string        `yaml:"name"`
	MAC         string        `yaml:"mac"`
	BroadcastIP string        `yaml:"broadcast_ip,omitempty"`
	Port        int           `yaml:"port,omitempty"`
}

type PowerOptions struct {
	Type  string `yaml:"type"`
	Host  string `yaml:"host"`
	Token string `yaml:"token"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode yaml: %w", err)
	}

	cfg.normalize()
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (cfg *Config) normalize() {
	cfg.HomeKit.Normalize()

	cfg.WOL.BroadcastIP = strings.TrimSpace(cfg.WOL.BroadcastIP)

	for index := range cfg.Devices {
		if o := cfg.Devices[index].Options; o != nil {
			o.Type = strings.TrimSpace(o.Type)
			o.Host = strings.TrimSpace(o.Host)
			o.Token = strings.TrimSpace(o.Token)
		}
		cfg.Devices[index].Name = strings.TrimSpace(cfg.Devices[index].Name)
		cfg.Devices[index].MAC = strings.TrimSpace(cfg.Devices[index].MAC)
		cfg.Devices[index].BroadcastIP = strings.TrimSpace(cfg.Devices[index].BroadcastIP)

		if hw, err := net.ParseMAC(cfg.Devices[index].MAC); err == nil {
			cfg.Devices[index].MAC = strings.ToLower(hw.String())
		}
	}
}

func (cfg *Config) applyDefaults() {
	cfg.HomeKit.ApplyDefaults(hapserver.Config{Name: defaultBridgeName, Pin: defaultPin, StoragePath: defaultStoragePath, Manufacturer: defaultManufacturer, Model: defaultModel, Firmware: defaultFirmware})
	if cfg.WOL.BroadcastIP == "" {
		cfg.WOL.BroadcastIP = defaultBroadcastIP
	}
	if cfg.WOL.Port == 0 {
		cfg.WOL.Port = defaultPort
	}
	for index := range cfg.Devices {
		if cfg.Devices[index].BroadcastIP == "" {
			cfg.Devices[index].BroadcastIP = cfg.WOL.BroadcastIP
		}
		if cfg.Devices[index].Port == 0 {
			cfg.Devices[index].Port = cfg.WOL.Port
		}
	}
}

func (cfg Config) Validate() error {
	var problems []string

	problems = append(problems, cfg.HomeKit.Problems()...)

	defaultBroadcastIPValid := isValidIPv4(cfg.WOL.BroadcastIP)
	if cfg.WOL.BroadcastIP == "" {
		problems = append(problems, "wol.broadcast_ip is required")
	} else if !defaultBroadcastIPValid {
		problems = append(problems, "wol.broadcast_ip must be a valid IPv4 address")
	}

	defaultPortValid := isValidPort(cfg.WOL.Port)
	if !defaultPortValid {
		problems = append(problems, "wol.port must be between 1 and 65535")
	}

	if len(cfg.Devices) == 0 {
		problems = append(problems, "devices must contain at least one device")
	}

	seenNames := make(map[string]int, len(cfg.Devices))
	seenMACs := make(map[string]int, len(cfg.Devices))
	for index, device := range cfg.Devices {
		path := fmt.Sprintf("devices[%d]", index)
		if o := device.Options; o != nil {
			if o.Type != "pve" {
				problems = append(problems, path+".options.type must be pve")
			}
			if ip := net.ParseIP(o.Host); ip == nil || ip.To4() == nil {
				problems = append(problems, path+".options.host must be a fixed IPv4 address")
			}
			id, secret, ok := strings.Cut(o.Token, "=")
			if !ok || !strings.Contains(id, "@") || !strings.Contains(id, "!") || secret == "" || strings.ContainsAny(o.Token, "\r\n") {
				problems = append(problems, path+".options.token must be user@realm!tokenid=secret")
			}
		}

		if device.Name == "" {
			problems = append(problems, path+".name is required")
		} else {
			nameKey := strings.ToLower(device.Name)
			if previousIndex, exists := seenNames[nameKey]; exists {
				problems = append(problems, fmt.Sprintf("%s.name duplicates devices[%d].name", path, previousIndex))
			} else {
				seenNames[nameKey] = index
			}
		}

		if device.MAC == "" {
			problems = append(problems, path+".mac is required")
		} else if hardwareAddr, err := net.ParseMAC(device.MAC); err != nil {
			problems = append(problems, path+".mac must be a valid MAC address")
		} else if len(hardwareAddr) != 6 {
			problems = append(problems, path+".mac must be a 6-byte MAC address")
		} else if previousIndex, exists := seenMACs[device.MAC]; exists {
			problems = append(problems, fmt.Sprintf("%s.mac duplicates devices[%d].mac", path, previousIndex))
		} else {
			seenMACs[device.MAC] = index
		}

		if device.BroadcastIP == "" {
			problems = append(problems, path+".broadcast_ip is required")
		} else if device.BroadcastIP != cfg.WOL.BroadcastIP || defaultBroadcastIPValid {
			if !isValidIPv4(device.BroadcastIP) {
				problems = append(problems, path+".broadcast_ip must be a valid IPv4 address")
			}
		}

		if !isValidPort(device.Port) {
			problems = append(problems, path+".port must be between 1 and 65535")
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid config: %s", strings.Join(problems, "; "))
	}

	return nil
}

func isValidPort(port int) bool {
	return port >= 1 && port <= 65535
}
