// Package update implements oma self-update with the
// docs/security-contract.md §5 trust chain: the update source is pinned
// to this repository's GitHub Releases at compile time, asset names must
// match the release naming contract, every download is verified against
// the release's checksums.txt, replacement is atomic with an .old backup
// and a post-replace self-check that auto-rolls-back, and an unwritable
// target degrades to printed manual instructions (never sudo).
package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Repo is the COMPILE-TIME pinned update source (security-contract §5):
// self-update never follows configuration to another repository.
const Repo = "sean2077/oh-my-agents"

// ErrUpdate marks fail-closed self-update refusals.
var ErrUpdate = errors.New("self-update refused (fail-closed)")

// Release is the subset of the GitHub release document oma reads.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset is one release asset.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// Updater performs the check/apply flow. Every external touchpoint is
// injectable so the §5 test matrix runs fully offline.
type Updater struct {
	APIBase    string                            // default https://api.github.com
	Client     *http.Client                      // default: redirect-restricted client
	BinaryPath string                            // default: os.Executable()
	Current    string                            // running version (version.Version)
	OS, Arch   string                            // default runtime.GOOS/GOARCH
	SelfCheck  func(path string) (string, error) // default: run `path version --json`
	Out        io.Writer                         // progress / manual instructions
}

// New builds a production Updater for the running binary.
func New(current string, out io.Writer) (*Updater, error) {
	bin, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("%w: cannot resolve own binary path: %v", ErrUpdate, err)
	}
	bin, err = filepath.EvalSymlinks(bin)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot resolve own binary path: %v", ErrUpdate, err)
	}
	return &Updater{
		APIBase:    "https://api.github.com",
		Client:     restrictedClient(),
		BinaryPath: bin,
		Current:    current,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		SelfCheck:  runVersionProbe,
		Out:        out,
	}, nil
}

// restrictedClient refuses redirects that leave the GitHub release
// domains (security-contract §5: no cross-repo/cross-domain redirects).
func restrictedClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("%w: too many redirects", ErrUpdate)
			}
			host := req.URL.Hostname()
			if host == "api.github.com" || host == "github.com" || strings.HasSuffix(host, ".githubusercontent.com") {
				return nil
			}
			return fmt.Errorf("%w: redirect to foreign host %q refused", ErrUpdate, host)
		},
	}
}

// assetName is the release naming contract (scripts/build-release.sh).
func (u *Updater) assetName(tag string) string {
	name := fmt.Sprintf("oma_%s_%s_%s", tag, u.OS, u.Arch)
	if u.OS == "windows" {
		name += ".exe"
	}
	return name
}

var assetNameRe = regexp.MustCompile(`^oma_[A-Za-z0-9.+-]+_[a-z0-9]+_[a-z0-9]+(\.exe)?$`)

// Check queries the pinned source for the latest release — strictly
// read-only (zero filesystem writes). It returns the release and whether
// it differs from the running version.
func (u *Updater) Check() (*Release, bool, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/releases/latest", u.APIBase, Repo)
	resp, err := u.Client.Get(endpoint)
	if err != nil {
		return nil, false, fmt.Errorf("%w: release metadata unavailable: %v", ErrUpdate, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("%w: release metadata unavailable (HTTP %d from %s)", ErrUpdate, resp.StatusCode, endpoint)
	}
	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return nil, false, fmt.Errorf("%w: release metadata not valid JSON: %v", ErrUpdate, err)
	}
	if rel.TagName == "" {
		return nil, false, fmt.Errorf("%w: release has no tag name", ErrUpdate)
	}
	for _, a := range rel.Assets {
		if !assetNameRe.MatchString(a.Name) && a.Name != "checksums.txt" {
			return nil, false, fmt.Errorf("%w: unexpected asset name %q in release %s (naming contract violated)", ErrUpdate, a.Name, rel.TagName)
		}
	}
	return &rel, rel.TagName != u.Current, nil
}

