package hapserver

import (
	"context"
	"github.com/brutella/hap/accessory"
	"net"
	"path/filepath"
	"testing"
)

func TestNormalizeDefaultsAndValidation(t *testing.T) {
	c := Config{Name: " Router ", Pin: "001-02-003", Interfaces: []string{" br-lan "}, ListenAddress: ":32043"}
	c.Normalize()
	c.ApplyDefaults(Config{Name: "fallback", StoragePath: "./db", Manufacturer: "test", Model: "sensor", Firmware: "1"})
	if c.Name != "Router" || c.Pin != "00102003" || c.Interfaces[0] != "br-lan" || len(c.Problems()) != 0 {
		t.Fatalf("%+v %v", c, c.Problems())
	}
	for _, bad := range []Config{{Pin: "12345678", StoragePath: "db"}, {Pin: "00102003"}, {Pin: "00102003", StoragePath: "db", ListenAddress: ":0"}, {Pin: "00102003", StoragePath: "db", ListenAddress: "invalid"}, {Pin: "00102003", StoragePath: "db", Interfaces: []string{""}}} {
		if len(bad.Problems()) == 0 {
			t.Fatalf("accepted %+v", bad)
		}
	}
}
func TestRunRejectsMissingAccessories(t *testing.T) {
	cfg := Config{Pin: "00102003", StoragePath: filepath.Join(t.TempDir(), "db")}
	if err := Run(context.Background(), cfg, nil, nil); err == nil {
		t.Fatal("empty accessory list accepted")
	}
}

// An occupied port fails before mDNS setup, allowing a deterministic check that
// startup errors propagate and cancel the background worker.
func TestRunBindFailureStopsWorker(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg := Config{Name: "Test", Pin: "00102003", StoragePath: t.TempDir(), ListenAddress: listener.Addr().String()}
	a := accessory.NewSwitch(accessory.Info{Name: "Test"})
	stopped := make(chan struct{})
	err = Run(context.Background(), cfg, []*accessory.A{a.A}, func(ctx context.Context) { <-ctx.Done(); close(stopped) })
	if err == nil {
		t.Fatal("bind error ignored")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("worker outlived server")
	}
}
