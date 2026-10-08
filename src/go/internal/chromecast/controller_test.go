package chromecast

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeBackend struct {
	mu                    sync.Mutex
	on                    bool
	statusErr, errorOnSet error
	sets                  []bool
	block                 <-chan struct{}
}

func (b *fakeBackend) Status(context.Context) (Status, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	on := b.on
	if b.statusErr != nil {
		return Status{Error: b.statusErr.Error()}, b.statusErr
	}
	return Status{Available: true, On: &on}, nil
}
func (b *fakeBackend) SetPower(ctx context.Context, on bool) error {
	if b.block != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-b.block:
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sets = append(b.sets, on)
	if b.errorOnSet != nil {
		return b.errorOnSet
	}
	b.on = on
	return nil
}
func configForTest() AppConfig {
	return AppConfig{Control: ControlConfig{Interval: "5s", Timeout: "100ms", TransitionTimeout: "200ms"}}
}
func TestNoOptimisticPublishAndRequestDedup(t *testing.T) {
	b := &fakeBackend{}
	c := NewControllerWithBackend(configForTest(), b)
	c.Sample(context.Background())
	if e := c.Request(true); e != nil {
		t.Fatal(e)
	}
	if e := c.Request(true); e != nil {
		t.Fatal(e)
	}
	if e := c.Request(false); e == nil {
		t.Fatal("accepted opposite command")
	}
	if c.Accessory.Television.Active.Value() == 1 {
		t.Fatal("optimistically published on")
	}
	if len(c.commands) != 1 {
		t.Fatal("duplicate command queued")
	}
	on := <-c.commands
	c.command(context.Background(), on)
	if c.Accessory.Television.Active.Value() != 1 {
		t.Fatal("observed on not published")
	}
	if e := c.Request(true); e != nil || len(c.commands) != 0 {
		t.Fatal("same-state command repeated", e)
	}
}
func TestUnavailableIsNotOffAndRecovers(t *testing.T) {
	b := &fakeBackend{on: true}
	c := NewControllerWithBackend(configForTest(), b)
	if _, code := c.Accessory.Television.Active.ValueRequestFunc(nil); code == 0 {
		t.Fatal("initial state falsely known")
	}
	c.Sample(context.Background())
	b.statusErr = errors.New("offline")
	c.Sample(context.Background())
	if _, code := c.Accessory.Television.Active.ValueRequestFunc(nil); code == 0 {
		t.Fatal("offline read succeeded")
	}
	if c.Accessory.Television.Active.Value() != 1 {
		t.Fatal("offline interpreted as off")
	}
	b.statusErr = nil
	b.on = false
	c.Sample(context.Background())
	if value, code := c.Accessory.Television.Active.ValueRequestFunc(nil); code != 0 || value != 0 {
		t.Fatal(value, code)
	}
}
func TestOperationFailureAndLaterReconciliation(t *testing.T) {
	b := &fakeBackend{errorOnSet: errors.New("ambiguous write")}
	c := NewControllerWithBackend(configForTest(), b)
	c.Sample(context.Background())
	c.Request(true)
	c.command(context.Background(), <-c.commands)
	if _, code := c.Accessory.Television.Active.ValueRequestFunc(nil); code == 0 {
		t.Fatal("failed command not surfaced")
	}
	if c.Accessory.Television.Active.Value() == 1 {
		t.Fatal("failed command published target")
	}
	b.on = true
	c.Sample(context.Background())
	if v, code := c.Accessory.Television.Active.ValueRequestFunc(nil); code != 0 || v != 1 {
		t.Fatal("did not reconcile eventual observation", v, code)
	}
}
func TestPollCancellationStopsWorker(t *testing.T) {
	block := make(chan struct{})
	b := &fakeBackend{block: block}
	c := NewControllerWithBackend(configForTest(), b)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Poll(ctx); close(done) }()
	c.Request(true)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("poller survived cancellation")
	}
	if e := c.Request(false); e == nil {
		t.Fatal("stopped poller accepted command")
	}
}
func TestStaleObservedStateReturnsCommunicationError(t *testing.T) {
	c := NewControllerWithBackend(configForTest(), &fakeBackend{on: true})
	c.Sample(context.Background())
	c.mu.Lock()
	c.observed = time.Now().Add(-time.Minute)
	c.mu.Unlock()
	if _, code := c.Accessory.Television.Active.ValueRequestFunc(nil); code == 0 {
		t.Fatal("stale status treated as fresh")
	}
}
func TestConfigurationRejectsTVAndInvalidFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	valid := "chromecast:\n  host: 192.168.10.208\n"
	for _, tc := range []struct {
		input string
		valid bool
	}{{valid, true}, {valid + "tv:\n  host: 192.168.10.173\n", false}, {valid + "---\n{}\n", false}, {valid + "control:\n  interval: -1s\n", false}, {"chromecast:\n  host: 'cc:6466'\n", false}, {valid + "control:\n  unknown: true\n", false}} {
		os.WriteFile(path, []byte(tc.input), 0600)
		c, e := Load(path)
		if (e == nil) != tc.valid {
			t.Fatalf("%q: %v", tc.input, e)
		}
		if tc.valid && (!filepath.IsAbs(c.Chromecast.KeyPath) || Duration(c.Control.Interval) != 5*time.Second || c.Chromecast.PowerMode != "toggle") {
			t.Fatal("invalid defaults")
		}
	}
}
func TestSessionLockBoundedAndReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	unlock, e := sessionLock(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	if release, e := sessionLock(ctx, path); e == nil {
		release()
		t.Fatal("same credential concurrently locked")
	}
	unlock()
	release, e := sessionLock(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	release()
}

func (b *fakeBackend) SendKey(context.Context, string) error   { return nil }
func (b *fakeBackend) LaunchApp(context.Context, string) error { return nil }
func (b *fakeBackend) SetMute(context.Context, bool) error     { return nil }
