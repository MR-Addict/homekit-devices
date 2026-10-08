package chromecast

import (
	"context"
	"errors"
	"github.com/brutella/hap/accessory"
	"github.com/brutella/hap/characteristic"
	"github.com/brutella/hap/service"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"
)

type Backend interface {
	Status(context.Context) (Status, error)
	SetPower(context.Context, bool) error
	SendKey(context.Context, string) error
	LaunchApp(context.Context, string) error
	SetMute(context.Context, bool) error
}
type Controller struct {
	Accessory      *accessory.Television
	backend        Backend
	cfg            AppConfig
	mu             sync.Mutex
	status         Status
	observed       time.Time
	pending        *bool
	fault          error
	failedTarget   *bool
	ctx            context.Context
	stopped        bool
	commands       chan bool
	operations     chan struct{}
	RemoteKey      *characteristic.RemoteKey
	VolumeSelector *characteristic.VolumeSelector
}

func NewController(cfg AppConfig) *Controller {
	return NewControllerWithBackend(cfg, &Client{Config: cfg.Chromecast, Timeout: Duration(cfg.Control.Timeout)})
}
func NewControllerWithBackend(cfg AppConfig, b Backend) *Controller {
	c := &Controller{backend: b, cfg: cfg, ctx: context.Background(), commands: make(chan bool, 1), operations: make(chan struct{}, 1)}
	c.Accessory = accessory.NewTelevision(accessory.Info{Name: cfg.HomeKit.Name, SerialNumber: cfg.HomeKit.SerialNumber, Manufacturer: cfg.HomeKit.Manufacturer, Model: cfg.HomeKit.Model, Firmware: cfg.HomeKit.Firmware})
	c.Accessory.Television.Active.RemoteWriteFunc = func(v interface{}, _ *http.Request) (interface{}, int) {
		value, ok := v.(int)
		on := value == characteristic.ActiveActive
		if !ok || value < 0 || value > 1 {
			return nil, -70410
		}
		if err := c.Request(on); err != nil {
			log.Printf("Chromecast power request: %v", err)
			return nil, -70402
		}
		return nil, 0
	}
	c.Accessory.Television.Active.ValueRequestFunc = func(_ *http.Request) (interface{}, int) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.healthyLocked() || c.fault != nil {
			return nil, -70402
		}
		return activeValue(*c.status.On), 0
	}
	c.configureTelevision()
	return c
}
func (c *Controller) healthyLocked() bool {
	return c.status.Available && c.status.On != nil && time.Since(c.observed) <= Duration(c.cfg.Control.Interval)+2*Duration(c.cfg.Control.Timeout)
}
func (c *Controller) acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case c.operations <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-c.operations
			return err
		}
		return nil
	}
}
func (c *Controller) Sample(ctx context.Context) {
	if err := c.acquire(ctx); err != nil {
		return
	}
	defer func() { <-c.operations }()
	c.sample(ctx)
}
func (c *Controller) sample(ctx context.Context) {
	s, err := c.backend.Status(ctx)
	if ctx.Err() != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = s
	c.observed = time.Now()
	if err != nil {
		c.status.Available = false
		c.status.On = nil
		c.status.Error = err.Error()
		return
	}
	if s.Available {
		id := 0
		if s.App != nil {
			for _, app := range c.cfg.Apps {
				if app.Package == *s.App {
					id = app.ID
					break
				}
			}
		}
		c.Accessory.Television.ActiveIdentifier.SetValue(id)
		if s.Volume != nil {
			c.Accessory.Speaker.Mute.SetValue(s.Volume.Muted)
		}
	}
	if s.Available && s.On != nil {
		// Observed Remote v2 state only; never publish the requested value.
		c.Accessory.Television.Active.SetValue(activeValue(*s.On))
		if c.failedTarget != nil && *s.On == *c.failedTarget {
			c.fault = nil
			c.failedTarget = nil
		}
	}
}
func (c *Controller) Request(on bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped || c.ctx.Err() != nil {
		return errors.New("Chromecast controller stopped")
	}
	if c.pending != nil {
		if *c.pending == on {
			return nil
		}
		return errors.New("opposite power operation already in progress")
	}
	if c.healthyLocked() && c.fault == nil && *c.status.On == on {
		return nil
	}
	c.pending = new(bool)
	*c.pending = on
	c.commands <- on
	return nil
}
func (c *Controller) command(ctx context.Context, on bool) {
	op, cancel := context.WithTimeout(ctx, Duration(c.cfg.Control.TransitionTimeout))
	err := c.acquire(op)
	if err == nil {
		err = c.backend.SetPower(op, on)
		c.sample(ctx)
		<-c.operations
	}
	cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = nil
	if err == nil && (!c.status.Available || c.status.On == nil || *c.status.On != on) {
		err = errors.New("Chromecast final power verification failed")
	}
	c.fault = err
	c.failedTarget = nil
	if err != nil {
		c.failedTarget = new(bool)
		*c.failedTarget = on
		log.Printf("Chromecast on=%t failed: %v", on, err)
	} else {
		log.Printf("Chromecast on=%t confirmed", on)
	}
}
func (c *Controller) Poll(ctx context.Context) {
	c.mu.Lock()
	c.ctx = ctx
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.stopped = true; c.pending = nil; c.mu.Unlock() }()
	c.Sample(ctx)
	ticker := time.NewTicker(Duration(c.cfg.Control.Interval))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case on := <-c.commands:
			if ctx.Err() == nil {
				c.command(ctx, on)
			}
		case <-ticker.C:
			c.Sample(ctx)
		}
	}
}

