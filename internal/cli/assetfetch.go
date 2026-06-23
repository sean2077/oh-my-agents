package cli

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The content-asset release channel. `oma asset install --ref <tag>` pulls the
// assets bundle published with a release and verifies it against the SAME
// checksums.txt self-update consumes (docs/reference/security-contract.md §5),
// so a clean machine can install core4 without a repo checkout and without ever
// fetching an unpinned ref. This mirrors internal/update's security posture —
// https-only, pinned to THIS repository's release downloads, checksum-verified,
// size-limited, redirect-restricted — and is deliberately a separate, self-
// contained path so the security-reviewed updater stays frozen.

const (
	assetsBundleMaxCompressedBytes = 64 << 20
	assetsBundleMaxEntryBytes      = 64 << 20
	assetsBundleMaxExtractedBytes  = 128 << 20
	assetsBundleMaxEntries         = 10_000
)

type assetFetchLimits struct {
	CompressedBytes int64
	EntryBytes      int64
	ExtractedBytes  int64
	Entries         int
}

var (
	refRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	repoRe = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)
)

// assetFetcher fetches and verifies a release's assets bundle. Every external
// touchpoint is injectable so tests run fully offline.
type assetFetcher struct {
	Repo         string
	Ref          string
	DownloadBase string // default https://github.com (overridden in tests)
	Client       *http.Client
	Out          io.Writer
	Limits       assetFetchLimits
}

var assetReleaseDownloadBase = "https://github.com"

func newAssetFetcher(repo, ref string, out io.Writer) *assetFetcher {
	return &assetFetcher{
		Repo:         repo,
		Ref:          ref,
		DownloadBase: assetReleaseDownloadBase,
		Client:       assetRestrictedClient(),
		Out:          out,
	}
}

// assetRestrictedClient refuses redirects that leave the GitHub release
// domains (same rule as the updater's client).
func assetRestrictedClient() *http.Client {
	return &http.Client{
		Timeout: 120 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			if req.URL.Scheme != "https" {
				return fmt.Errorf("redirect to non-HTTPS URL refused")
			}
			host := req.URL.Hostname()
			if host == "github.com" || strings.HasSuffix(host, ".githubusercontent.com") {
				return nil
			}
			return fmt.Errorf("redirect to foreign host %q refused", host)
		},
	}
}

func (f *assetFetcher) bundleName() string { return "assets-" + f.Ref + ".tar.gz" }
func (f *assetFetcher) base() string {
	return fmt.Sprintf("%s/%s/releases/download/%s", f.DownloadBase, f.Repo, f.Ref)
}
func (f *assetFetcher) bundleURL() string { return f.base() + "/" + f.bundleName() }

func (f *assetFetcher) limits() assetFetchLimits {
	limits := assetFetchLimits{
		CompressedBytes: assetsBundleMaxCompressedBytes,
		EntryBytes:      assetsBundleMaxEntryBytes,
		ExtractedBytes:  assetsBundleMaxExtractedBytes,
		Entries:         assetsBundleMaxEntries,
	}
	if f.Limits.CompressedBytes > 0 {
		limits.CompressedBytes = f.Limits.CompressedBytes
	}
	if f.Limits.EntryBytes > 0 {
		limits.EntryBytes = f.Limits.EntryBytes
	}
	if f.Limits.ExtractedBytes > 0 {
		limits.ExtractedBytes = f.Limits.ExtractedBytes
	}
	if f.Limits.Entries > 0 {
		limits.Entries = f.Limits.Entries
	}
	return limits
}

// validate guards the repo/ref before they reach any URL: ref pins the release
// download path, so it must be a clean tag with no separators or traversal.
func (f *assetFetcher) validate() error {
	if !repoRe.MatchString(f.Repo) {
		return Errf(ExitUsage, "invalid --repo %q (want owner/name)", f.Repo)
	}
	if !refRe.MatchString(f.Ref) || strings.Contains(f.Ref, "..") {
		return Errf(ExitUsage, "invalid --ref %q (want a release tag)", f.Ref)
	}
	return nil
}

