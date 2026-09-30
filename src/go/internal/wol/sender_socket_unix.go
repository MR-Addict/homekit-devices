//go:build !windows

package wol

import "syscall"

func enableBroadcastSocket(_, _ string, rawConn syscall.RawConn) error {
	var controlErr error
	if err := rawConn.Control(func(fd uintptr) {
		controlErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	}); err != nil {
		return err
	}

	return controlErr
}
