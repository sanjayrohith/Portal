//go:build !linux

package transport

import (
	"time"
)

func setTCPUserTimeout(fd uintptr, timeout time.Duration) error {
	return nil
}