// Fetch downloads assets-<ref>.tar.gz, verifies it against the release
// checksums.txt, and extracts it under a fresh temp dir, returning the
// extracted assets root (skills/agents/hooks/prompts) plus a cleanup func.
func (f *assetFetcher) Fetch(parent string) (root string, cleanup func(), err error) {
	if err := f.validate(); err != nil {
		return "", nil, err
	}
	bundle := f.bundleName()
	sums, err := f.fetchChecksums(f.base()+"/checksums.txt", bundle)
	if err != nil {
		return "", nil, err
	}
	wantSHA, ok := sums[bundle]
	if !ok {
		return "", nil, Errf(ExitState, "release %s ships no %s (no asset bundle); install with --from <checkout>/assets instead", f.Ref, bundle)
	}

	tmp, err := os.MkdirTemp(parent, "oma-assets-")
	if err != nil {
		return "", nil, Errf(ExitState, "create temp dir: %v", err)
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	limits := f.limits()

	tarPath := filepath.Join(tmp, bundle)
	if err := f.download(tarPath, f.bundleURL(), wantSHA, limits.CompressedBytes); err != nil {
		cleanup()
		return "", nil, err
	}
	root = filepath.Join(tmp, "assets")
	if err := extractTarGz(tarPath, root, limits); err != nil {
		cleanup()
		return "", nil, Errf(ExitState, "extract %s: %v", bundle, err)
	}
	_ = os.Remove(tarPath)
	return root, cleanup, nil
}

// fetchChecksums parses sha256sum format: "<hex>  <name>" per line. Duplicate
// entries for consumed names are fail-closed; duplicates for unrelated release
// assets are ignored because this path validates only the bundle it consumes.
func (f *assetFetcher) fetchChecksums(url string, consumedNames ...string) (map[string]string, error) {
	resp, err := f.Client.Get(url)
	if err != nil {
		return nil, Errf(ExitState, "checksums download failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, Errf(ExitState, "checksums download failed (HTTP %d from %s)", resp.StatusCode, url)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, Errf(ExitState, "checksums read failed: %v", err)
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
			return nil, Errf(ExitState, "checksums.txt has duplicate entries for %s", name)
		}
		sums[name] = strings.ToLower(fields[0])
	}
	if len(sums) == 0 {
		return nil, Errf(ExitState, "checksums.txt is empty or malformed")
	}
	return sums, nil
}

// download streams the bundle to path while hashing, then verifies the digest
// BEFORE it can be used (mismatch deletes it).
func (f *assetFetcher) download(path, url, wantHex string, maxBytes int64) error {
	resp, err := f.Client.Get(url)
	if err != nil {
		return Errf(ExitState, "asset bundle download failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Errf(ExitState, "asset bundle download failed (HTTP %d from %s)", resp.StatusCode, url)
	}
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return Errf(ExitState, "create %s: %v", path, err)
	}
	h := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(out, h), io.LimitReader(resp.Body, maxBytes+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return Errf(ExitState, "download %s: %v", filepath.Base(path), firstErr(copyErr, closeErr))
	}
	if written > maxBytes {
		_ = os.Remove(path)
		return Errf(ExitState, "asset bundle %s exceeds the %d-byte compressed limit", filepath.Base(path), maxBytes)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantHex {
		_ = os.Remove(path)
		return Errf(ExitState, "checksum mismatch for %s (got %s, want %s)", filepath.Base(path), got, wantHex)
	}
	return nil
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// extractTarGz unpacks a gzip'd tar under root, refusing entries that would
// escape root (zip-slip), non-file/dir entries (no symlinks/devices), oversized
// entries, oversized total extracted content, or excessive entry counts.
func extractTarGz(tarPath, root string, limits assetFetchLimits) error {
	fh, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer func() { _ = fh.Close() }()
	gz, err := gzip.NewReader(fh)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	cleanRoot := filepath.Clean(root)
	tr := tar.NewReader(gz)
	var totalBytes int64
	entryCount := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		entryCount++
		if entryCount > limits.Entries {
			return fmt.Errorf("archive has more than %d entries", limits.Entries)
		}
		name := filepath.Clean(hdr.Name)
		if name == "." {
			continue
		}
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) || strings.Contains(name, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path %q in archive", hdr.Name)
		}
		target := filepath.Join(cleanRoot, name)
		if target != cleanRoot && !strings.HasPrefix(target, cleanRoot+string(os.PathSeparator)) {
			return fmt.Errorf("path %q escapes the extraction root", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if hdr.Size < 0 {
				return fmt.Errorf("archive entry %q has negative size %d", hdr.Name, hdr.Size)
			}
			if hdr.Size > limits.EntryBytes {
				return fmt.Errorf("archive entry %q (%d bytes) exceeds the %d-byte per-entry limit", hdr.Name, hdr.Size, limits.EntryBytes)
			}
			if totalBytes > limits.ExtractedBytes-hdr.Size {
				return fmt.Errorf("archive extracted size exceeds the %d-byte limit", limits.ExtractedBytes)
			}
			totalBytes += hdr.Size
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			// Read one byte past the cap so a header under-reporting its size
			// (streaming more than declared) is still caught as truncation.
			written, copyErr := io.Copy(out, io.LimitReader(tr, limits.EntryBytes+1))
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if written > limits.EntryBytes {
				return fmt.Errorf("archive entry %q exceeds the %d-byte limit (truncation refused)", hdr.Name, limits.EntryBytes)
			}
		default:
			return fmt.Errorf("unsupported archive entry %q (type %d): only regular files and directories are allowed", hdr.Name, hdr.Typeflag)
		}
	}
	return nil
}
