package wol

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAppliesDefaultsAndNormalizesValues(t *testing.T) {
	path := writeTempConfig(t, `
homekit:
  pin: "001-02-003"
wol:
  broadcast_ip: "192.168.1.255"
devices:
  - name: " Gaming PC "
    mac: "00-11-22-33-44-55"
  - name: "NAS"
    mac: "66-77-88-99-AA-BB"
    port: 7
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.HomeKit.Pin != "00102003" {
		t.Fatalf("expected normalized pin, got %q", cfg.HomeKit.Pin)
	}
	if cfg.HomeKit.Name != defaultBridgeName {
		t.Fatalf("expected default bridge name, got %q", cfg.HomeKit.Name)
	}
	if cfg.HomeKit.StoragePath != "./db" {
		t.Fatalf("expected default storage path, got %q", cfg.HomeKit.StoragePath)
	}
	if cfg.WOL.BroadcastIP != "192.168.1.255" {
		t.Fatalf("expected shared broadcast IP, got %q", cfg.WOL.BroadcastIP)
	}
	if cfg.WOL.Port != 9 {
		t.Fatalf("expected default shared port, got %d", cfg.WOL.Port)
	}
	if len(cfg.Devices) != 2 {
		t.Fatalf("expected two devices, got %d", len(cfg.Devices))
	}
	if cfg.Devices[0].Name != "Gaming PC" {
		t.Fatalf("expected trimmed device name, got %q", cfg.Devices[0].Name)
	}
	if cfg.Devices[0].MAC != "00:11:22:33:44:55" {
		t.Fatalf("expected canonical MAC format, got %q", cfg.Devices[0].MAC)
	}
	if cfg.Devices[0].BroadcastIP != "192.168.1.255" {
		t.Fatalf("expected inherited broadcast IP, got %q", cfg.Devices[0].BroadcastIP)
	}
	if cfg.Devices[0].Port != 9 {
		t.Fatalf("expected inherited port, got %d", cfg.Devices[0].Port)
	}
	if cfg.Devices[1].MAC != "66:77:88:99:aa:bb" {
		t.Fatalf("expected canonical MAC format, got %q", cfg.Devices[1].MAC)
	}
	if cfg.Devices[1].BroadcastIP != "192.168.1.255" {
		t.Fatalf("expected inherited broadcast IP, got %q", cfg.Devices[1].BroadcastIP)
	}
	if cfg.Devices[1].Port != 7 {
		t.Fatalf("expected device port override, got %d", cfg.Devices[1].Port)
	}
}

func TestLoadDefaultsPinWhenMissing(t *testing.T) {
	path := writeTempConfig(t, `
devices:
  - name: "Gaming PC"
    mac: "00:11:22:33:44:55"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.HomeKit.Pin != defaultPin {
		t.Fatalf("expected default pin %q, got %q", defaultPin, cfg.HomeKit.Pin)
	}
}

func TestLoadRejectsInvalidPin(t *testing.T) {
	path := writeTempConfig(t, `
homekit:
  pin: "123-45-678"
devices:
  - name: "Gaming PC"
    mac: "00:11:22:33:44:55"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected invalid pin to fail validation")
	}
	if !strings.Contains(err.Error(), "homekit.pin") {
		t.Fatalf("expected homekit.pin validation error, got %v", err)
	}
}

func TestLoadRejectsInvalidMAC(t *testing.T) {
	path := writeTempConfig(t, `
homekit:
  pin: "001-02-003"
devices:
  - name: "Gaming PC"
    mac: "not-a-mac"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected invalid MAC to fail validation")
	}
	if !strings.Contains(err.Error(), "devices[0].mac") {
		t.Fatalf("expected devices[0].mac validation error, got %v", err)
	}
}

func TestLoadAllowsDeviceBroadcastOverride(t *testing.T) {
	path := writeTempConfig(t, `
homekit:
  pin: "001-02-003"
wol:
  broadcast_ip: "255.255.255.255"
devices:
  - name: "Gaming PC"
    mac: "00:11:22:33:44:55"
    broadcast_ip: "192.168.1.255"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Devices[0].BroadcastIP != "192.168.1.255" {
		t.Fatalf("expected device override broadcast IP, got %q", cfg.Devices[0].BroadcastIP)
	}
}

func TestLoadRejectsInvalidDeviceBroadcastOverride(t *testing.T) {
	path := writeTempConfig(t, `
homekit:
  pin: "001-02-003"
wol:
  broadcast_ip: "255.255.255.255"
devices:
  - name: "Gaming PC"
    mac: "00:11:22:33:44:55"
    broadcast_ip: "not-an-ip"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected invalid device broadcast override to fail validation")
	}
	if !strings.Contains(err.Error(), "devices[0].broadcast_ip") {
		t.Fatalf("expected devices[0].broadcast_ip validation error, got %v", err)
	}
}

func TestLoadRejectsNonEthernetMAC(t *testing.T) {
	path := writeTempConfig(t, `
homekit:
  pin: "001-02-003"
devices:
  - name: "Gaming PC"
    mac: "00:11:22:33:44:55:66:77"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected non-6-byte MAC to fail validation")
	}
	if !strings.Contains(err.Error(), "6-byte MAC") {
		t.Fatalf("expected 6-byte MAC validation error, got %v", err)
	}
}

func TestLoadRejectsDuplicateDeviceNames(t *testing.T) {
	path := writeTempConfig(t, `
homekit:
  pin: "001-02-003"
devices:
  - name: "Gaming PC"
    mac: "00:11:22:33:44:55"
  - name: "gaming pc"
    mac: "66:77:88:99:aa:bb"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected duplicate device name to fail validation")
	}
	if !strings.Contains(err.Error(), "duplicates devices[0].name") {
		t.Fatalf("expected duplicate name validation error, got %v", err)
	}
}

func TestLoadRejectsDuplicateDeviceMACs(t *testing.T) {
	path := writeTempConfig(t, `
homekit:
  pin: "001-02-003"
devices:
  - name: "Gaming PC"
    mac: "00:11:22:33:44:55"
  - name: "NAS"
    mac: "00-11-22-33-44-55"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected duplicate device MAC to fail validation")
	}
	if !strings.Contains(err.Error(), "duplicates devices[0].mac") {
		t.Fatalf("expected duplicate MAC validation error, got %v", err)
	}
}

func TestLoadRejectsEmptyDeviceList(t *testing.T) {
	path := writeTempConfig(t, `
homekit:
  pin: "001-02-003"
devices: []
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected empty device list to fail validation")
	}
	if !strings.Contains(err.Error(), "devices must contain at least one device") {
		t.Fatalf("expected empty devices validation error, got %v", err)
	}
}

func TestLoadRejectsLegacyDeviceConfig(t *testing.T) {
	path := writeTempConfig(t, `
homekit:
  pin: "001-02-003"
device:
  name: "Gaming PC"
  mac: "00:11:22:33:44:55"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected legacy device config to fail validation")
	}
	if !strings.Contains(err.Error(), "field device not found") {
		t.Fatalf("expected unknown-field error, got %v", err)
	}
}

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return path
}

func TestPowerOptionsValidation(t *testing.T) {
	base := Config{Devices: []DeviceConfig{{Name: "PC", MAC: "02:00:00:00:00:01"}}}
	base.applyDefaults()
	for _, tc := range []struct {
		name    string
		options *PowerOptions
		valid   bool
	}{
		{"legacy", nil, true},
		{"pve", &PowerOptions{Type: "pve", Host: "192.0.2.10", Token: "homekit@pve!power=secret"}, true},
		{"unknown", &PowerOptions{Type: "ssh", Host: "192.0.2.10", Token: "homekit@pve!power=secret"}, false},
		{"missing host", &PowerOptions{Type: "pve", Token: "homekit@pve!power=secret"}, false},
		{"missing token", &PowerOptions{Type: "pve", Host: "192.0.2.10"}, false},
		{"header injection", &PowerOptions{Type: "pve", Host: "192.0.2.10", Token: "homekit@pve!power=secret\n"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base.Devices[0].Options = tc.options
			err := base.Validate()
			if (err == nil) != tc.valid {
				t.Fatalf("validation=%v", err)
			}
		})
	}
}
func TestMissingConfigDoesNotFallback(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "config.yaml")); err == nil {
		t.Fatal("missing config accepted")
	}
}
