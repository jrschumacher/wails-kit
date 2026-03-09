//go:build windows

package updates

import (
	"errors"
	"syscall"
)

func isCrossDeviceError(err error) bool {
	// Windows error code 0x11 = ERROR_NOT_SAME_DEVICE
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == 0x11
}
