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
	aux        []string // extra release assets the updater must ignore (SBOM, sigs, attestations)
	dupBinary  bool     // serve the platform binary asset twice (ambiguous release)
	dupSums    bool     // serve checksums.txt twice (ambiguous release)
	dupSumName bool     // serve duplicate checksum entries for the same consumed file
	requestLog *[]string
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
	record := func(r *http.Request) {
		if fr.requestLog != nil {
			*fr.requestLog = append(*fr.requestLog, r.URL.Path)
		}
	}
	mux.HandleFunc("/repos/"+Repo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		binURL := srv.URL + "/dl/" + assetName
		if fr.foreignURL {
			binURL = "https://evil.example.com/dl/" + assetName
		}
		var assets []string
		add := func(name, url string) {
			assets = append(assets, fmt.Sprintf(`{"name":%q,"browser_download_url":%q}`, name, url))
		}
		add(assetName, binURL)
		if fr.dupBinary {
			add(assetName, binURL)
		}
		if !fr.noSums {
			add("checksums.txt", srv.URL+"/dl/checksums.txt")
		}
		if fr.dupSums {
			add("checksums.txt", srv.URL+"/dl/checksums.txt")
		}
		for _, name := range fr.aux {
			add(name, srv.URL+"/dl/"+name)
		}
		_, _ = fmt.Fprintf(w, `{"tag_name":%q,"assets":[%s]}`, fr.tag, strings.Join(assets, ","))
	})
	mux.HandleFunc("/dl/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		record(r)
		_, _ = w.Write(fr.binary)
	})
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		sum := sha256.Sum256(fr.sumOf)
		_, _ = fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), assetName)
		if fr.dupSumName {
			_, _ = fmt.Fprintf(w, "%s  %s\n", strings.Repeat("f", 64), assetName)
		}
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
		TempDir:    t.TempDir(),
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
	if err := u.Apply(rel, false, false); err != nil {
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
	if err := u.Apply(rel, false, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "checksum mismatch") {
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
	err := u.Apply(rel, false, false)
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
	if err := u.Apply(rel, false, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "interrupted swap") {
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
	if err := u.Apply(rel, false, false); err != nil {
		t.Fatalf("first update: %v", err)
	}

	bin10 := []byte("NEW BINARY v10")
	srv2 := serveRelease(t, fakeRelease{tag: "v0.0.10", binary: bin10, sumOf: bin10})
	u.APIBase, u.Client = srv2.URL, srv2.Client()
	u.Current = "v0.0.9"
	u.SelfCheck = func(string) (string, error) { return "v0.0.10", nil }
	rel2, _, _ := u.Check()
	if err := u.Apply(rel2, false, false); err != nil {
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
	if err := u.Apply(rel, true, false); err != nil {
		t.Fatalf("dry-run with canary present: %v", err)
	}
	raw, err := os.ReadFile(canary)
	if err != nil || string(raw) != "canary" {
		t.Fatal("canary file was touched by the writability check")
	}
	if err := u.Apply(rel, false, false); err != nil {
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
	if err := u.Apply(rel, true, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "not writable") {
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
	err := u.Apply(rel, false, false)
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
	if err := u.Apply(rel, false, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "self-check") {
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
	var requests []string
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: newBin, sumOf: newBin, requestLog: &requests})
	u, bin := testUpdater(t, srv, "v0.0.9")
	var out strings.Builder
	u.Out = &out
	before := dirFingerprint(t, filepath.Dir(bin))
	rel, _, _ := u.Check()
	if err := u.Apply(rel, true, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"would download", "would backup", bin + ".old", "would write a private temp dir"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out.String())
		}
	}
	if got := dirFingerprint(t, filepath.Dir(bin)); got != before {
		t.Fatal("dry-run wrote to the binary directory")
	}
	for _, want := range []string{"/dl/checksums.txt", "/dl/oma_v0.0.9_linux_amd64"} {
		if !sawRequest(requests, want) {
			t.Fatalf("dry-run did not request %s; requests=%v", want, requests)
		}
	}
	if entries, err := os.ReadDir(u.TempDir); err != nil || len(entries) != 0 {
		t.Fatalf("dry-run validation temp dir not cleaned: entries=%v err=%v", entries, err)
	}
}

func TestDryRunChecksumMismatchFailsClosedAndCleansTemp(t *testing.T) {
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: []byte("TAMPERED"), sumOf: []byte("EXPECTED")})
	u, bin := testUpdater(t, srv, "v0.0.9")
	before := dirFingerprint(t, filepath.Dir(bin))
	rel, _, err := u.Check()
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Apply(rel, true, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("dry-run checksum mismatch: err = %v", err)
	}
	if got := dirFingerprint(t, filepath.Dir(bin)); got != before {
		t.Fatal("dry-run checksum mismatch changed the binary directory")
	}
	if entries, err := os.ReadDir(u.TempDir); err != nil || len(entries) != 0 {
		t.Fatalf("dry-run checksum mismatch left temp residue: entries=%v err=%v", entries, err)
	}
}

