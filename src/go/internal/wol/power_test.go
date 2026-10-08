package wol

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/brutella/hap/accessory"
)

type fakePowerBackend struct {
	reachable   bool
	err         error
	shutdownErr error
	shutdowns   int
}

func (b *fakePowerBackend) Status(context.Context) (bool, error) { return b.reachable, b.err }
func (b *fakePowerBackend) Shutdown(context.Context) error       { b.shutdowns++; return b.shutdownErr }
func fixturePower() (*PowerController, *fakePowerBackend, *int, *bool, *error) {
	b := &fakePowerBackend{reachable: true}
	wakes := new(int)
	alive := new(bool)
	probeErr := new(error)
	c := newPowerController(accessory.Info{Name: "test"}, b, func(context.Context) error { *wakes++; return nil }, func(context.Context) (bool, error) { return *alive, *probeErr })
	return c, b, wakes, alive, probeErr
}
func remoteWrite(c *PowerController, on bool) int {
	_, code := c.Accessory.Switch.On.SetValueRequest(on, httptest.NewRequest("PUT", "/characteristics", nil))
	return code
}
func readPower(c *PowerController) (bool, int) {
	v, code := c.Accessory.Switch.On.ValueRequest(httptest.NewRequest("GET", "/characteristics", nil))
	if code != 0 {
		return false, code
	}
	return v.(bool), code
}
func TestPowerHAPObservedStateAndCommands(t *testing.T) {
	c, b, wakes, _, _ := fixturePower()
	var notifications []bool
	c.Accessory.Switch.On.OnValueUpdate(func(current, _ bool, req *http.Request) {
		if req != nil {
			t.Error("command produced a value notification")
		}
		notifications = append(notifications, current)
	})
	if _, code := readPower(c); code == 0 {
		t.Fatal("initial state must be unavailable")
	}
	if remoteWrite(c, false) == 0 || b.shutdowns != 0 {
		t.Fatal("initial same-value shutdown accepted")
	}
	if remoteWrite(c, true) != 0 || *wakes != 1 {
		t.Fatal("initial wake failed")
	}
	if c.Accessory.Switch.On.Value() {
		t.Fatal("command published desired state")
	}
	if remoteWrite(c, true) != 0 || *wakes != 1 {
		t.Fatal("duplicate wake resent")
	}
	if remoteWrite(c, false) == 0 {
		t.Fatal("reverse command accepted")
	}
	c.Sample(context.Background())
	if on, code := readPower(c); !on || code != 0 || c.pending != nil {
		t.Fatal("running state not published")
	}
	if remoteWrite(c, false) != 0 || b.shutdowns != 1 {
		t.Fatal("shutdown failed")
	}
	if !c.Accessory.Switch.On.Value() {
		t.Fatal("shutdown turned switch off early")
	}
	if remoteWrite(c, false) != 0 || b.shutdowns != 1 {
		t.Fatal("duplicate shutdown resent")
	}
	if remoteWrite(c, true) == 0 {
		t.Fatal("same-value reverse command bypassed hook")
	}
	b.reachable, b.err = false, errors.New("unreachable")
	for i := 0; i < 2; i++ {
		c.Sample(context.Background())
		if !c.Accessory.Switch.On.Value() {
			t.Fatal("offline before threshold")
		}
	}
	c.Sample(context.Background())
	if on, code := readPower(c); on || code != 0 || c.pending != nil {
		t.Fatal("offline state not published")
	}
	if len(notifications) != 2 || !notifications[0] || notifications[1] {
		t.Fatalf("observed-state notifications=%v", notifications)
	}
}
func TestPowerFaultClassificationRecoveryAndDebounce(t *testing.T) {
	c, b, _, alive, probeErr := fixturePower()
	c.Sample(context.Background())
	b.err = errors.New("HTTP 403")
	for i := 0; i < 4; i++ {
		c.Sample(context.Background())
	}
	if !c.on || c.misses != 0 {
		t.Fatal("API response classified offline")
	}
	if _, code := readPower(c); code == 0 {
		t.Fatal("fault not reported")
	}
	if remoteWrite(c, false) == 0 {
		t.Fatal("shutdown permitted during fault")
	}
	b.reachable = false
	*alive = true
	c.Sample(context.Background())
	if !c.on || c.misses != 0 {
		t.Fatal("ICMP reachable classified offline")
	}
	*alive = false
	c.Sample(context.Background())
	*probeErr = errors.New("probe failure")
	c.Sample(context.Background())
	if !c.on || c.misses != 0 {
		t.Fatal("probe error classified offline or failed to reset debounce")
	}
	*probeErr = nil
	for i := 0; i < 3; i++ {
		c.Sample(context.Background())
	}
	if c.on || !c.known {
		t.Fatal("offline not detected")
	}
	b.reachable, b.err = true, nil
	c.Sample(context.Background())
	if on, code := readPower(c); !on || code != 0 {
		t.Fatal("recovery failed")
	}
}
func TestPowerTimeoutFailureAndCanceledLifecycle(t *testing.T) {
	c, b, wakes, _, _ := fixturePower()
	c.Sample(context.Background())
	b.shutdownErr = errors.New("forbidden")
	if remoteWrite(c, false) == 0 || c.pending != nil {
		t.Fatal("failed command accepted")
	}
	b.shutdownErr = nil
	now := time.Now()
	c.now = func() time.Time { return now }
	if remoteWrite(c, false) != 0 {
		t.Fatal("shutdown failed")
	}
	now = now.Add(5 * time.Minute)
	c.Sample(context.Background())
	if c.pending != nil || !c.on {
		t.Fatal("timeout not cleared or state changed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Poll(ctx)
	if remoteWrite(c, false) == 0 || *wakes != 0 {
		t.Fatal("stopped controller accepted command")
	}
}
func TestPowerConcurrentRequestsAreMerged(t *testing.T) {
	c, b, _, _, _ := fixturePower()
	c.Sample(context.Background())
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.Request(false); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if b.shutdowns != 1 {
		t.Fatalf("shutdowns=%d", b.shutdowns)
	}
}

func TestWakeWaitsForAPIReadiness(t *testing.T) {
	c, b, _, alive, _ := fixturePower()
	b.reachable, b.err = false, errors.New("offline")
	for i := 0; i < 3; i++ {
		c.Sample(context.Background())
	}
	if remoteWrite(c, true) != 0 {
		t.Fatal("wake rejected")
	}
	*alive = true
	c.Sample(context.Background())
	if c.Accessory.Switch.On.Value() || c.pending == nil {
		t.Fatal("ICMP readiness completed wake early")
	}
	b.reachable, b.err = true, errors.New("HTTP 503")
	c.Sample(context.Background())
	if c.Accessory.Switch.On.Value() || c.pending == nil {
		t.Fatal("API fault completed wake early")
	}
	b.err = nil
	c.Sample(context.Background())
	if on, code := readPower(c); !on || code != 0 || c.pending != nil {
		t.Fatal("API readiness did not complete wake")
	}
}
