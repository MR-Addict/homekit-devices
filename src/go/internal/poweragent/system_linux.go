package poweragent

import (
	"context"
	"os"
	"strings"

	"github.com/godbus/dbus/v5"
)

type SystemBackend struct{ conn *dbus.Conn }

func NewSystemBackend() (*SystemBackend, string, error) {
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return nil, "", err
	}
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, "", err
	}
	return &SystemBackend{conn: conn}, strings.TrimSpace(string(boot)), nil
}

func (b *SystemBackend) Shutdown(ctx context.Context) error {
	// The unprivileged service account has only power-off permissions, never
	// power-off-ignore-inhibit. logind retains control over shutdown inhibitors.
	return b.conn.Object("org.freedesktop.login1", "/org/freedesktop/login1").CallWithContext(ctx, "org.freedesktop.login1.Manager.PowerOff", 0, false).Err
}

func (b *SystemBackend) Close() { _ = b.conn.Close() }
