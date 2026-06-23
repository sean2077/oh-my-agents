package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
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

func serveAssets(t *testing.T, ref string, tarball []byte, tamperSum bool) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256(tarball)
	hexsum := hex.EncodeToString(sum[:])
	if tamperSum {
		hexsum = strings.Repeat("0", 64)
	}
	bundle := "assets-" + ref + ".tar.gz"
	base := "/sean2077/oh-my-agents/releases/download/" + ref
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc(base+"/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		// A real release lists the platform binaries too; only the bundle matters here.
		_, _ = fmt.Fprintf(w, "%s  oma_%s_linux_amd64\n", strings.Repeat("a", 64), ref)
		_, _ = fmt.Fprintf(w, "%s  %s\n", hexsum, bundle)
	})
	mux.HandleFunc(base+"/"+bundle, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarball)
	})
	return srv
}

func testFetcher(srv *httptest.Server, ref string) *assetFetcher {
	return &assetFetcher{
		Repo:         "sean2077/oh-my-agents",
		Ref:          ref,
		DownloadBase: srv.URL,
		Client:       srv.Client(),
		Out:          io.Discard,
	}
}

func testExtractLimits(entryBytes, extractedBytes int64, entries int) assetFetchLimits {
	return assetFetchLimits{EntryBytes: entryBytes, ExtractedBytes: extractedBytes, Entries: entries}
}

func TestAssetFetchSuccess(t *testing.T) {
	ref := "v1.2.3"
	tarball := makeTarGz(t, map[string]string{
		"skills/deep-interview/manifest.json": `{"schema":"oma-manifest/1"}`,
		"skills/deep-interview/SKILL.md":      "body",
	})
	srv := serveAssets(t, ref, tarball, false)
	root, cleanup, err := testFetcher(srv, ref).Fetch(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	got, err := os.ReadFile(filepath.Join(root, "skills", "deep-interview", "manifest.json"))
	if err != nil || string(got) != `{"schema":"oma-manifest/1"}` {
		t.Fatalf("extracted manifest = %q err=%v", got, err)
	}
}

func TestAssetFetchChecksumMismatchFailsClosed(t *testing.T) {
	ref := "v1.2.3"
	tarball := makeTarGz(t, map[string]string{"skills/x/manifest.json": "{}"})
	srv := serveAssets(t, ref, tarball, true)
	dest := t.TempDir()
	_, _, err := testFetcher(srv, ref).Fetch(dest)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want checksum mismatch", err)
	}
	// No residue left behind on a refused fetch.
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Fatalf("temp residue after refused fetch: %v", entries)
	}
}

func TestAssetFetchDuplicateChecksumNameFailsClosed(t *testing.T) {
	ref := "v1.2.3"
	tarball := makeTarGz(t, map[string]string{"skills/x/manifest.json": "{}"})
	sum := sha256.Sum256(tarball)
	hexsum := hex.EncodeToString(sum[:])
	bundle := "assets-" + ref + ".tar.gz"
	base := "/sean2077/oh-my-agents/releases/download/" + ref
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc(base+"/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "%s  %s\n", hexsum, bundle)
		_, _ = fmt.Fprintf(w, "%s  %s\n", strings.Repeat("f", 64), bundle)
	})
	mux.HandleFunc(base+"/"+bundle, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(tarball)
	})
	_, _, err := testFetcher(srv, ref).Fetch(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "duplicate entries") {
		t.Fatalf("err = %v, want duplicate checksum-name rejection", err)
	}
}

func TestAssetFetchCompressedLimitFailsClosedAndCleansTemp(t *testing.T) {
	ref := "v1.2.3"
	tarball := makeTarGz(t, map[string]string{"skills/x/manifest.json": strings.Repeat("x", 128)})
	srv := serveAssets(t, ref, tarball, false)
	dest := t.TempDir()
	fetcher := testFetcher(srv, ref)
	fetcher.Limits.CompressedBytes = int64(len(tarball) - 1)
	_, _, err := fetcher.Fetch(dest)
	if err == nil || !strings.Contains(err.Error(), "compressed limit") {
		t.Fatalf("err = %v, want compressed limit refusal", err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Fatalf("temp residue after refused compressed fetch: %v", entries)
	}
}

func TestAssetFetchMissingBundleFailsClosed(t *testing.T) {
	ref := "v9.9.9"
	base := "/sean2077/oh-my-agents/releases/download/" + ref
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc(base+"/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "%s  oma_%s_linux_amd64\n", strings.Repeat("a", 64), ref)
	})
	_, _, err := testFetcher(srv, ref).Fetch(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no asset bundle") {
		t.Fatalf("err = %v, want no asset bundle", err)
	}
}

