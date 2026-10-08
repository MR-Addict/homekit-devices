//go:build !darwin && !linux

package chromecast

import (
	"context"
	"errors"
)

func sessionLock(context.Context, string) (func(), error) {
	return nil, errors.New("Chromecast controller currently supports Linux and macOS")
}
