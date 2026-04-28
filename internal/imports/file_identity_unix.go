//go:build !windows

package imports

import (
	"os"
	"syscall"
)

// fileIdentity returns the inode and device IDs for a file. Used as part of
// the fast-path "unchanged file" check in the importer. On non-Unix systems
// (or when the underlying os.FileInfo.Sys() value is not a *syscall.Stat_t)
// both values are zero, in which case callers fall back to size+mtime+hash.
func fileIdentity(info os.FileInfo) (uint64, uint64) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0
	}
	return uint64(stat.Ino), uint64(stat.Dev)
}
