package asset

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// DigestTree computes a deterministic, unambiguous content digest of a file
// or directory tree. Managed-ownership checks compare it against the
// registry digest to detect drift (docs/security-contract.md §2).
//
// Framing (B3 recheck blocker 2): each record is
//
//	type byte ('F' file | 'D' dir) + uvarint(len(rel)) + rel + sha256(content) [files only]
//
// Length-prefixed paths and fixed-size per-file hashes make record
// boundaries unambiguous for arbitrary file bytes; empty directories are
// recorded so their appearance/removal changes the digest.
func DigestTree(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if !info.IsDir() {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: refusing to digest symlink %s", ErrInvalid, path)
		}
		sum, err := fileSHA256(path)
		if err != nil {
			return "", err
		}
		writeRecord(h, 'F', "", sum)
		return hex.EncodeToString(h.Sum(nil)), nil
	}

	type record struct {
		rel string
		typ byte
		sum []byte
	}
	var records []record
	err = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: refusing to digest symlink %s", ErrInvalid, p)
		}
		rel, err := filepath.Rel(path, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case fi.IsDir():
			if rel != "." {
				records = append(records, record{rel: rel, typ: 'D'})
			}
		case fi.Mode().IsRegular():
			sum, err := fileSHA256(p)
			if err != nil {
				return err
			}
			records = append(records, record{rel: rel, typ: 'F', sum: sum})
		default:
			return fmt.Errorf("%w: refusing special file %s", ErrInvalid, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].rel < records[j].rel })
	for _, r := range records {
		writeRecord(h, r.typ, r.rel, r.sum)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeRecord(h io.Writer, typ byte, rel string, sum []byte) {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], uint64(len(rel)))
	_, _ = h.Write([]byte{typ})
	_, _ = h.Write(buf[:n])
	_, _ = h.Write([]byte(rel))
	if sum != nil {
		_, _ = h.Write(sum)
	}
}

func fileSHA256(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