func TestDryRunWrongVersionFailsClosedAndCleansTemp(t *testing.T) {
	newBin := []byte("WRONG VERSION BINARY")
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: newBin, sumOf: newBin})
	u, bin := testUpdater(t, srv, "v0.0.8")
	before := dirFingerprint(t, filepath.Dir(bin))
	rel, _, err := u.Check()
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Apply(rel, true, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "self-check") {
		t.Fatalf("dry-run wrong version: err = %v", err)
	}
	if got := dirFingerprint(t, filepath.Dir(bin)); got != before {
		t.Fatal("dry-run wrong-version validation changed the binary directory")
	}
	if _, err := os.Lstat(bin + ".old"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dry-run wrong-version validation must not create .old")
	}
	if _, err := os.Lstat(bin + ".oma-update-tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dry-run wrong-version validation must not create the target temp file")
	}
	if entries, err := os.ReadDir(u.TempDir); err != nil || len(entries) != 0 {
		t.Fatalf("dry-run wrong-version validation left temp residue: entries=%v err=%v", entries, err)
	}
}

func TestMissingTargetBinaryAndForeignURLRefused(t *testing.T) {
	// An off-contract asset is present but the expected platform binary is not:
	// Check ignores the stray asset (§5: validate only what we consume), and
	// Apply fails closed for the missing target.
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: []byte("x"), sumOf: []byte("x"), badName: true})
	u, _ := testUpdater(t, srv, "v0.0.9")
	rel, _, err := u.Check()
	if err != nil {
		t.Fatalf("check must ignore the off-contract asset, got: %v", err)
	}
	if err := u.Apply(rel, false, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "no asset") {
		t.Fatalf("missing target binary: err = %v", err)
	}
	// Foreign download URL fails closed before any network fetch.
	srv2 := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: []byte("x"), sumOf: []byte("x"), foreignURL: true})
	u2, _ := testUpdater(t, srv2, "v0.0.9")
	rel2, _, err := u2.Check()
	if err != nil {
		t.Fatal(err)
	}
	if err := u2.Apply(rel2, false, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "outside the pinned source") {
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
	if err := u.Apply(rel, false, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "checksums.txt") {
		t.Fatalf("err = %v", err)
	}
}

