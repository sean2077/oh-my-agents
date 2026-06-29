package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// S1 / SEC-2: negative-test coverage for the archive-extraction defenses that
// `oma asset install --ref` relies on. makeTarGz only emits regular files, so
// these tests build the malformed/hostile archives those guards exist for.
//
// Two guards in extractTarGz are deliberately NOT covered behaviorally because
// they are unreachable through archive/tar's Reader (the only reader used):
//   - negative size (assetfetch.go:298-300): the Reader's handleRegularFile
//     rejects a negative-size regular entry with ErrHeader before tr.Next()
//     ever returns it, so extractTarGz never sees hdr.Size < 0.
//   - under-reported truncation (assetfetch.go:315-327): the Reader bounds each
//     entry read at hdr.Size, so it never hands back more bytes than declared.
// Both are defense-in-depth against a non-stdlib reader; exercising them would
// require replacing the reader (a behavior change out of this test-only slice).
// The reachable per-entry cap is covered by TestExtractTarGzRejectsOversizedEntry.

type tarEntry struct {
	hdr  tar.Header
	body string
}

// tarGzHeaders builds a gzip'd tar from exactly the given headers, in order, so
// a test can place non-regular entry types and raw names that makeTarGz's
// regular-file-only path cannot express.
func tarGzHeaders(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := e.hdr
		h.Size = int64(len(e.body))
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatalf("WriteHeader(%q type %d): %v", h.Name, h.Typeflag, err)
		}
		if len(e.body) > 0 {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeTarGz(t *testing.T, tarball []byte) string {
	t.Helper()
	tarPath := filepath.Join(t.TempDir(), "b.tar.gz")
	if err := os.WriteFile(tarPath, tarball, 0o600); err != nil {
		t.Fatal(err)
	}
	return tarPath
}

func TestExtractTarGzRejectsUnsupportedEntryTypes(t *testing.T) {
	// Only regular files and directories may be unpacked. symlink/hardlink are
	// the classic archive-escape vectors; char-device/fifo are special-file
	// vectors. All must hit the default reject branch (assetfetch.go:328-330).
	cases := []struct {
		name string
		hdr  tar.Header
	}{
		{"symlink", tar.Header{Name: "evil", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}},
		{"hardlink", tar.Header{Name: "hard", Typeflag: tar.TypeLink, Linkname: "skills/x"}},
		{"char-device", tar.Header{Name: "dev", Typeflag: tar.TypeChar, Devmajor: 1, Devminor: 3}},
		{"fifo", tar.Header{Name: "pipe", Typeflag: tar.TypeFifo}},
	}
	for _, c := range cases {
		tarPath := writeTarGz(t, tarGzHeaders(t, tarEntry{hdr: c.hdr}))
		err := extractTarGz(tarPath, filepath.Join(t.TempDir(), "root"), testExtractLimits(1<<20, 1<<20, 1000))
		if err == nil || !strings.Contains(err.Error(), "unsupported archive entry") {
			t.Errorf("%s: err = %v, want unsupported-entry rejection", c.name, err)
		}
	}
}

func TestExtractTarGzRejectsEscapeVariants(t *testing.T) {
	// Beyond the single "../" prefix already covered: an absolute path, a bare
	// "..", and a deeper "../../" must all be refused before any file is made
	// (assetfetch.go:285,289).
	cases := []struct {
		name, entryName string
	}{
		{"absolute", "/etc/cron.d/x"},
		{"bare-dotdot", ".."},
		{"nested-dotdot", "skills/../../etc/x"},
	}
	for _, c := range cases {
		tarPath := writeTarGz(t, tarGzHeaders(t, tarEntry{
			hdr:  tar.Header{Name: c.entryName, Typeflag: tar.TypeReg, Mode: 0o644},
			body: "x",
		}))
		root := filepath.Join(t.TempDir(), "root")
		err := extractTarGz(tarPath, root, testExtractLimits(1<<20, 1<<20, 1000))
		if err == nil || (!strings.Contains(err.Error(), "unsafe path") && !strings.Contains(err.Error(), "escapes")) {
			t.Errorf("%s (%q): err = %v, want escape rejection", c.name, c.entryName, err)
		}
	}
}

func TestExtractTarGzRejectsCorruptGzip(t *testing.T) {
	// A non-gzip payload must fail closed at gzip.NewReader, not panic
	// (assetfetch.go:257-260).
	tarPath := writeTarGz(t, []byte("this is plainly not a gzip stream"))
	err := extractTarGz(tarPath, filepath.Join(t.TempDir(), "root"), testExtractLimits(1<<20, 1<<20, 1000))
	if err == nil {
		t.Fatal("corrupt gzip must fail, not pass")
	}
	if !strings.Contains(err.Error(), "invalid header") && !strings.Contains(err.Error(), "gzip") {
		t.Fatalf("err = %v, want a gzip header error", err)
	}
}
