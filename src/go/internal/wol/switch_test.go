package wol

import (
	"context"
	"errors"
	"github.com/brutella/hap/accessory"
	"net/http"
	"testing"
	"time"
)

func TestWakeSwitchTriggerAndReset(t *testing.T) {
	calls := 0
	a := NewWakeSwitch(accessory.Info{Name: "PC"}, 10*time.Millisecond, func(context.Context) error { calls++; return nil })
	if a.Switch.On.Value() {
		t.Fatal("initial switch on")
	}
	if _, status := a.Switch.On.SetValueRequestFunc(false, nil); status != 0 || calls != 0 {
		t.Fatal("off triggered wake")
	}
	a.Switch.On.SetValue(true)
	reset := make(chan struct{}, 1)
	a.Switch.On.OnValueUpdate(func(new, old bool, _ *http.Request) {
		if !new && old {
			reset <- struct{}{}
		}
	})
	if _, status := a.Switch.On.SetValueRequestFunc(true, nil); status != 0 || calls != 1 {
		t.Fatal("wake failed")
	}
	select {
	case <-reset:
	case <-time.After(time.Second):
		t.Fatal("switch did not reset")
	}
	if a.Switch.On.Value() {
		t.Fatal("switch still on")
	}
}
func TestWakeSwitchPropagatesSendFailure(t *testing.T) {
	a := NewWakeSwitch(accessory.Info{Name: "PC"}, DefaultResetDelay, func(context.Context) error { return errors.New("send failed") })
	if _, status := a.Switch.On.SetValueRequestFunc(true, nil); status == 0 {
		t.Fatal("send failure hidden")
	}
}
