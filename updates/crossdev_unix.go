//go:build !windows

package updates

import (
	"errors"
	"syscall"
)

func isCrossDeviceError(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == syscall.EXDEV
}
