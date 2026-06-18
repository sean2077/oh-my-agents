package update

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeRelease serves a release with one platform binary + checksums.txt.
type fakeRelease struct {
	tag        string
	binary     []byte
	sumOf      []byte // bytes the checksum is computed over (≠ binary to force mismatch)
	noSums     bool
	badName    bool
	foreignURL bool
}

func serveRelease(t *testing.T, fr fakeRelease) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	assetName := fmt.Sprintf("oma_%s_linux_amd64", fr.tag)
	if fr.badName {
		assetName = "definitely-not-oma.tar.gz"
	}
	mux.HandleFunc("/repos/"+Repo+"/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		binURL := srv.URL + "/dl/" + assetName
		if fr.foreignURL {
			binURL = "https://evil.example.com/dl/" + assetName
		}
		assets := fmt.Sprintf(`{"name":%q,"browser_download_url":%q}`, assetName, binURL)
		if !fr.noSums {
			assets += fmt.Sprintf(`,{"name":"checksums.txt","browser_download_url":%q}`, srv.URL+"/dl/checksums.txt")
		}
		_, _ = fmt.Fprintf(w, `{"tag_name":%q,"assets":[%s]}`, fr.tag, assets)
	})
	mux.HandleFunc("/dl/"+assetName, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fr.binary)
	})
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		sum := sha256.Sum256(fr.sumOf)
		_, _ = fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), assetName)
	})
	return srv
}

// testUpdater anchors a fake installed binary in a temp dir.
func testUpdater(t *testing.T, srv *httptest.Server, newVersion string) (*Updater, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "oma")
	if err := os.WriteFile(bin, []byte("OLD BINARY"), 0o755); err != nil {
		t.Fatal(err)
	}
	u := &Updater{
		APIBase:    srv.URL,
		Client:     srv.Client(),
		BinaryPath: bin,
		Current:    "v0.0.1",
		OS:         "linux",
		Arch:       "amd64",
		Out:        io.Discard,
		SelfCheck:  func(string) (string, error) { return newVersion, nil },
	}
	return u, bin
}

func dirFingerprint(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		_, _ = fmt.Fprintf(h, "%s|%d\n", path, info.Size())
		if !info.IsDir() {
			raw, _ := os.ReadFile(path)
			h.Write(raw)
		}
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

func TestHappyPathUpdateAndOldBackup(t *testing.T) {
	newBin := []byte("NEW BINARY v9")
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: newBin, sumOf: newBin})
	u, bin := testUpdater(t, srv, "v0.0.9")

	rel, differs, err := u.Check()
	if err != nil || !differs || rel.TagName != "v0.0.9" {
		t.Fatalf("check: rel=%+v differs=%v err=%v", rel, differs, err)
	}
	if err := u.Apply(rel, false); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, _ := os.ReadFile(bin)
	if string(got) != "NEW BINARY v9" {
		t.Fatalf("binary content = %q", got)
	}
	old, _ := os.ReadFile(bin + ".old")
	if string(old) != "OLD BINARY" {
		t.Fatalf(".old backup = %q", old)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(bin)
		if err != nil || info.Mode().Perm()&0o111 == 0 {
			t.Fatal("replaced binary must be executable")
		}
	}
}

func TestChecksumMismatchRefusedBinaryUntouched(t *testing.T) {
	// §5 matrix: checksum mismatch → refuse, keep the current binary.
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: []byte("TAMPERED"), sumOf: []byte("EXPECTED")})
	u, bin := testUpdater(t, srv, "v0.0.9")
	before := dirFingerprint(t, filepath.Dir(bin))

	rel, _, err := u.Check()
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Apply(rel, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want checksum refusal", err)
	}
	if got := dirFingerprint(t, filepath.Dir(bin)); got != before {
		t.Fatal("binary directory changed after refused update (tmp residue?)")
	}
}

func TestMetadataUnavailableFailsClosed(t *testing.T) {
	// §5 matrix: metadata endpoint down → refuse.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	u, _ := testUpdater(t, srv, "")
	if _, _, err := u.Check(); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "metadata unavailable") {
		t.Fatalf("err = %v", err)
	}
}

func TestUnwritableTargetDegradesToInstructions(t *testing.T) {
	// §5 matrix: target not writable → manual instructions, no elevation.
	newBin := []byte("NEW")
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: newBin, sumOf: newBin})
	u, bin := testUpdater(t, srv, "v0.0.9")
	var out strings.Builder
	u.Out = &out
	if err := os.Chmod(filepath.Dir(bin), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(bin), 0o700) })

	rel, _, _ := u.Check()
	err := u.Apply(rel, false)
	if !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "not writable") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out.String(), "update manually") {
		t.Fatalf("manual instructions missing: %s", out.String())
	}
}

