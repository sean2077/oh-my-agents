// Package update implements oma self-update with the
// docs/reference/security-contract.md §5 trust chain: the update source is pinned
// to this repository's GitHub Releases at compile time, only the assets the
// updater consumes are validated (the platform binary + checksums.txt; auxiliary
// assets such as SBOMs, signatures and attestations are ignored), updates move
// strictly forward by semantic version (a downgrade is refused unless explicitly
// allowed), every download is verified against the release's checksums.txt,
// replacement is atomic with an .old backup and a post-replace self-check that
// auto-rolls-back, and an unwritable target degrades to printed manual
// instructions (never sudo).
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
	"strconv"
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
	TagName    string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
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
	TempDir    string                            // optional parent for dry-run validation downloads
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
			if req.URL.Scheme != "https" {
				return fmt.Errorf("%w: redirect to non-HTTPS URL refused", ErrUpdate)
			}
			host := req.URL.Hostname()
			if host == "api.github.com" || host == "github.com" || strings.HasSuffix(host, ".githubusercontent.com") {
				return nil
			}
			return fmt.Errorf("%w: redirect to foreign host %q refused", ErrUpdate, host)
		},
	}
}

// assetName is the release naming contract (tools/release/build-release.sh).
func (u *Updater) assetName(tag string) string {
	name := fmt.Sprintf("oma_%s_%s_%s", tag, u.OS, u.Arch)
	if u.OS == "windows" {
		name += ".exe"
	}
	return name
}

var releaseTagRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Check resolves the latest stable release (the default channel). Strictly
// read-only (zero filesystem writes).
func (u *Updater) Check() (*Release, bool, error) {
	return u.CheckTarget("stable", "")
}

// CheckTarget resolves the release selected by channel/version and reports
// whether it is a strictly newer semantic version than the running binary —
// strictly read-only. Auxiliary release assets the updater does not consume
// (SBOMs, signatures, attestations) are ignored here; only the assets Apply
// consumes are validated, and only at apply time (security-contract.md §5: a
// supply-chain artifact added to a release can never break the update channel).
func (u *Updater) CheckTarget(channel, version string) (*Release, bool, error) {
	rel, err := u.Resolve(channel, version)
	if err != nil {
		return nil, false, err
	}
	return rel, u.updateAvailable(rel.TagName), nil
}

// Resolve fetches the target release document. An explicit version pins a tag;
// the "prerelease" channel selects the highest semantic version including
// prereleases; otherwise the latest stable release (GitHub's /releases/latest
// excludes prereleases).
func (u *Updater) Resolve(channel, version string) (*Release, error) {
	switch {
	case version != "":
		if !releaseTagRe.MatchString(version) {
			return nil, fmt.Errorf("%w: invalid --version %q (want a release tag)", ErrUpdate, version)
		}
		return u.fetchRelease(fmt.Sprintf("%s/repos/%s/releases/tags/%s", u.APIBase, Repo, version))
	case channel == "prerelease":
		return u.latestIncludingPrerelease()
	case channel == "" || channel == "stable":
		return u.fetchRelease(fmt.Sprintf("%s/repos/%s/releases/latest", u.APIBase, Repo))
	default:
		return nil, fmt.Errorf("%w: unknown channel %q (want stable or prerelease)", ErrUpdate, channel)
	}
}

// fetchRelease GETs and validates a single release document.
func (u *Updater) fetchRelease(endpoint string) (*Release, error) {
	resp, err := u.Client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: release metadata unavailable: %v", ErrUpdate, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: release metadata unavailable (HTTP %d from %s)", ErrUpdate, resp.StatusCode, endpoint)
	}
	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("%w: release metadata not valid JSON: %v", ErrUpdate, err)
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("%w: release has no tag name", ErrUpdate)
	}
	// A remote tag we cannot parse is unsafe to compare against → fail closed.
	if _, _, ok := parseSemver(rel.TagName); !ok {
		return nil, fmt.Errorf("%w: release tag %q is not a valid semantic version", ErrUpdate, rel.TagName)
	}
	return &rel, nil
}

