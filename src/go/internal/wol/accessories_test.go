package wol

import (
	"testing"
)

func TestAccessoryIDForMAC(t *testing.T) {
	accessoryID, err := accessoryIDForMAC("00:11:22:33:44:55")
	if err != nil {
		t.Fatalf("accessoryIDForMAC() returned error: %v", err)
	}

	const want uint64 = 0x001122334455 + bridgeAccessoryID + 1
	if accessoryID != want {
		t.Fatalf("expected accessory ID %d, got %d", want, accessoryID)
	}
}

func TestAccessoryIDForMACAcceptsDifferentFormats(t *testing.T) {
	colonSeparated, err := accessoryIDForMAC("00:11:22:33:44:55")
	if err != nil {
		t.Fatalf("accessoryIDForMAC() returned error: %v", err)
	}
	hyphenSeparated, err := accessoryIDForMAC("00-11-22-33-44-55")
	if err != nil {
		t.Fatalf("accessoryIDForMAC() returned error: %v", err)
	}

	if colonSeparated != hyphenSeparated {
		t.Fatalf("expected consistent accessory ID, got %d and %d", colonSeparated, hyphenSeparated)
	}
}

func TestBridgeSerialNumberIgnoresDeviceOrder(t *testing.T) {
	forward := Config{
		Devices: []DeviceConfig{
			{Name: "Gaming PC", MAC: "00:11:22:33:44:55"},
			{Name: "NAS", MAC: "66:77:88:99:aa:bb"},
		},
	}
	reversed := Config{
		Devices: []DeviceConfig{
			{Name: "NAS", MAC: "66:77:88:99:aa:bb"},
			{Name: "Gaming PC", MAC: "00:11:22:33:44:55"},
		},
	}

	if bridgeSerialNumber(forward) != bridgeSerialNumber(reversed) {
		t.Fatal("expected bridge serial number to be stable regardless of device order")
	}
}

func TestBuildServerAccessoriesCreatesBridgeAndSwitches(t *testing.T) {
	cfg := Config{
		HomeKit: HomeKitConfig{
			Name:         "Wake Targets",
			Pin:          "00102003",
			StoragePath:  "./db",
			Manufacturer: "homekit-wol",
			Model:        "wake-switch",
			Firmware:     "1.0.0",
		},
		Devices: []DeviceConfig{
			{Name: "Gaming PC", MAC: "00:11:22:33:44:55", BroadcastIP: "255.255.255.255", Port: 9},
			{Name: "NAS", MAC: "66:77:88:99:aa:bb", BroadcastIP: "255.255.255.255", Port: 9},
		},
	}

	accessories, err := BuildAccessories(cfg)
	if err != nil {
		t.Fatalf("BuildAccessories() returned error: %v", err)
	}
	if len(accessories) != 3 {
		t.Fatalf("expected 3 accessories, got %d", len(accessories))
	}
	if accessories[0].Id != bridgeAccessoryID {
		t.Fatalf("expected bridge ID %d, got %d", bridgeAccessoryID, accessories[0].Id)
	}
	if accessories[0].Name() != "Wake Targets" {
		t.Fatalf("expected bridge name Wake Targets, got %q", accessories[0].Name())
	}

	firstID, err := accessoryIDForMAC(cfg.Devices[0].MAC)
	if err != nil {
		t.Fatalf("accessoryIDForMAC() returned error: %v", err)
	}
	secondID, err := accessoryIDForMAC(cfg.Devices[1].MAC)
	if err != nil {
		t.Fatalf("accessoryIDForMAC() returned error: %v", err)
	}
	if accessories[1].Id != firstID {
		t.Fatalf("expected first child ID %d, got %d", firstID, accessories[1].Id)
	}
	if accessories[2].Id != secondID {
		t.Fatalf("expected second child ID %d, got %d", secondID, accessories[2].Id)
	}
	if accessories[1].Name() != "Gaming PC" {
		t.Fatalf("expected first child name Gaming PC, got %q", accessories[1].Name())
	}
	if accessories[2].Name() != "NAS" {
		t.Fatalf("expected second child name NAS, got %q", accessories[2].Name())
	}
}
