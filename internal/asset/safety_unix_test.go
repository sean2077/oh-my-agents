//go:build !windows

package asset

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestPayloadSpecialFileRefused(t *testing.T) {
	e := newTestEngine(t)
	root := t.TempDir()
	src := writeSkillSource(t, root, "x", "body")
	if err := syscall.Mkfifo(filepath.Join(src, "pipe"), 0o600); err != nil {
		t.Skip("mkfifo unavailable on this platform")
	}
	_, err := e.Install(src, Options{})
	if err == nil || !strings.Contains(err.Error(), "non-regular") {
		t.Fatalf("special-file payload: err = %v, want non-regular refusal", err)
	}
}