func TestInterruptedSwapRefusedOnlyWhenBinaryMissing(t *testing.T) {
	// §5 matrix: interrupted replace → .old is RECOVERY state only when
	// the binary itself is gone (the kill window between the two
	// renames); refuse with a restore hint and leave it intact
	// (review 064 blocker 2 narrowed the fatal case to exactly this).
	newBin := []byte("NEW")
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: newBin, sumOf: newBin})
	u, bin := testUpdater(t, srv, "v0.0.9")
	if err := os.WriteFile(bin+".old", []byte("interrupted recovery copy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(bin); err != nil { // the kill window: binary gone
		t.Fatal(err)
	}
	rel, _, _ := u.Check()
	if err := u.Apply(rel, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "interrupted swap") {
		t.Fatalf("err = %v", err)
	}
	old, _ := os.ReadFile(bin + ".old")
	if string(old) != "interrupted recovery copy" {
		t.Fatal("recovery sibling must be left intact")
	}
}

func TestTwoSuccessiveUpdatesRotateBackup(t *testing.T) {
	// review 064 blocker 2: a successful update must not block the next
	// one — .old from a previous success is rotated, not fatal.
	bin9 := []byte("NEW BINARY v9")
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: bin9, sumOf: bin9})
	u, bin := testUpdater(t, srv, "v0.0.9")
	rel, _, _ := u.Check()
	if err := u.Apply(rel, false); err != nil {
		t.Fatalf("first update: %v", err)
	}

	bin10 := []byte("NEW BINARY v10")
	srv2 := serveRelease(t, fakeRelease{tag: "v0.0.10", binary: bin10, sumOf: bin10})
	u.APIBase, u.Client = srv2.URL, srv2.Client()
	u.Current = "v0.0.9"
	u.SelfCheck = func(string) (string, error) { return "v0.0.10", nil }
	rel2, _, _ := u.Check()
	if err := u.Apply(rel2, false); err != nil {
		t.Fatalf("second update must rotate the backup, got: %v", err)
	}
	got, _ := os.ReadFile(bin)
	if string(got) != "NEW BINARY v10" {
		t.Fatalf("binary = %q", got)
	}
	old, _ := os.ReadFile(bin + ".old")
	if string(old) != "NEW BINARY v9" {
		t.Fatalf(".old after rotation = %q, want the v9 binary", old)
	}
}

func TestNoProbeFileEverCreated(t *testing.T) {
	// review 064 blocker 1: the writability check must not create
	// anything. Canary: a pre-existing file at the old probe path would
	// have made the O_EXCL probe misreport "unwritable"; the Access-based
	// check ignores it and never touches it.
	newBin := []byte("NEW")
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: newBin, sumOf: newBin})
	u, bin := testUpdater(t, srv, "v0.0.9")
	canary := filepath.Join(filepath.Dir(bin), ".oma-writable-probe")
	if err := os.WriteFile(canary, []byte("canary"), 0o400); err != nil {
		t.Fatal(err)
	}
	rel, _, _ := u.Check()
	var out strings.Builder
	u.Out = &out
	if err := u.Apply(rel, true); err != nil {
		t.Fatalf("dry-run with canary present: %v", err)
	}
	raw, err := os.ReadFile(canary)
	if err != nil || string(raw) != "canary" {
		t.Fatal("canary file was touched by the writability check")
	}
	if err := u.Apply(rel, false); err != nil {
		t.Fatalf("real apply with canary present: %v", err)
	}
}

func TestDryRunOnUnwritableDirValidatesWithZeroWrites(t *testing.T) {
	// Dry-run runs the SAME validation: an unwritable target is reported
	// (manual instructions + error) and the tree stays byte-identical.
	newBin := []byte("NEW")
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: newBin, sumOf: newBin})
	u, bin := testUpdater(t, srv, "v0.0.9")
	var out strings.Builder
	u.Out = &out
	if err := os.Chmod(filepath.Dir(bin), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(bin), 0o700) })
	before := dirFingerprint(t, filepath.Dir(bin))
	rel, _, _ := u.Check()
	if err := u.Apply(rel, true); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "not writable") {
		t.Fatalf("dry-run on unwritable dir: err = %v", err)
	}
	if got := dirFingerprint(t, filepath.Dir(bin)); got != before {
		t.Fatal("dry-run validation wrote to the directory")
	}
}