// Apply downloads, verifies and atomically installs the release over the
// running binary. dryRun discloses the exact paths and writes nothing.
func (u *Updater) Apply(rel *Release, dryRun bool) error {
	wantName := u.assetName(rel.TagName)
	binAsset, sumAsset := u.findAssets(rel, wantName)
	if binAsset == nil {
		return fmt.Errorf("%w: release %s has no asset %q for this platform", ErrUpdate, rel.TagName, wantName)
	}
	if sumAsset == nil {
		return fmt.Errorf("%w: release %s has no checksums.txt (unverifiable)", ErrUpdate, rel.TagName)
	}
	if err := u.checkSourceURL(binAsset.URL); err != nil {
		return err
	}
	if err := u.checkSourceURL(sumAsset.URL); err != nil {
		return err
	}

	dir := filepath.Dir(u.BinaryPath)
	tmp := u.BinaryPath + ".oma-update-tmp"
	old := u.BinaryPath + ".old"
	if !dirWritable(dir) {
		fmt.Fprintf(u.Out, "binary directory %s is not writable; update manually:\n", dir)
		fmt.Fprintf(u.Out, "  1. download %s\n  2. verify against checksums.txt\n  3. replace %s\n", binAsset.URL, u.BinaryPath)
		return fmt.Errorf("%w: %s not writable (manual instructions printed; oma never elevates)", ErrUpdate, dir)
	}
	if _, err := os.Lstat(old); err == nil {
		return fmt.Errorf("%w: recovery sibling %s already exists (a previous update was interrupted; inspect and remove it first)", ErrUpdate, old)
	}
	if dryRun {
		fmt.Fprintf(u.Out, "would download %s\nwould verify against %s\nwould write %s\nwould backup %s -> %s\nwould replace %s\n",
			binAsset.URL, sumAsset.URL, tmp, u.BinaryPath, old, u.BinaryPath)
		return nil
	}

	sums, err := u.fetchChecksums(sumAsset.URL)
	if err != nil {
		return err
	}
	wantSum, ok := sums[wantName]
	if !ok {
		return fmt.Errorf("%w: checksums.txt has no entry for %q", ErrUpdate, wantName)
	}

	// Download to a same-directory temp so the final rename is atomic.
	if err := u.downloadTo(tmp, binAsset.URL, wantSum); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// Pre-flight the NEW binary before it takes over the real path.
	if got, err := u.SelfCheck(tmp); err != nil || got != rel.TagName {
		_ = os.Remove(tmp)
		return fmt.Errorf("%w: downloaded binary failed self-check (version %q err=%v); current binary untouched", ErrUpdate, got, err)
	}

	// Swap: current -> .old, tmp -> current; verify; roll back on failure.
	if err := os.Rename(u.BinaryPath, old); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, u.BinaryPath); err != nil {
		_ = os.Rename(old, u.BinaryPath) // restore
		_ = os.Remove(tmp)
		return fmt.Errorf("%w: swap failed, previous binary restored: %v", ErrUpdate, err)
	}
	if got, err := u.SelfCheck(u.BinaryPath); err != nil || got != rel.TagName {
		_ = os.Rename(u.BinaryPath, tmp) // move the bad one aside…
		_ = os.Rename(old, u.BinaryPath) // …and bring the previous back
		_ = os.Remove(tmp)
		return fmt.Errorf("%w: post-replace self-check failed (version %q err=%v); previous binary rolled back", ErrUpdate, got, err)
	}
	fmt.Fprintf(u.Out, "updated %s -> %s (previous kept at %s)\n", u.Current, rel.TagName, old)
	return nil
}

func (u *Updater) findAssets(rel *Release, wantName string) (bin, sums *Asset) {
	for i := range rel.Assets {
		switch rel.Assets[i].Name {
		case wantName:
			bin = &rel.Assets[i]
		case "checksums.txt":
			sums = &rel.Assets[i]
		}
	}
	return bin, sums
}

// checkSourceURL pins download URLs to the release locations of THIS
// repository (or GitHub's asset CDN).
func (u *Updater) checkSourceURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" && u.APIBase == "https://api.github.com" {
		return fmt.Errorf("%w: asset URL %q is not https", ErrUpdate, raw)
	}
	if strings.HasPrefix(raw, u.APIBase+"/") {
		return nil // test servers and api.github.com asset endpoints
	}
	host := parsed.Hostname()
	switch {
	case host == "github.com" && strings.HasPrefix(parsed.Path, "/"+Repo+"/releases/download/"):
		return nil
	case strings.HasSuffix(host, ".githubusercontent.com"):
		return nil
	default:
		return fmt.Errorf("%w: asset URL %q is outside the pinned source (%s releases)", ErrUpdate, raw, Repo)
	}
}

// fetchChecksums parses sha256sum format: "<hex>  <name>" per line.
func (u *Updater) fetchChecksums(rawURL string) (map[string]string, error) {
	resp, err := u.Client.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: checksums download failed: %v", ErrUpdate, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: checksums download failed (HTTP %d)", ErrUpdate, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	sums := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != 64 {
			continue
		}
		sums[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
	}
	if len(sums) == 0 {
		return nil, fmt.Errorf("%w: checksums.txt is empty or malformed", ErrUpdate)
	}
	return sums, nil
}

// downloadTo streams the asset to path while hashing, then verifies the
// digest BEFORE the file can be used (mismatch deletes it).
func (u *Updater) downloadTo(path, rawURL, wantHex string) error {
	resp, err := u.Client.Get(rawURL)
	if err != nil {
		return fmt.Errorf("%w: download failed: %v", ErrUpdate, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: download failed (HTTP %d)", ErrUpdate, resp.StatusCode)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 512<<20))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		return errors.Join(copyErr, closeErr)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantHex {
		return fmt.Errorf("%w: checksum mismatch for %s (got %s, want %s); current binary untouched", ErrUpdate, filepath.Base(path), got, wantHex)
	}
	return nil
}

func dirWritable(dir string) bool {
	probe := filepath.Join(dir, ".oma-writable-probe")
	f, err := os.OpenFile(probe, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return true
}

// runVersionProbe asks a binary for its version via `version --json`.
func runVersionProbe(path string) (string, error) {
	out, err := execCommand(path, "version", "--json")
	if err != nil {
		return "", err
	}
	var doc struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return "", fmt.Errorf("version output not valid JSON: %w", err)
	}
	return doc.Version, nil
}
