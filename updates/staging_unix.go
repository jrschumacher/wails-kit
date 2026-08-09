//go:build !windows

package updates

import (
	"fmt"
	"os"
	"syscall"
)

// verifyPrivateDir checks that dir is owned by the current user and is not
// group- or world-accessible. os.MkdirAll alone is not sufficient: it
// succeeds silently when the directory already exists, regardless of who
// created it or with what permissions. On a shared, world-writable parent
// (Linux's /tmp is mode 1777) a local attacker can pre-create the app's
// staging directory and own it — this closes that hole by refusing to use
// a directory this process does not exclusively own. See AGENTS.md,
// "Linux /tmp TOCTOU".
func verifyPrivateDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("stat staging dir: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("staging path %q is not a directory", dir)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		// Unknown Sys() implementation for this platform variant; nothing
		// more we can check.
		return nil
	}
	if int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("staging dir %q is not owned by the current user (uid %d, want %d) — refusing to use a directory another user could have pre-created", dir, stat.Uid, os.Getuid())
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("staging dir %q is group/world accessible (mode %04o) — refusing", dir, info.Mode().Perm())
	}
	return nil
}
