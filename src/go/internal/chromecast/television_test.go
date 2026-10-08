package chromecast

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/brutella/hap/accessory"
	"github.com/brutella/hap/characteristic"
	"github.com/brutella/hap/service"
	"github.com/jkiddo/atvremote/pkg/v2/remote"
)

type tvBackend struct {
	fakeBackend
	events     []string
	app        *string
	volume     *remote.VolumeInfo
	commandErr error
	hold       chan struct{}
	entered    chan struct{}
}

func (b *tvBackend) Status(ctx context.Context) (Status, error) {
	s, err := b.fakeBackend.Status(ctx)
	b.mu.Lock()
	defer b.mu.Unlock()
	s.App, s.Volume = b.app, b.volume
	return s, err
}
func (b *tvBackend) SetPower(ctx context.Context, on bool) error {
	b.mu.Lock()
	b.events = append(b.events, "power")
	b.mu.Unlock()
	return b.fakeBackend.SetPower(ctx, on)
}
func (b *tvBackend) record(ctx context.Context, event string) error {
	b.mu.Lock()
	b.events = append(b.events, event)
	err := b.commandErr
	b.mu.Unlock()
	if b.entered != nil {
		b.entered <- struct{}{}
	}
	if b.hold != nil {
		select {
		case <-b.hold:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}
func (b *tvBackend) SendKey(ctx context.Context, k string) error   { return b.record(ctx, k) }
func (b *tvBackend) LaunchApp(ctx context.Context, a string) error { return b.record(ctx, "launch:"+a) }
func (b *tvBackend) SetMute(ctx context.Context, m bool) error     { return b.record(ctx, "mute") }

func write(t *testing.T, c *characteristic.C, v any, want int) {
	t.Helper()
	if _, code := c.SetValueRequest(v, &http.Request{}); code != want {
		t.Fatalf("write %v: code %d, want %d", v, code, want)
	}
}
func tvConfig() AppConfig {
	cfg := configForTest()
	cfg.HomeKit.Name = "Chromecast"
	cfg.Apps = []AppInput{{ID: 9, Name: "YouTube", Launch: "com.google.android.youtube.tv", Package: "com.google.android.youtube.tv"}, {ID: 3, Name: "Netflix", Launch: "com.netflix.ninja", Package: "com.netflix.ninja"}}
	return cfg
}
func TestTelevisionServicesAndRemoteMappings(t *testing.T) {
	b := &tvBackend{}
	c := NewControllerWithBackend(tvConfig(), b)
	tv := c.Accessory.Television
	if c.Accessory.Type != accessory.TypeTelevision || !tv.Primary {
		t.Fatal("not a primary television")
	}
	if tv.ConfiguredName.Value() != "Chromecast" || tv.SleepDiscoveryMode.Value() != 1 {
		t.Fatal("invalid television metadata")
	}
	if len(tv.Linked) != 3 || tv.Linked[0] != c.Accessory.Speaker.S {
		t.Fatal("speaker/inputs not linked")
	}
	for i, id := range []int{3, 9} {
		input := tv.Linked[i+1]
		if input.Type != service.TypeInputSource {
			t.Fatal("wrong linked input type")
		}
		found := false
		for _, ch := range input.Cs {
			if ch.Type == characteristic.TypeIdentifier {
				found = ch.Value() == id
			}
			if ch.Type == characteristic.TypeConfiguredName && !reflect.DeepEqual(ch.Permissions, []string{characteristic.PermissionRead, characteristic.PermissionEvents}) {
				t.Fatal("input name writable")
			}
		}
		if !found {
			t.Fatal("input identifier missing or unstable")
		}
	}
	cases := []struct {
		id  int
		key string
	}{{4, "DPAD_UP"}, {5, "DPAD_DOWN"}, {6, "DPAD_LEFT"}, {7, "DPAD_RIGHT"}, {8, "DPAD_CENTER"}, {9, "BACK"}, {10, "HOME"}, {11, "MEDIA_PLAY_PAUSE"}, {0, "MEDIA_REWIND"}, {1, "MEDIA_FAST_FORWARD"}, {2, "MEDIA_NEXT"}, {3, "MEDIA_PREVIOUS"}, {15, "HOME"}}
	for _, tc := range cases {
		if _, err := remote.ParseKeyCode(tc.key); err != nil {
			t.Fatal(err)
		}
		write(t, c.RemoteKey.C, tc.id, 0)
		write(t, c.RemoteKey.C, tc.id, 0)
		if !reflect.DeepEqual(b.events[len(b.events)-2:], []string{tc.key, tc.key}) {
			t.Fatal("wrong mapping or suppressed repeat", tc)
		}
	}
	write(t, c.RemoteKey.C, 12, -70410)
	write(t, c.VolumeSelector.C, 0, 0)
	write(t, c.VolumeSelector.C, 0, 0)
	write(t, c.VolumeSelector.C, 1, 0)
	if !reflect.DeepEqual(b.events[len(b.events)-3:], []string{"VOLUME_UP", "VOLUME_UP", "VOLUME_DOWN"}) {
		t.Fatal("volume commands lost")
	}
	if len(b.sets) != 0 {
		t.Fatal("buttons woke device")
	}
	b.commandErr = errors.New("unsupported")
	write(t, c.RemoteKey.C, 4, -70402)
}
func TestApplicationWakeLaunchAndObservedIdentifier(t *testing.T) {
	b := &tvBackend{}
	c := NewControllerWithBackend(tvConfig(), b)
	c.Sample(context.Background())
	write(t, c.Accessory.Television.ActiveIdentifier.C, 9, 0)
	if !reflect.DeepEqual(b.events, []string{"power", "launch:com.google.android.youtube.tv"}) {
		t.Fatal(b.events)
	}
	if c.Accessory.Television.ActiveIdentifier.Value() != 0 || c.Accessory.Television.Active.Value() != 0 {
		t.Fatal("optimistic app or power state")
	}
	app := "com.google.android.youtube.tv"
	b.app = &app
	c.Sample(context.Background())
	if c.Accessory.Television.ActiveIdentifier.Value() != 9 || c.Accessory.Television.Active.Value() != 1 {
		t.Fatal("observed state not published")
	}
	write(t, c.Accessory.Television.ActiveIdentifier.C, 9, 0)
	if len(b.events) != 4 {
		t.Fatal("same application selection suppressed")
	}
	app = "unknown"
	c.Sample(context.Background())
	if c.Accessory.Television.ActiveIdentifier.Value() != 0 {
		t.Fatal("unknown application not cleared")
	}
	b.app = nil
	c.Sample(context.Background())
	write(t, c.Accessory.Television.ActiveIdentifier.C, 0, -70410)
	write(t, c.Accessory.Television.ActiveIdentifier.C, 99, -70410)
	b.errorOnSet = errors.New("cannot wake")
	before := len(b.events)
	write(t, c.Accessory.Television.ActiveIdentifier.C, 3, -70402)
	if len(b.events) != before+1 || b.events[before] != "power" {
		t.Fatal("launched after failed wake")
	}
	b.statusErr = errors.New("offline")
	c.Sample(context.Background())
	if _, code := c.Accessory.Television.ActiveIdentifier.ValueRequestFunc(nil); code != -70402 {
		t.Fatal("offline app read succeeded")
	}
}
func TestMuteReadsOnlyObservedState(t *testing.T) {
	b := &tvBackend{}
	c := NewControllerWithBackend(tvConfig(), b)
	c.Sample(context.Background())
	if _, code := c.Accessory.Speaker.Mute.ValueRequestFunc(nil); code != -70402 {
		t.Fatal("unknown mute state read succeeded")
	}
	b.volume = &remote.VolumeInfo{Max: 100, Muted: true}
	c.Sample(context.Background())
	if v, code := c.Accessory.Speaker.Mute.ValueRequestFunc(nil); v != true || code != 0 {
		t.Fatal(v, code)
	}
	write(t, c.Accessory.Speaker.Mute.C, false, 0)
	if !c.Accessory.Speaker.Mute.Value() {
		t.Fatal("optimistic mute publication")
	}
}
func TestRemoteOperationsSerializeTimeoutAndCancel(t *testing.T) {
	b := &tvBackend{hold: make(chan struct{}), entered: make(chan struct{}, 2)}
	c := NewControllerWithBackend(tvConfig(), b)
	ctx, cancel := context.WithCancel(context.Background())
	c.ctx = ctx
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.execute(time.Second, func(ctx context.Context) error { return b.SendKey(ctx, "DPAD_UP") })
	}()
	<-b.entered
	if _, code := c.execute(20*time.Millisecond, func(ctx context.Context) error { return b.SendKey(ctx, "DPAD_DOWN") }); code != -70402 {
		t.Fatal("waiting operation did not time out")
	}
	b.mu.Lock()
	if !reflect.DeepEqual(b.events, []string{"DPAD_UP"}) {
		t.Fatal(b.events)
	}
	b.mu.Unlock()
	cancel()
	wg.Wait()
	write(t, c.RemoteKey.C, 4, -70402)
	if len(b.events) != 1 {
		t.Fatal("command ran after cancel")
	}
	c.stopped = true
	write(t, c.VolumeSelector.C, 0, -70402)
}
func TestAppConfigurationValidation(t *testing.T) {
	valid := "chromecast:\n  host: localhost\n"
	app := "apps:\n  - id: 7\n    name: YouTube\n    launch: com.google.android.youtube.tv\n    package: com.google.android.youtube.tv\n"
	for _, tc := range []struct {
		yaml string
		ok   bool
	}{
		{valid, true}, {valid + app, true},
		{valid + "apps: [{id: 0, name: A, launch: a, package: a}]", false},
		{valid + "apps: [{id: 1, name: ' ', launch: a, package: a}]", false},
		{valid + "apps: [{id: 1, name: A, launch: '', package: a}]", false},
		{valid + "apps: [{id: 1, name: A, launch: a}]", false},
		{valid + "apps: [{id: 1, name: A, launch: a, package: a}, {id: 1, name: B, launch: b, package: b}]", false},
		{valid + "apps: [{id: 1, name: A, launch: a, package: a}, {id: 2, name: B, launch: b, package: a}]", false},
		{valid + "apps: [{id: 1, name: A, launch: a, package: a, unknown: true}]", false},
	} {
		p := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(p, []byte(tc.yaml), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(p)
		if (err == nil) != tc.ok {
			t.Fatalf("%s: %v", tc.yaml, err)
		}
		if tc.ok && len(cfg.Apps) > 0 && cfg.Apps[0].ID != 7 {
			t.Fatal("app not loaded")
		}
	}
}
