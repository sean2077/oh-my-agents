// Package atomicfile centralizes durable same-directory file replacement.
package atomicfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Write replaces path with data using a unique temp file in the same
// directory, then fsyncs the written file and best-effort syncs the parent.
func Write(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := writeTemp(path, data, mode)
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tmp)
		}
	}()
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	keep = true
	syncDir(filepath.Dir(path))
	return nil
}

// WriteWithBackup writes path and first refreshes path+".bak" with the
// previous generation when one exists.
func WriteWithBackup(path string, data []byte, mode os.FileMode) error {
	prev, err := os.ReadFile(path)
	if err == nil {
		if err := Write(path+".bak", prev, mode); err != nil {
			return fmt.Errorf("write %s.bak: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return Write(path, data, mode)
}

func writeTemp(path string, data []byte, mode os.FileMode) (string, error) {
	base := "." + strings.TrimPrefix(filepath.Base(path), ".") + "-*.tmp"
	f, err := os.CreateTemp(filepath.Dir(path), base)
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return tmp, nil
}

func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