func TestAuxiliaryAssetsIgnored(t *testing.T) {
	// Regression: a release carrying SBOM / signature / attestation assets the
	// updater does not consume must still check and apply cleanly. An SBOM
	// asset (sbom.spdx.json) used to trip a naming-contract rejection in Check,
	// which silently broke self-update for every release that shipped one.
	newBin := []byte("NEW BINARY v9")
	srv := serveRelease(t, fakeRelease{
		tag:    "v0.0.9",
		binary: newBin,
		sumOf:  newBin,
		aux:    []string{"sbom.spdx.json", "oma_v0.0.9_linux_amd64.sig", "provenance.intoto.jsonl"},
	})
	u, bin := testUpdater(t, srv, "v0.0.9")
	rel, available, err := u.Check()
	if err != nil || !available {
		t.Fatalf("check with aux assets: available=%v err=%v", available, err)
	}
	if err := u.Apply(rel, false, false); err != nil {
		t.Fatalf("apply with aux assets: %v", err)
	}
	if got, _ := os.ReadFile(bin); string(got) != "NEW BINARY v9" {
		t.Fatalf("binary = %q", got)
	}
}

func TestDuplicateConsumedAssetsRefused(t *testing.T) {
	// An ambiguous release (the platform binary or checksums.txt served twice)
	// must never be silently resolved — fail closed.
	newBin := []byte("NEW")
	srvBin := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: newBin, sumOf: newBin, dupBinary: true})
	u, _ := testUpdater(t, srvBin, "v0.0.1")
	rel, _, err := u.Check()
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Apply(rel, false, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "more than one") {
		t.Fatalf("duplicate binary: err = %v", err)
	}

	srvSums := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: newBin, sumOf: newBin, dupSums: true})
	u2, _ := testUpdater(t, srvSums, "v0.0.1")
	rel2, _, err := u2.Check()
	if err != nil {
		t.Fatal(err)
	}
	if err := u2.Apply(rel2, false, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "more than one") {
		t.Fatalf("duplicate checksums: err = %v", err)
	}
}

func TestDuplicateChecksumEntryRefused(t *testing.T) {
	newBin := []byte("NEW")
	srv := serveRelease(t, fakeRelease{tag: "v0.0.9", binary: newBin, sumOf: newBin, dupSumName: true})
	u, _ := testUpdater(t, srv, "v0.0.9")
	rel, _, err := u.Check()
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Apply(rel, false, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "duplicate entries") {
		t.Fatalf("duplicate checksum entry: err = %v", err)
	}
}

func TestDowngradeRefusedByDefaultAndAllowedWithFlag(t *testing.T) {
	// The latest stable release is OLDER than the running binary (e.g. a
	// v1.0.0-rc.1 build vs latest stable v0.9.0). Default: not an update, and
	// Apply refuses; --allow-downgrade performs it.
	older := []byte("OLDER RELEASE")
	srv := serveRelease(t, fakeRelease{tag: "v0.9.0", binary: older, sumOf: older})
	u, bin := testUpdater(t, srv, "v0.9.0")
	u.Current = "v1.0.0-rc.1" // running ahead of the latest stable

	rel, available, err := u.Check()
	if err != nil {
		t.Fatal(err)
	}
	if available {
		t.Fatal("a release older than the running version must not report as an update")
	}
	if err := u.Apply(rel, false, false); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "refusing downgrade") {
		t.Fatalf("default apply err = %v, want downgrade refusal", err)
	}
	if got, _ := os.ReadFile(bin); string(got) != "OLD BINARY" {
		t.Fatalf("binary changed on a refused downgrade = %q", got)
	}
	if err := u.Apply(rel, false, true); err != nil {
		t.Fatalf("allow-downgrade apply: %v", err)
	}
	if got, _ := os.ReadFile(bin); string(got) != "OLDER RELEASE" {
		t.Fatalf("binary after allowed downgrade = %q", got)
	}
}

func TestDevBuildSeesUpdate(t *testing.T) {
	// An unversioned dev/source build is treated as older than any release.
	newBin := []byte("REL")
	srv := serveRelease(t, fakeRelease{tag: "v0.9.1", binary: newBin, sumOf: newBin})
	u, _ := testUpdater(t, srv, "dev")
	_, available, err := u.Check()
	if err != nil || !available {
		t.Fatalf("dev build: available=%v err=%v, want update available", available, err)
	}
}

