package temperature

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"homekit-devices/internal/hapserver"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "temp")
	for _, tc := range []struct {
		raw  string
		want float64
		fail bool
	}{{"42500\n", 42.5, false}, {"0", 0, false}, {"100000", 100, false}, {"", 0, true}, {"NaN", 0, true}, {"42000.5", 0, true}, {"-1", 0, true}, {"100001", 0, true}} {
		t.Run(tc.raw, func(t *testing.T) {
			write(t, path, tc.raw)
			got, err := Read(path)
			if (err != nil) != tc.fail || (!tc.fail && got != tc.want) {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
	if _, err := Read(path + "missing"); err == nil {
		t.Fatal("missing file accepted")
	}
}
func TestConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	write(t, path, "temperature:\n  path: /sys/class/thermal/thermal_zone3/temp\n")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Temperature.Name != "路由器温度" || c.Temperature.Interval != "30s" || c.HomeKit.StoragePath != "./temperature-db" {
		t.Fatalf("defaults: %+v", c)
	}
	for _, body := range []string{"temperature: {}", "temperature:\n  path: /tmp/temp\n  interval: 0s", "temperature:\n  path: /tmp/temp\n  interval: bogus", "temperature:\n  path: /tmp/temp\n  typo: true", "homekit:\n  pin: 12345678\ntemperature:\n  path: /tmp/temp"} {
		write(t, path, body)
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted invalid config %s", body)
		}
	}
}
func TestMonitorFailureRecoveryAndIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "temp")
	write(t, path, "42500")
	c := Config{HomeKit: hapserver.Config{Manufacturer: "test", Model: "router", Firmware: "1"}, Temperature: SensorConfig{Name: "Router", Path: path, Interval: "30s"}}
	m, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	write(t, path, "invalid")
	if err := m.Sample(); err == nil {
		t.Fatal("expected fault")
	}
	if m.Current.Value() != 42.5 || m.Fault.Value() != 1 {
		t.Fatal("failure overwrote valid reading or omitted fault")
	}
	write(t, path, "53000")
	if err := m.Sample(); err != nil {
		t.Fatal(err)
	}
	if m.Current.Value() != 53 || m.Fault.Value() != 0 {
		t.Fatal("recovery failed")
	}
	c.Temperature.Name = "Renamed"
	other, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	if m.Accessory.Id != other.Accessory.Id || m.Accessory.Info.SerialNumber.Value() != other.Accessory.Info.SerialNumber.Value() {
		t.Fatal("rename changed identity")
	}
	write(t, path, "bad")
	if _, err := New(c); err == nil || !strings.Contains(err.Error(), "initial temperature") {
		t.Fatal("initial failure accepted", err)
	}
}
func TestDiscover(t *testing.T) {
	root := t.TempDir()
	if _, err := Discover(root); err == nil {
		t.Fatal("empty discovery accepted")
	}
	zone := filepath.Join(root, "thermal_zone4")
	if err := os.Mkdir(zone, 0700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(zone, "type"), "cpu-thermal\n")
	write(t, filepath.Join(zone, "temp"), "61000")
	sensors, err := Discover(root)
	if err != nil || len(sensors) != 1 || sensors[0].Type != "cpu-thermal" || sensors[0].Celsius != 61 {
		t.Fatalf("%+v %v", sensors, err)
	}
	os.Remove(filepath.Join(zone, "temp"))
	sensors, err = Discover(root)
	if err != nil || sensors[0].Err == nil {
		t.Fatal("unreadable zone hidden")
	}
}
func TestPollStopsOnCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "temp")
	write(t, path, "40000")
	m, err := New(Config{Temperature: SensorConfig{Name: "Router", Path: path, Interval: "1ms"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Poll(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("poll did not stop")
	}
}
