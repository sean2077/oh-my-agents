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

const assetsBundleMaxBytes = 64 << 20 // a generous ceiling for the assets/ tree

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
}

func newAssetFetcher(repo, ref string, out io.Writer) *assetFetcher {
	return &assetFetcher{
		Repo:         repo,
		Ref:          ref,
		DownloadBase: "https://github.com",
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
	sums, err := f.fetchChecksums(f.base() + "/checksums.txt")
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

	tarPath := filepath.Join(tmp, bundle)
	if err := f.download(tarPath, f.bundleURL(), wantSHA); err != nil {
		cleanup()
		return "", nil, err
	}
	root = filepath.Join(tmp, "assets")
	if err := extractTarGz(tarPath, root); err != nil {
		cleanup()
		return "", nil, Errf(ExitState, "extract %s: %v", bundle, err)
	}
	_ = os.Remove(tarPath)
	return root, cleanup, nil
}

// fetchChecksums parses sha256sum format: "<hex>  <name>" per line.
func (f *assetFetcher) fetchChecksums(url string) (map[string]string, error) {
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
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != 64 {
			continue
		}
		sums[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
	}
	if len(sums) == 0 {
		return nil, Errf(ExitState, "checksums.txt is empty or malformed")
	}
	return sums, nil
}

// download streams the bundle to path while hashing, then verifies the digest
// BEFORE it can be used (mismatch deletes it).
func (f *assetFetcher) download(path, url, wantHex string) error {
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
	_, copyErr := io.Copy(io.MultiWriter(out, h), io.LimitReader(resp.Body, assetsBundleMaxBytes))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return Errf(ExitState, "download %s: %v", filepath.Base(path), firstErr(copyErr, closeErr))
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

// extractTarGz unpacks a gzip'd tar under root, refusing any entry that would
// escape root (zip-slip) or is not a plain file/dir (no symlinks/devices).
func extractTarGz(tarPath, root string) error {
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
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
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
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, io.LimitReader(tr, assetsBundleMaxBytes)); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry %q (type %d): only regular files and directories are allowed", hdr.Name, hdr.Typeflag)
		}
	}
	return nil
}
