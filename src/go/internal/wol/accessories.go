package wol

import (
	"context"
	"crypto/sha1"
	"fmt"
	"log"
	"net"
	"sort"
	"strings"

	"github.com/brutella/hap/accessory"
)

const bridgeAccessoryID uint64 = 1

func BuildAccessories(cfg Config) ([]*accessory.A, error) {
	bridge := accessory.NewBridge(accessory.Info{
		Name:         cfg.HomeKit.Name,
		SerialNumber: bridgeSerialNumber(cfg),
		Manufacturer: cfg.HomeKit.Manufacturer,
		Model:        cfg.HomeKit.Model,
		Firmware:     cfg.HomeKit.Firmware,
	})
	bridge.Id = bridgeAccessoryID

	accessories := make([]*accessory.A, 0, len(cfg.Devices)+1)
	accessories = append(accessories, bridge.A)

	for _, device := range cfg.Devices {
		wakeSwitch := NewWakeSwitch(accessory.Info{
			Name:         device.Name,
			SerialNumber: serialNumberForMAC(device.MAC),
			Manufacturer: cfg.HomeKit.Manufacturer,
			Model:        cfg.HomeKit.Model,
			Firmware:     cfg.HomeKit.Firmware,
		}, DefaultResetDelay, func(ctx context.Context) error {
			log.Printf("sending Wake-on-LAN packet to %s (%s)", device.Name, device.MAC)
			return Send(ctx, device.MAC, device.BroadcastIP, device.Port)
		})

		accessoryID, err := accessoryIDForMAC(device.MAC)
		if err != nil {
			return nil, fmt.Errorf("derive accessory id for %q: %w", device.Name, err)
		}
		wakeSwitch.Id = accessoryID

		accessories = append(accessories, wakeSwitch.A)
	}

	return accessories, nil
}

func bridgeSerialNumber(cfg Config) string {
	if cfg.HomeKit.SerialNumber != "" {
		return cfg.HomeKit.SerialNumber
	}

	macs := make([]string, 0, len(cfg.Devices))
	for _, device := range cfg.Devices {
		macs = append(macs, serialNumberForMAC(device.MAC))
	}
	sort.Strings(macs)

	sum := sha1.Sum([]byte(strings.Join(macs, ",")))
	return fmt.Sprintf("BRIDGE-%X", sum[:6])

}

func serialNumberForMAC(mac string) string {
	return strings.ToUpper(strings.ReplaceAll(mac, ":", ""))
}

func accessoryIDForMAC(mac string) (uint64, error) {
	hardwareAddr, err := net.ParseMAC(mac)
	if err != nil {
		return 0, fmt.Errorf("parse mac address: %w", err)
	}
	if len(hardwareAddr) != 6 {
		return 0, fmt.Errorf("mac address must be 6 bytes")
	}

	var id uint64
	for _, octet := range hardwareAddr {
		id = (id << 8) | uint64(octet)
	}

	return id + bridgeAccessoryID + 1, nil
}