func TestExtractTarGzRejectsTraversal(t *testing.T) {
	tarball := makeTarGz(t, map[string]string{"../escape.txt": "x"})
	tarPath := filepath.Join(t.TempDir(), "b.tar.gz")
	if err := os.WriteFile(tarPath, tarball, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(tarPath, filepath.Join(t.TempDir(), "root"), testExtractLimits(1<<20, 1<<20, 1000)); err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("err = %v, want unsafe-path rejection", err)
	}
}

func TestExtractTarGzRejectsOversizedEntry(t *testing.T) {
	// An entry larger than the cap must fail closed, not be silently truncated.
	tarball := makeTarGz(t, map[string]string{"skills/x/manifest.json": "0123456789"}) // 10 bytes
	tarPath := filepath.Join(t.TempDir(), "b.tar.gz")
	if err := os.WriteFile(tarPath, tarball, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(tarPath, filepath.Join(t.TempDir(), "root"), testExtractLimits(4, 1<<20, 1000)); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v, want oversize rejection", err)
	}
}

func TestExtractTarGzRejectsTotalExtractedSize(t *testing.T) {
	tarball := makeTarGz(t, map[string]string{
		"skills/x/a.txt": "1234",
		"skills/x/b.txt": "5678",
	})
	tarPath := filepath.Join(t.TempDir(), "b.tar.gz")
	if err := os.WriteFile(tarPath, tarball, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(tarPath, filepath.Join(t.TempDir(), "root"), testExtractLimits(10, 6, 1000)); err == nil || !strings.Contains(err.Error(), "extracted size") {
		t.Fatalf("err = %v, want total-size rejection", err)
	}
}

func TestExtractTarGzRejectsEntryCount(t *testing.T) {
	tarball := makeTarGz(t, map[string]string{
		"skills/x/a.txt": "",
		"skills/x/b.txt": "",
		"skills/x/c.txt": "",
	})
	tarPath := filepath.Join(t.TempDir(), "b.tar.gz")
	if err := os.WriteFile(tarPath, tarball, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(tarPath, filepath.Join(t.TempDir(), "root"), testExtractLimits(10, 10, 2)); err == nil || !strings.Contains(err.Error(), "more than 2 entries") {
		t.Fatalf("err = %v, want entry-count rejection", err)
	}
}

func TestAssetFetchExtractLimitFailsClosedAndCleansTemp(t *testing.T) {
	ref := "v1.2.3"
	tarball := makeTarGz(t, map[string]string{
		"skills/x/a.txt": "1234",
		"skills/x/b.txt": "5678",
	})
	srv := serveAssets(t, ref, tarball, false)
	dest := t.TempDir()
	fetcher := testFetcher(srv, ref)
	fetcher.Limits.ExtractedBytes = 6
	_, _, err := fetcher.Fetch(dest)
	if err == nil || !strings.Contains(err.Error(), "extracted size") {
		t.Fatalf("err = %v, want extracted-size refusal", err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Fatalf("temp residue after refused extract: %v", entries)
	}
}

func TestAssetFetcherValidate(t *testing.T) {
	bad := []struct{ repo, ref string }{
		{"sean2077/oh-my-agents", "../etc"},
		{"sean2077/oh-my-agents", "v1/2"},
		{"sean2077/oh-my-agents", ""},
		{"not-a-repo", "v1.0.0"},
	}
	for _, c := range bad {
		if err := (&assetFetcher{Repo: c.repo, Ref: c.ref}).validate(); err == nil {
			t.Errorf("validate(repo=%q ref=%q) = nil, want error", c.repo, c.ref)
		}
	}
	if err := (&assetFetcher{Repo: "sean2077/oh-my-agents", Ref: "v1.0.0"}).validate(); err != nil {
		t.Errorf("validate(valid) = %v", err)
	}
}

func TestAssetRestrictedClientRejectsNonHTTPSRedirects(t *testing.T) {
	client := assetRestrictedClient()
	via, _ := http.NewRequest(http.MethodGet, "https://github.com/sean2077/oh-my-agents/releases/download/v1.0.0/checksums.txt", nil)
	cases := []struct {
		name    string
		target  string
		wantErr string
	}{
		{name: "github https", target: "https://github.com/sean2077/oh-my-agents/releases/download/v1.0.0/assets-v1.0.0.tar.gz"},
		{name: "githubusercontent https", target: "https://objects.githubusercontent.com/github-production-release-asset-2e65be/asset"},
		{name: "github http", target: "http://github.com/sean2077/oh-my-agents/releases/download/v1.0.0/assets-v1.0.0.tar.gz", wantErr: "non-HTTPS"},
		{name: "foreign https", target: "https://evil.example.com/assets-v1.0.0.tar.gz", wantErr: "foreign host"},
	}
	for _, tc := range cases {
		req, _ := http.NewRequest(http.MethodGet, tc.target, nil)
		err := client.CheckRedirect(req, []*http.Request{via})
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("%s: err = %v, want nil", tc.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.wantErr)
		}
	}
}
