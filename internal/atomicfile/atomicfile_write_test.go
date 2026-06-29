package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hasTempLeftover reports whether dir contains any writeTemp-style temp file
// (".<base sans dot>-*.tmp"). It mirrors the naming in writeTemp so a leaked
// temp from a failed Write is detected.
func hasTempLeftover(t *testing.T, dir, base string) bool {
	t.Helper()
	pattern := "." + strings.TrimPrefix(base, ".") + "-*.tmp"
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		t.Fatalf("glob %q: %v", pattern, err)
	}
	return len(matches) > 0
}

func TestWriteCleansTempOnRenameFailure(t *testing.T) {
	// Write does writeTemp -> os.Rename(tmp, path). os.Rename onto an existing
	// NON-EMPTY directory fails, exercising the deferred temp-file cleanup
	// (atomicfile.go:22-30): keep stays false, so the temp file is removed.
	parent := t.TempDir()
	base := "target"
	path := filepath.Join(parent, base)

	// Make path an existing non-empty directory so os.Rename(tmp, path) fails.
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "child"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Write(path, []byte("payload"), 0o600); err == nil {
		t.Fatal("Write onto a non-empty directory must fail")
	}

	if hasTempLeftover(t, parent, base) {
		t.Fatal("Write must remove its temp file after a rename failure")
	}
	// The pre-existing directory must be untouched.
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("path should remain the original directory, stat=%v err=%v", info, err)
	}
}

func TestWriteWithBackupFailsClosedOnNonENOENTRead(t *testing.T) {
	// WriteWithBackup reads path; a NON-ENOENT read error must propagate
	// (fail-closed) WITHOUT writing path or path.bak (atomicfile.go:39-46).
	//
	// We make path a SYMLINK to a directory: os.ReadFile follows it and fails
	// with a non-ENOENT error (EISDIR), yet os.Rename would happily REPLACE the
	// symlink itself — so ONLY the line 44-46 guard prevents the write. (A plain
	// directory at path would also make the later os.Rename fail, masking the
	// guard; the symlink isolates the guard so this is a true fail-before pin:
	// drop the guard and WriteWithBackup returns nil and clobbers the symlink.)
	parent := t.TempDir()
	realDir := filepath.Join(parent, "realdir")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "config")
	if err := os.Symlink(realDir, path); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}

	err := WriteWithBackup(path, []byte("payload"), 0o600)
	if err == nil {
		t.Fatal("WriteWithBackup must fail closed when the existing path cannot be read")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a non-ENOENT read error must not be reported as ErrNotExist: %v", err)
	}

	// No backup may be created on the fail-closed path.
	if _, statErr := os.Lstat(path + ".bak"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("no .bak must be written on a fail-closed read, stat err = %v", statErr)
	}
	// path must be untouched: still the original symlink, not overwritten by a
	// regular file — which is exactly what dropping the guard would do.
	info, statErr := os.Lstat(path)
	if statErr != nil {
		t.Fatalf("lstat path: %v", statErr)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("path must remain the original symlink (the guard must prevent the write); mode = %v", info.Mode())
	}
}

func TestWriteWithBackupPreservesBakOnWriteFailure(t *testing.T) {
	// WriteWithBackup refreshes path.bak with the PREVIOUS generation BEFORE it
	// rewrites path (atomicfile.go:39-47). If the main write then fails, that
	// freshly-written .bak must survive so the prior generation stays
	// recoverable. Triggering this needs a seam: ONLY the main write's rename
	// may fail while the .bak rename succeeds, which no real filesystem
	// condition can express selectively — hence the renameFn injection.
	parent := t.TempDir()
	path := filepath.Join(parent, "config")

	// Previous generation: path = "v1". path is new, so no .bak is written yet.
	if err := WriteWithBackup(path, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Fail ONLY the main write's rename (dest == path); the .bak write
	// (dest == path+".bak") keeps using the real os.Rename.
	boom := errors.New("simulated main-write failure")
	orig := renameFn
	renameFn = func(oldp, newp string) error {
		if newp == path {
			return boom
		}
		return orig(oldp, newp)
	}
	defer func() { renameFn = orig }()

	if err := WriteWithBackup(path, []byte("v2"), 0o600); !errors.Is(err, boom) {
		t.Fatalf("WriteWithBackup err = %v, want the injected main-write failure", err)
	}

	// The .bak must hold the previous generation, refreshed before the failed
	// main write (the fail-before behavior: drop atomicfile.go:40-43 and no .bak
	// exists here).
	if bak, rerr := os.ReadFile(path + ".bak"); rerr != nil {
		t.Fatalf("a failed main write must leave the refreshed .bak in place: %v", rerr)
	} else if string(bak) != "v1" {
		t.Fatalf(".bak = %q, want the previous generation %q", bak, "v1")
	}
	// path itself is unchanged: the main write failed before replacing it.
	if cur, rerr := os.ReadFile(path); rerr != nil {
		t.Fatal(rerr)
	} else if string(cur) != "v1" {
		t.Fatalf("path = %q, want unchanged %q after a failed main write", cur, "v1")
	}
}
