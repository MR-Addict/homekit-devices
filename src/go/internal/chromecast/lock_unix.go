//go:build darwin || linux

package chromecast

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"time"
)

// The persistent lock file protects a credential identity across the service
// and CLI processes. Never unlink it: another process may be waiting on it.
func sessionLock(ctx context.Context, path string) (func(), error) {
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		if err = ctx.Err(); err != nil {
			file.Close()
			return nil, err
		}
		err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() { unix.Flock(int(file.Fd()), unix.LOCK_UN); file.Close() }, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			file.Close()
			return nil, fmt.Errorf("credential session lock: %w", err)
		}
		if err = Sleep(ctx, 50*time.Millisecond); err != nil {
			file.Close()
			return nil, fmt.Errorf("another Chromecast session is busy: %w", err)
		}
	}
}