// latestIncludingPrerelease lists releases and returns the highest semantic
// version, prereleases included (the prerelease channel). Drafts are skipped.
func (u *Updater) latestIncludingPrerelease() (*Release, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/releases?per_page=50", u.APIBase, Repo)
	resp, err := u.Client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: release list unavailable: %v", ErrUpdate, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: release list unavailable (HTTP %d from %s)", ErrUpdate, resp.StatusCode, endpoint)
	}
	var rels []Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&rels); err != nil {
		return nil, fmt.Errorf("%w: release list not valid JSON: %v", ErrUpdate, err)
	}
	var best *Release
	for i := range rels {
		if rels[i].Draft {
			continue
		}
		if _, _, ok := parseSemver(rels[i].TagName); !ok {
			continue
		}
		if best == nil {
			best = &rels[i]
			continue
		}
		if order, ok := CompareVersions(rels[i].TagName, best.TagName); ok && order > 0 {
			best = &rels[i]
		}
	}
	if best == nil {
		return nil, fmt.Errorf("%w: no published release with a valid version", ErrUpdate)
	}
	return best, nil
}

// updateAvailable reports whether remote is a strictly newer version than the
// running binary. An unparseable running version (a dev/source build) is
// treated as older than any release, so self-update can still move a dev build
// onto a real release.
func (u *Updater) updateAvailable(remote string) bool {
	order, ok := CompareVersions(u.Current, remote)
	if !ok {
		return true
	}
	return order < 0
}