func TestMalformedRemoteTagFailsClosed(t *testing.T) {
	newBin := []byte("NEW")
	// Non-semver and leading-zero tags both fail closed at the parse gate.
	for _, tag := range []string{"not-a-version", "v01.2.3", "v1.0.0-rc.01"} {
		srv := serveRelease(t, fakeRelease{tag: tag, binary: newBin, sumOf: newBin})
		u, _ := testUpdater(t, srv, "v0.0.1")
		if _, _, err := u.Check(); !errors.Is(err, ErrUpdate) || !strings.Contains(err.Error(), "not a valid semantic version") {
			t.Fatalf("malformed remote tag %q: err = %v", tag, err)
		}
	}
}

func TestParseSemverRejectsLeadingZero(t *testing.T) {
	bad := []string{"v01.2.3", "v1.02.3", "v1.2.03", "v1.2.3-rc.01", "v1.2.3-00", "1.2.3-01.2", "v1.2.3-"}
	for _, s := range bad {
		if IsSemver(s) {
			t.Errorf("IsSemver(%q) = true, want false (invalid per SemVer)", s)
		}
	}
	good := []string{"v1.2.3", "v0.9.1", "v1.0.0-rc.1", "v1.0.0-rc.10", "v1.0.0-0", "v1.0.0-alpha01", "v1.0.0-0a", "1.2.3"}
	for _, s := range good {
		if !IsSemver(s) {
			t.Errorf("IsSemver(%q) = false, want true", s)
		}
	}
}

func TestParseSemverRejectsMalformedBuildMetadata(t *testing.T) {
	bad := []string{"v1.2.3+.", "v1.2.3+build.", "v1.2.3+build..meta", "v1.2.3+", "v1.2.3-rc.1+"}
	for _, s := range bad {
		if IsSemver(s) {
			t.Errorf("IsSemver(%q) = true, want false (malformed build metadata)", s)
		}
	}
	good := []string{"v1.2.3+001", "v1.2.3+build.1", "v1.2.3+exp.sha", "v1.2.3-rc.1+build.2"}
	for _, s := range good {
		if !IsSemver(s) {
			t.Errorf("IsSemver(%q) = false, want true (valid build metadata)", s)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
		ok   bool
	}{
		{"v0.9.1", "v0.9.2", -1, true},
		{"v0.9.2", "v0.9.1", 1, true},
		{"v1.0.0", "v1.0.0", 0, true},
		{"v2.0.0", "v1.9.9", 1, true},
		{"v1.2.0", "v1.1.9", 1, true},
		{"1.2.3", "v1.2.3", 0, true},        // optional leading v on either side
		{"v1.0.0-rc.1", "v1.0.0", -1, true}, // prerelease < release
		{"v1.0.0", "v1.0.0-rc.1", 1, true},
		{"v1.0.0-rc.1", "v1.0.0-rc.2", -1, true},
		{"v1.0.0-rc.2", "v1.0.0-rc.10", -1, true}, // numeric identifiers compare numerically, not lexically
		{"v1.0.0-rc.10", "v1.0.0-rc.2", 1, true},
		{"v1.0.0-alpha", "v1.0.0-beta", -1, true},   // alphanumeric lexical
		{"v1.0.0-rc.1", "v1.0.0-rc.1.1", -1, true},  // more fields wins when all prior tie
		{"v1.0.0-1", "v1.0.0-alpha", -1, true},      // numeric below alphanumeric
		{"v1.0.0+build1", "v1.0.0+build2", 0, true}, // build metadata ignored
		{"dev", "v0.9.1", 0, false},                 // unparseable → ok=false
		{"v0.9.1", "main", 0, false},
		{"", "v0.9.1", 0, false},
	}
	for _, c := range cases {
		got, ok := CompareVersions(c.a, c.b)
		if got != c.want || ok != c.ok {
			t.Errorf("CompareVersions(%q, %q) = (%d, %v), want (%d, %v)", c.a, c.b, got, ok, c.want, c.ok)
		}
	}
}

// serveReleaseList serves the /releases list and a /releases/tags/<tag> doc per
// tag (the prerelease channel and --version pin sources).
func serveReleaseList(t *testing.T, tags []string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/repos/"+Repo+"/releases", func(w http.ResponseWriter, _ *http.Request) {
		var items []string
		for _, tg := range tags {
			items = append(items, fmt.Sprintf(`{"tag_name":%q,"draft":false,"prerelease":%v,"assets":[]}`, tg, strings.Contains(tg, "-")))
		}
		_, _ = fmt.Fprintf(w, "[%s]", strings.Join(items, ","))
	})
	for _, tg := range tags {
		tag := tg
		mux.HandleFunc("/repos/"+Repo+"/releases/tags/"+tag, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprintf(w, `{"tag_name":%q,"assets":[]}`, tag)
		})
	}
	return srv
}

