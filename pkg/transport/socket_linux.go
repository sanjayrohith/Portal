//go:build linux

package transport

import (
	"syscall"
	"time"
)

// tcpUserTimeoutOpt corresponds to TCP_USER_TIMEOUT (18 on Linux).
const tcpUserTimeoutOpt = 18

func setTCPUserTimeout(fd uintptr, timeout time.Duration) error {
	if timeout <= 0 {
		return nil
	}
	msec := int(timeout.Milliseconds())
	return syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, tcpUserTimeoutOpt, msec)
}
