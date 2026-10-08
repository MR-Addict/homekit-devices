package wol

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/brutella/hap/accessory"
)

// Explicit opt-in hardware check. Normal test runs never contact live hosts.
// Compile for OpenWrt with go test -c, then set HOMEKIT_POWER_CONFIG and
// optionally HOMEKIT_POWER_ACTION=on|off to exercise the HAP command path.
func TestLivePVE(t *testing.T) {
	path := os.Getenv("HOMEKIT_POWER_CONFIG")
	if path == "" {
		t.Skip("set HOMEKIT_POWER_CONFIG to opt into hardware testing")
	}
	action := os.Getenv("HOMEKIT_POWER_ACTION")
	if action != "" && action != "on" && action != "off" {
		t.Fatal("action must be on or off")
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var d *DeviceConfig
	for i := range cfg.Devices {
		if cfg.Devices[i].Options != nil {
			if d != nil {
				t.Fatal("live test requires one power target")
			}
			d = &cfg.Devices[i]
		}
	}
	if d == nil {
		t.Fatal("no PVE device configured")
	}
	b := NewPVEClient(*d.Options)
	defer b.Close()
	c := newPowerController(accessory.Info{Name: d.Name}, b, func(ctx context.Context) error { return Send(ctx, d.MAC, d.BroadcastIP, d.Port) }, func(ctx context.Context) (bool, error) { return pingHost(ctx, d.Options.Host) })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute+10*time.Second)
	defer cancel()
	c.ctx = ctx
	c.Sample(ctx)
	if action == "" {
		v, code := c.Accessory.Switch.On.ValueRequest(httptest.NewRequest("GET", "/characteristics", nil))
		if code != 0 {
			t.Fatalf("state unavailable, HAP code=%d", code)
		}
		t.Logf("live observed state on=%v, node=%s", v, b.node)
		return
	}
	desired := action == "on"
	_, code := c.Accessory.Switch.On.SetValueRequest(desired, httptest.NewRequest("PUT", "/characteristics", nil))
	if code != 0 {
		t.Fatalf("command rejected, HAP code=%d", code)
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if c.known && c.fault == nil && c.on == desired && c.pending == nil {
			t.Logf("live operation completed, on=%t", desired)
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("live operation deadline exceeded")
		case <-ticker.C:
			c.Sample(ctx)
		}
		if c.pending == nil && c.on != desired {
			t.Fatal("controller operation timed out")
		}
	}
}