func activeValue(on bool) int {
	if on {
		return characteristic.ActiveActive
	}
	return characteristic.ActiveInactive
}

var remoteKeys = map[int]string{
	characteristic.RemoteKeyArrowUp: "DPAD_UP", characteristic.RemoteKeyArrowDown: "DPAD_DOWN",
	characteristic.RemoteKeyArrowLeft: "DPAD_LEFT", characteristic.RemoteKeyArrowRight: "DPAD_RIGHT",
	characteristic.RemoteKeySelect: "DPAD_CENTER", characteristic.RemoteKeyBack: "BACK",
	characteristic.RemoteKeyExit: "HOME", characteristic.RemoteKeyPlayPause: "MEDIA_PLAY_PAUSE",
	characteristic.RemoteKeyRewind: "MEDIA_REWIND", characteristic.RemoteKeyFastForward: "MEDIA_FAST_FORWARD",
	characteristic.RemoteKeyNextTrack: "MEDIA_NEXT", characteristic.RemoteKeyPrevTrack: "MEDIA_PREVIOUS",
	// iOS labels this button "i"; use it as the Android TV Home button.
	characteristic.RemoteKeyInfo: "HOME",
}

// execute bounds both waiting for other operations and the command itself.
// RemoteWriteFunc preserves observed values and handles repeated button writes.
func (c *Controller) execute(timeout time.Duration, fn func(context.Context) error) (interface{}, int) {
	c.mu.Lock()
	parent, stopped := c.ctx, c.stopped
	c.mu.Unlock()
	if stopped {
		return nil, -70402
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if err := c.acquire(ctx); err != nil {
		return nil, -70402
	}
	defer func() { <-c.operations }()
	err := fn(ctx)
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		log.Printf("Chromecast remote command: %v", err)
		return nil, -70402
	}
	return nil, 0
}

