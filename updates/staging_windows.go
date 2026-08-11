//go:build windows

package updates

// verifyPrivateDir is a best-effort no-op on Windows. There is no direct
// analogue to POSIX uid/mode bits, and the default ACLs under a user's
// profile directory (AppData\Local, which is what appdirs.Cache() resolves
// to) already restrict access to the owning user and Administrators. If
// this needs stronger enforcement later, inspect the security descriptor
// via golang.org/x/sys/windows.
func verifyPrivateDir(dir string) error {
	return nil
}
