package wol

import (
	"context"
	"time"

	"github.com/brutella/hap/accessory"
)

const DefaultResetDelay = time.Second * 2

type WakeFunc func(context.Context) error

func NewWakeSwitch(info accessory.Info, resetDelay time.Duration, wake WakeFunc) *accessory.Switch {
	if resetDelay <= 0 {
		resetDelay = DefaultResetDelay
	}
	if wake == nil {
		wake = func(context.Context) error { return nil }
	}

	a := accessory.NewSwitch(info)
	a.Switch.On.SetValue(false)
	a.Switch.On.OnSetRemoteValue(func(on bool) error {
		if !on {
			return nil
		}

		if err := wake(context.Background()); err != nil {
			return err
		}

		time.AfterFunc(resetDelay, func() {
			a.Switch.On.SetValue(false)
		})

		return nil
	})

	return a
}
