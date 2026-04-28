//go:build windows

package imports

import "os"

// fileIdentity returns zero values on Windows; the importer falls back to
// the size+mtime+hash path.
func fileIdentity(_ os.FileInfo) (uint64, uint64) {
	return 0, 0
}