func TestSelfCheckFailureRollsBack(t *testing.T) {
	// §5 matrix: post-replace self-check fails → previous binary restored.
	newBin := []byte("BROKEN NEW")
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: newBin, sumOf: newBin})
	u, bin := testUpdater(t, srv, "v0.0.9")
	calls := 0
	u.SelfCheck = func(path string) (string, error) {
		calls++
		if calls == 1 {
			return "v0.0.9", nil // pre-flight on the tmp passes…
		}
		return "", errors.New("crash on startup") // …the installed one fails
	}
	rel, _, _ := u.Check()
	err := u.Apply(rel, false)
	if !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("err = %v", err)
	}
	got, _ := os.ReadFile(bin)
	if string(got) != "OLD BINARY" {
		t.Fatalf("binary after rollback = %q, want the previous one", got)
	}
}

func TestPreflightSelfCheckBlocksBadDownloadBeforeSwap(t *testing.T) {
	newBin := []byte("WRONG VERSION BINARY")
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: newBin, sumOf: newBin})
	u, bin := testUpdater(t, srv, "v0.0.8") // claims the wrong version
	rel, _, _ := u.Check()
	if err := u.Apply(rel, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "self-check") {
		t.Fatalf("err = %v", err)
	}
	got, _ := os.ReadFile(bin)
	if string(got) != "OLD BINARY" {
		t.Fatal("current binary must be untouched when pre-flight fails")
	}
	if _, err := os.Lstat(bin + ".old"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("no .old may exist when the swap never started")
	}
}

func TestCheckIsStrictlyReadOnly(t *testing.T) {
	// §5 matrix: --check zero-write.
	newBin := []byte("NEW")
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: newBin, sumOf: newBin})
	u, bin := testUpdater(t, srv, "v0.0.9")
	before := dirFingerprint(t, filepath.Dir(bin))
	if _, _, err := u.Check(); err != nil {
		t.Fatal(err)
	}
	if got := dirFingerprint(t, filepath.Dir(bin)); got != before {
		t.Fatal("Check wrote to the binary directory")
	}
}

func TestDryRunDisclosesPathsWritesNothing(t *testing.T) {
	newBin := []byte("NEW")
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: newBin, sumOf: newBin})
	u, bin := testUpdater(t, srv, "v0.0.9")
	var out strings.Builder
	u.Out = &out
	before := dirFingerprint(t, filepath.Dir(bin))
	rel, _, _ := u.Check()
	if err := u.Apply(rel, true); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"would download", "would backup", bin + ".old", bin + ".oma-update-tmp"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out.String())
		}
	}
	if got := dirFingerprint(t, filepath.Dir(bin)); got != before {
		t.Fatal("dry-run wrote to the binary directory")
	}
}

func TestNamingContractAndForeignURLRefused(t *testing.T) {
	// §5: unexpected asset names fail closed.
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: []byte("x"), sumOf: []byte("x"), badName: true})
	u, _ := testUpdater(t, srv, "v0.0.9")
	if _, _, err := u.Check(); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "naming contract") {
		t.Fatalf("bad name: err = %v", err)
	}
	// Foreign download URL fails closed before any network fetch.
	srv2 := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: []byte("x"), sumOf: []byte("x"), foreignURL: true})
	u2, _ := testUpdater(t, srv2, "v0.0.9")
	rel, _, err := u2.Check()
	if err != nil {
		t.Fatal(err)
	}
	if err := u2.Apply(rel, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "outside the pinned source") {
		t.Fatalf("foreign URL: err = %v", err)
	}
}

func TestUpToDateNoUpdate(t *testing.T) {
	newBin := []byte("SAME")
	srv := serveRelease(t, fakeRelease{tag: "v0.0.1", binary: newBin, sumOf: newBin})
	u, _ := testUpdater(t, srv, "v0.0.1")
	_, differs, err := u.Check()
	if err != nil || differs {
		t.Fatalf("differs=%v err=%v, want up to date", differs, err)
	}
}

func TestMissingChecksumsRefused(t *testing.T) {
	newBin := []byte("NEW")
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: newBin, sumOf: newBin, noSums: true})
	u, _ := testUpdater(t, srv, "v0.0.9")
	rel, _, _ := u.Check()
	if err := u.Apply(rel, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "checksums.txt") {
		t.Fatalf("err = %v", err)
	}
}