// Apply downloads, verifies and atomically installs the release over the
// running binary. dryRun discloses the exact paths and writes no persistent
// target state; it may use an auto-cleaned private temp dir to validate the
// remote artifact.
// allowDowngrade permits replacing the running binary with an equal or older
// release; without it a downgrade is refused fail-closed.
func (u *Updater) Apply(rel *Release, dryRun, allowDowngrade bool) error {
	wantName := u.assetName(rel.TagName)
	binAsset, sumAsset, err := u.findAssets(rel, wantName)
	if err != nil {
		return err
	}
	if binAsset == nil {
		return fmt.Errorf("%w: release %s has no asset %q for this platform", ErrUpdate, rel.TagName, wantName)
	}
	if sumAsset == nil {
		return fmt.Errorf("%w: release %s has no checksums.txt (unverifiable)", ErrUpdate, rel.TagName)
	}
	// Never silently move backward: a release older than the running binary is
	// refused unless the caller explicitly opted into a downgrade.
	if !allowDowngrade {
		if order, ok := CompareVersions(u.Current, rel.TagName); ok && order > 0 {
			return fmt.Errorf("%w: running %s is newer than release %s; refusing downgrade (re-run with --allow-downgrade)", ErrUpdate, u.Current, rel.TagName)
		}
	}
	if err := u.checkSourceURL(binAsset.URL); err != nil {
		return err
	}
	if err := u.checkSourceURL(sumAsset.URL); err != nil {
		return err
	}

	dir := filepath.Dir(u.BinaryPath)
	old := u.BinaryPath + ".old"
	// Zero-write validation path, shared by dry-run and real execution
	// (review 064 blocker 1: the old probe-file check WROTE during
	// --dry-run; canWriteDir never creates anything).
	if !canWriteDir(dir) {
		_, _ = fmt.Fprintf(u.Out, "binary directory %s is not writable; update manually:\n", dir)
		_, _ = fmt.Fprintf(u.Out, "  1. download %s\n  2. verify against checksums.txt\n  3. replace %s\n", binAsset.URL, u.BinaryPath)
		return fmt.Errorf("%w: %s not writable (manual instructions printed; oma never elevates)", ErrUpdate, dir)
	}
	// .old plays two roles and only ONE is fatal (review 064 blocker 2):
	// with the binary itself missing, .old is the interrupted-swap
	// recovery copy — refuse and point at it. With the binary present,
	// .old is just the previous successful backup and gets rotated.
	if _, err := os.Lstat(u.BinaryPath); err != nil {
		if _, oldErr := os.Lstat(old); oldErr == nil {
			return fmt.Errorf("%w: %s is missing but %s exists (interrupted swap; restore it manually: mv %s %s)", ErrUpdate, u.BinaryPath, old, old, u.BinaryPath)
		}
		return fmt.Errorf("%w: %s does not exist", ErrUpdate, u.BinaryPath)
	}
	sums, err := u.fetchChecksums(sumAsset.URL, wantName)
	if err != nil {
		return err
	}
	wantSum, ok := sums[wantName]
	if !ok {
		return fmt.Errorf("%w: checksums.txt has no entry for %q", ErrUpdate, wantName)
	}
	if dryRun {
		validationDir, err := os.MkdirTemp(u.TempDir, "oma-self-update-")
		if err != nil {
			return fmt.Errorf("%w: create dry-run validation temp dir: %v", ErrUpdate, err)
		}
		defer func() { _ = os.RemoveAll(validationDir) }()
		validationPath := filepath.Join(validationDir, wantName)
		if err := u.prepareCandidate(validationPath, binAsset.URL, wantSum, rel.TagName); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(u.Out, "would download %s\nwould verify against %s\nwould write a private temp dir under %s\nwould backup %s -> %s (rotating any previous backup)\nwould replace %s\n",
			binAsset.URL, sumAsset.URL, dir, u.BinaryPath, old, u.BinaryPath)
		return nil
	}

	// Download into a same-directory PRIVATE temp dir (0700, unique name) so the
	// final rename stays atomic AND no predictable path in a shared bin dir can
	// be pre-created or symlinked by another user (the fixed `<bin>.oma-update-tmp`
	// was both predictable and shared across concurrent runs).
	tmpDir, err := os.MkdirTemp(dir, ".oma-update-")
	if err != nil {
		return fmt.Errorf("%w: create update temp dir in %s: %v", ErrUpdate, dir, err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	tmp := filepath.Join(tmpDir, wantName)
	if err := u.prepareCandidate(tmp, binAsset.URL, wantSum, rel.TagName); err != nil {
		return err
	}

	// Swap: current -> .old (atomically rotating a previous successful
	// backup), tmp -> current; verify; roll back on failure.
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
	_, _ = fmt.Fprintf(u.Out, "updated %s -> %s (previous kept at %s)\n", u.Current, rel.TagName, old)
	return nil
}

func (u *Updater) prepareCandidate(path, rawURL, wantSum, tag string) error {
	if err := u.downloadTo(path, rawURL, wantSum); err != nil {
		_ = os.Remove(path)
		return err
	}
	if err := os.Chmod(path, 0o755); err != nil {
		_ = os.Remove(path)
		return err
	}
	// Pre-flight the NEW binary before it takes over the real path.
	if got, err := u.SelfCheck(path); err != nil || got != tag {
		_ = os.Remove(path)
		return fmt.Errorf("%w: downloaded binary failed self-check (version %q err=%v); current binary untouched", ErrUpdate, got, err)
	}
	return nil
}

// findAssets locates exactly the two assets Apply consumes — the current
// platform binary and checksums.txt — and fails closed if either appears more
// than once (an ambiguous release must never be silently resolved). Every other
// asset in the release is ignored.
func (u *Updater) findAssets(rel *Release, wantName string) (bin, sums *Asset, err error) {
	for i := range rel.Assets {
		switch rel.Assets[i].Name {
		case wantName:
			if bin != nil {
				return nil, nil, fmt.Errorf("%w: release %s has more than one %q asset", ErrUpdate, rel.TagName, wantName)
			}
			bin = &rel.Assets[i]
		case "checksums.txt":
			if sums != nil {
				return nil, nil, fmt.Errorf("%w: release %s has more than one checksums.txt", ErrUpdate, rel.TagName)
			}
			sums = &rel.Assets[i]
		}
	}
	return bin, sums, nil
}

// checkSourceURL pins download URLs to the release locations of THIS
// repository (or GitHub's asset CDN).
func (u *Updater) checkSourceURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: asset URL %q is not parseable", ErrUpdate, raw)
	}
	// The injected APIBase prefix is the one trusted non-https-checked source
	// (api.github.com in production, the httptest server under test); the
	// trailing "/" blocks prefix-confusion (e.g. api.github.com.evil.com). EVERY
	// other URL must be https — unconditionally, no longer coupled to whether
	// APIBase happens to be the production default.
	if strings.HasPrefix(raw, u.APIBase+"/") {
		return nil
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("%w: asset URL %q is not https", ErrUpdate, raw)
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

// fetchChecksums parses sha256sum format: "<hex>  <name>" per line. Duplicate
// entries for consumed names are fail-closed; duplicates for unrelated release
// assets are ignored because self-update validates only the platform binary it
// consumes.
func (u *Updater) fetchChecksums(rawURL string, consumedNames ...string) (map[string]string, error) {
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
	consumed := map[string]bool{}
	for _, name := range consumedNames {
		consumed[name] = true
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != 64 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if _, exists := sums[name]; exists && consumed[name] {
			return nil, fmt.Errorf("%w: checksums.txt has duplicate entries for %s", ErrUpdate, name)
		}
		sums[name] = strings.ToLower(fields[0])
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

// --- semantic version comparison (security-contract.md §5) ------------------
//
// self-update compares versions by SemVer 2.0 precedence rather than string
// (in)equality, so it offers only genuine upgrades and refuses downgrades. The
// version space is our own release tags (vMAJOR.MINOR.PATCH[-prerelease]); a
// small focused comparator keeps the binary dependency-free.

// Core identifiers reject leading zeros (SemVer §2). Prerelease numeric
// identifiers do too (§9) — enforced below, since the regex alone can't.
var semverRe = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.-]+))?(?:\+([0-9A-Za-z.-]+))?$`)

// parseSemver extracts the MAJOR.MINOR.PATCH core and the prerelease string
// (build metadata is accepted but ignored, per SemVer). ok is false for any
// input that is not a recognizable semantic version (e.g. "dev", "main", "",
// or a tag with leading-zero numeric identifiers like "v01.2.3" / "rc.01").
func parseSemver(s string) (core [3]int, prerelease string, ok bool) {
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return core, "", false
	}
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return core, "", false
		}
		core[i] = n
	}
	if pre := m[4]; pre != "" {
		for _, id := range strings.Split(pre, ".") {
			// Empty identifier (a stray/leading/trailing dot) is invalid, and a
			// purely-numeric identifier must not carry a leading zero (§9).
			if id == "" {
				return core, "", false
			}
			if isAllDigits(id) && len(id) > 1 && id[0] == '0' {
				return core, "", false
			}
		}
	}
	if build := m[5]; build != "" {
		// Build metadata is dot-separated identifiers that must each be
		// non-empty (§10); it is otherwise ignored for precedence.
		for _, id := range strings.Split(build, ".") {
			if id == "" {
				return core, "", false
			}
		}
	}
	return core, m[4], true
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// IsSemver reports whether s is a parseable semantic version (a real release
// tag) rather than a dev/source build stamp like "dev" or "main". `oma asset
// install` uses it to decide whether the running binary's version can pin a
// release to fetch assets from.
func IsSemver(s string) bool {
	_, _, ok := parseSemver(s)
	return ok
}

// CompareVersions reports -1/0/+1 for a<b / a==b / a>b under SemVer precedence.
// ok is false when either side is not a valid semantic version, leaving the
// ordering undefined (callers decide the dev-build policy).
func CompareVersions(a, b string) (order int, ok bool) {
	ac, apre, aok := parseSemver(a)
	bc, bpre, bok := parseSemver(b)
	if !aok || !bok {
		return 0, false
	}
	for i := 0; i < 3; i++ {
		if ac[i] != bc[i] {
			if ac[i] < bc[i] {
				return -1, true
			}
			return 1, true
		}
	}
	return comparePrerelease(apre, bpre), true
}

// comparePrerelease implements SemVer §11.4: a version carrying a prerelease has
// lower precedence than the same core without one, and prerelease identifiers
// compare field-by-field — numeric numerically, alphanumeric lexically, numeric
// below alphanumeric, and a longer field list winning when all prior fields tie.
func comparePrerelease(a, b string) int {
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return 1 // a has no prerelease → higher precedence
	case b == "":
		return -1
	}
	ai, bi := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(ai) && i < len(bi); i++ {
		if c := compareIdent(ai[i], bi[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(ai) < len(bi):
		return -1
	case len(ai) > len(bi):
		return 1
	default:
		return 0
	}
}

func compareIdent(a, b string) int {
	an, aNum := atoiOK(a)
	bn, bNum := atoiOK(b)
	switch {
	case aNum && bNum:
		switch {
		case an < bn:
			return -1
		case an > bn:
			return 1
		default:
			return 0
		}
	case aNum:
		return -1 // a numeric identifier is lower than an alphanumeric one
	case bNum:
		return 1
	default:
		switch {
		case a < b:
			return -1
		case a > b:
			return 1
		default:
			return 0
		}
	}
}

func atoiOK(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}
