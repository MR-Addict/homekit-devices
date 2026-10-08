package wol

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/brutella/hap/accessory"
)

type powerBackend interface {
	Status(context.Context) (bool, error)
	Shutdown(context.Context) error
}

// PowerController serializes probes and commands. Only Sample publishes values;
// command writes never publish the desired state as observed state.
type PowerController struct {
	Accessory *accessory.Switch
	mu        sync.Mutex
	backend   powerBackend
	wake      WakeFunc
	ping      func(context.Context) (bool, error)
	ctx       context.Context
	known, on bool
	fault     error
	misses    int
	pending   *bool
	deadline  time.Time
	now       func() time.Time
	timeout   time.Duration
}

func newPowerController(info accessory.Info, b powerBackend, wake WakeFunc, ping func(context.Context) (bool, error)) *PowerController {
	c := &PowerController{Accessory: accessory.NewSwitch(info), backend: b, wake: wake, ping: ping, ctx: context.Background(), now: time.Now, timeout: 5 * time.Minute}
	c.Accessory.Switch.On.SetValue(false)
	c.Accessory.Switch.On.RemoteWriteFunc = func(v interface{}, _ *http.Request) (interface{}, int) {
		if err := c.Request(v.(bool)); err != nil {
			log.Printf("power command %q: %v", info.Name, err)
			return nil, -70402
		}
		return nil, 0
	}
	c.Accessory.Switch.On.ValueRequestFunc = func(_ *http.Request) (interface{}, int) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.known || c.fault != nil {
			return nil, -70402
		}
		return c.on, 0
	}
	return c
}
func (c *PowerController) Request(on bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx.Err() != nil {
		return errors.New("power controller stopped")
	}
	if c.pending != nil {
		if *c.pending == on {
			return nil
		}
		return errors.New("power operation in progress")
	}
	if c.known && c.on == on {
		return nil
	}
	if !on && (!c.known || c.fault != nil) {
		return errors.New("cannot shut down without healthy observed state")
	}
	ctx, cancel := context.WithTimeout(c.ctx, 3*time.Second)
	defer cancel()
	var err error
	if on {
		err = c.wake(ctx)
	} else {
		err = c.backend.Shutdown(ctx)
	}
	if err != nil {
		return err
	}
	c.pending = new(bool)
	*c.pending = on
	c.deadline = c.now().Add(c.timeout)
	log.Printf("power %q: command accepted, waiting for on=%t", c.Accessory.Name(), on)
	return nil
}
func (c *PowerController) Sample(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	previousKnown, previousOn := c.known, c.on
	reachable, err := c.backend.Status(ctx)
	if ctx.Err() != nil {
		return
	}
	switch {
	case err == nil:
		c.known, c.on, c.fault, c.misses = true, true, nil, 0
	case reachable:
		c.known, c.on, c.fault, c.misses = true, true, err, 0
	default:
		alive, probeErr := c.ping(ctx)
		if ctx.Err() != nil {
			return
		}
		if probeErr != nil {
			c.fault = probeErr
			c.misses = 0
		} else if alive {
			c.known, c.on, c.fault, c.misses = true, true, err, 0
		} else {
			c.misses++
			if c.misses >= 3 {
				c.known, c.on, c.fault = true, false, nil
			} else {
				c.fault = err
			}
		}
	}
	// During our wake operation, network reachability alone is not readiness.
	// Preserve the last observed value until the node status API succeeds.
	if c.pending != nil && *c.pending && err != nil {
		c.known, c.on = previousKnown, previousOn
	}
	if c.known {
		if !previousKnown || previousOn != c.on {
			log.Printf("power %q: observed on=%t", c.Accessory.Name(), c.on)
		}
		c.Accessory.Switch.On.SetValue(c.on)
	}
	if c.pending != nil {
		if c.known && c.fault == nil && c.on == *c.pending {
			log.Printf("power %q: operation completed, on=%t", c.Accessory.Name(), c.on)
			c.pending = nil
		} else if !c.now().Before(c.deadline) {
			log.Printf("power %q: operation timed out", c.Accessory.Name())
			c.pending = nil
		}
	}
	if c.fault != nil {
		log.Printf("power %q: %v", c.Accessory.Name(), c.fault)
	}
}
func (c *PowerController) Poll(ctx context.Context) {
	c.mu.Lock()
	c.ctx = ctx
	c.mu.Unlock()
	c.Sample(ctx)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	defer func() {
		if closer, ok := c.backend.(interface{ Close() }); ok {
			closer.Close()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.Sample(ctx)
		}
	}
}