func (c *Controller) configureTelevision() {
	tv := c.Accessory.Television
	tv.Primary = true
	tv.ConfiguredName.SetValue(c.cfg.HomeKit.Name)
	tv.ConfiguredName.Permissions = []string{characteristic.PermissionRead, characteristic.PermissionEvents}
	tv.SleepDiscoveryMode.SetValue(characteristic.SleepDiscoveryModeAlwaysDiscoverable)
	c.RemoteKey = characteristic.NewRemoteKey()
	tv.AddC(c.RemoteKey.C)
	timeout := Duration(c.cfg.Control.Timeout) + time.Second
	c.RemoteKey.RemoteWriteFunc = func(v interface{}, _ *http.Request) (interface{}, int) {
		n, ok := v.(int)
		key, exists := remoteKeys[n]
		if !ok || !exists {
			return nil, -70410
		}
		return c.execute(timeout, func(ctx context.Context) error { return c.backend.SendKey(ctx, key) })
	}
	speaker := c.Accessory.Speaker
	tv.AddS(speaker.S)
	active := characteristic.NewActive()
	active.SetValue(characteristic.ActiveActive)
	active.Permissions = []string{characteristic.PermissionRead, characteristic.PermissionEvents}
	speaker.AddC(active.C)
	volumeType := characteristic.NewVolumeControlType()
	volumeType.SetValue(characteristic.VolumeControlTypeRelative)
	speaker.AddC(volumeType.C)
	c.VolumeSelector = characteristic.NewVolumeSelector()
	speaker.AddC(c.VolumeSelector.C)
	c.VolumeSelector.RemoteWriteFunc = func(v interface{}, _ *http.Request) (interface{}, int) {
		n, ok := v.(int)
		if !ok || n < 0 || n > 1 {
			return nil, -70410
		}
		key := "VOLUME_UP"
		if n == characteristic.VolumeSelectorDecrement {
			key = "VOLUME_DOWN"
		}
		return c.execute(timeout, func(ctx context.Context) error { return c.backend.SendKey(ctx, key) })
	}
	speaker.Mute.RemoteWriteFunc = func(v interface{}, _ *http.Request) (interface{}, int) {
		muted, ok := v.(bool)
		if !ok {
			return nil, -70410
		}
		return c.execute(timeout, func(ctx context.Context) error { return c.backend.SetMute(ctx, muted) })
	}
	speaker.Mute.ValueRequestFunc = func(_ *http.Request) (interface{}, int) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.healthyLocked() || c.status.Volume == nil {
			return nil, -70402
		}
		return c.status.Volume.Muted, 0
	}
	// Sorting by ID also keeps service/IID ordering stable across YAML reorderings.
	apps := append([]AppInput(nil), c.cfg.Apps...)
	sort.Slice(apps, func(i, j int) bool { return apps[i].ID < apps[j].ID })
	for _, app := range apps {
		input := service.NewInputSource()
		input.ConfiguredName.SetValue(app.Name)
		input.ConfiguredName.Permissions = []string{characteristic.PermissionRead, characteristic.PermissionEvents}
		input.InputSourceType.SetValue(characteristic.InputSourceTypeApplication)
		input.IsConfigured.SetValue(characteristic.IsConfiguredConfigured)
		input.CurrentVisibilityState.SetValue(characteristic.CurrentVisibilityStateShown)
		id := characteristic.NewIdentifier()
		id.SetValue(app.ID)
		input.AddC(id.C)
		c.Accessory.AddS(input.S)
		tv.AddS(input.S)
	}
	tv.ActiveIdentifier.ValueRequestFunc = func(_ *http.Request) (interface{}, int) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.healthyLocked() {
			return nil, -70402
		}
		return tv.ActiveIdentifier.Value(), 0
	}
	tv.ActiveIdentifier.RemoteWriteFunc = func(v interface{}, _ *http.Request) (interface{}, int) {
		id, ok := v.(int)
		if !ok {
			return nil, -70410
		}
		for _, app := range apps {
			if app.ID != id {
				continue
			}
			return c.execute(Duration(c.cfg.Control.TransitionTimeout)+timeout, func(ctx context.Context) error {
				power, cancel := context.WithTimeout(ctx, Duration(c.cfg.Control.TransitionTimeout))
				err := c.backend.SetPower(power, true)
				cancel()
				if err != nil {
					return err
				}
				launch, cancel := context.WithTimeout(ctx, timeout)
				defer cancel()
				return c.backend.LaunchApp(launch, app.Launch)
			})
		}
		return nil, -70410
	}
}
