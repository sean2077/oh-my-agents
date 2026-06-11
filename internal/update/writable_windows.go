//go:build windows

package update

import "os"

// canWriteDir on Windows approximates writability from mode bits without
// creating anything (zero-write contract). Windows is currently a
// build-target-only platform; real-machine verification is a Phase D
// item.
func canWriteDir(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir() && info.Mode().Perm()&0o200 != 0
}