func TestResolvePrereleaseChannelPicksHighest(t *testing.T) {
	srv := serveReleaseList(t, []string{"v0.9.1", "v1.0.0-rc.1", "v1.0.0-rc.2", "v0.9.2"})
	u := &Updater{APIBase: srv.URL, Client: srv.Client(), Current: "v0.9.1", OS: "linux", Arch: "amd64", Out: io.Discard}
	rel, available, err := u.CheckTarget("prerelease", "")
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v1.0.0-rc.2" {
		t.Fatalf("prerelease channel picked %q, want the highest v1.0.0-rc.2", rel.TagName)
	}
	if !available {
		t.Fatal("v1.0.0-rc.2 is newer than v0.9.1 — should be available")
	}
}

func TestResolveVersionPinAndDowngradeRefused(t *testing.T) {
	srv := serveReleaseList(t, []string{"v0.9.1", "v1.0.0-rc.2"})
	u := &Updater{APIBase: srv.URL, Client: srv.Client(), Current: "v1.0.0-rc.2", OS: "linux", Arch: "amd64", Out: io.Discard}
	rel, available, err := u.CheckTarget("stable", "v0.9.1") // explicit older pin
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v0.9.1" {
		t.Fatalf("version pin resolved %q, want v0.9.1", rel.TagName)
	}
	if available {
		t.Fatal("pinning an older version must not report as an available update")
	}
}

func TestResolveInvalidVersionAndChannelRejected(t *testing.T) {
	u := &Updater{APIBase: "http://127.0.0.1:0", Client: http.DefaultClient}
	if _, err := u.Resolve("stable", "../etc/passwd"); err == nil || !strings.Contains(err.Error(), "invalid --version") {
		t.Fatalf("invalid version: err = %v", err)
	}
	if _, err := u.Resolve("weird", ""); err == nil || !strings.Contains(err.Error(), "unknown channel") {
		t.Fatalf("unknown channel: err = %v", err)
	}
}

func TestRestrictedClientRejectsNonHTTPSRedirects(t *testing.T) {
	client := restrictedClient()
	via, _ := http.NewRequest(http.MethodGet, "https://api.github.com/repos/"+Repo+"/releases/latest", nil)
	cases := []struct {
		name    string
		target  string
		wantErr string
	}{
		{name: "same host https", target: "https://github.com/" + Repo + "/releases/download/v1.0.0/oma_v1.0.0_linux_amd64"},
		{name: "githubusercontent https", target: "https://objects.githubusercontent.com/github-production-release-asset-2e65be/asset"},
		{name: "same host http", target: "http://github.com/" + Repo + "/releases/download/v1.0.0/oma_v1.0.0_linux_amd64", wantErr: "non-HTTPS"},
		{name: "foreign host https", target: "https://evil.example.com/asset", wantErr: "foreign host"},
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

func sawRequest(requests []string, want string) bool {
	for _, got := range requests {
		if got == want {
			return true
		}
	}
	return false
}
