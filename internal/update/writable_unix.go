//go:build !windows

package update

import "golang.org/x/sys/unix"

// canWriteDir checks directory writability WITHOUT creating anything
// (review 064 blocker 1: a transient probe file is still a write, which
// breaks the --dry-run zero-write contract). unix.Access consults the
// effective uid and catches read-only mounts (EROFS) too.
func canWriteDir(dir string) bool {
	return unix.Access(dir, unix.W_OK) == nil
}
