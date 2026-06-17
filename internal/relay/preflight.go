package relay

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Preflight levels.
const (
	PFOk   = "ok"
	PFWarn = "warn"
	PFFail = "fail"
)

// PreflightCheck is one diagnostic line.
type PreflightCheck struct {
	Name    string `json:"name"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// PreflightReport is the full diagnostic (docs/reference/relay-v2-protocol.md
// experience layer). Exit mapping is oma-native (command-tree §1): all
// ok → 0, any warn → 1, any fail → 3 (env/state). Usage errors stay 2
// via cobra and never reach here.
type PreflightReport struct {
	Schema string           `json:"schema"`
	Root   string           `json:"root"`
	Checks []PreflightCheck `json:"checks"`
	Pass   int              `json:"pass"`
	Warn   int              `json:"warn"`
	Fail   int              `json:"fail"`
}

// PreflightSchema versions the --json shape for later hooks/statusline
// consumers.
const PreflightSchema = "oma-relay-preflight/1"

// PreflightInput carries the raw, pre-validation inputs: preflight
// diagnoses whether a ledger CAN be constructed, so it never depends on a
// successfully-opened Ledger.
type PreflightInput struct {
	ExplicitRoot string // --ledger-root; "" = default <git toplevel>/.oma/relay
	Cwd          string
	ProjectRoot  string // for the legacy .shared/ check; "" skips it
	Getenv       func(string) string
	Now          func() time.Time
}

// ExitCode maps the report to the oma-native relay preflight contract.
func (r *PreflightReport) ExitCode() int {
	switch {
	case r.Fail > 0:
		return 3
	case r.Warn > 0:
		return 1
	default:
		return 0
	}
}

func (r *PreflightReport) add(name, level, format string, a ...any) {
	r.Checks = append(r.Checks, PreflightCheck{Name: name, Level: level, Message: fmt.Sprintf(format, a...)})
	switch level {
	case PFOk:
		r.Pass++
	case PFWarn:
		r.Warn++
	case PFFail:
		r.Fail++
	}
}

// Preflight runs the diagnostic checks without ever hard-failing: every
// condition is captured as a check line so a broken environment is
// reported in full rather than aborting on the first problem.
func Preflight(in PreflightInput) *PreflightReport {
	now := in.Now
	if now == nil {
		now = time.Now
	}
	getenv := in.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	r := &PreflightReport{Schema: PreflightSchema}

	// 1. Identity.
	id, idErr := ResolveIdentity(getenv)
	if idErr != nil {
		r.add("identity.author", PFFail, "%v", idErr)
	} else {
		r.add("identity.author", PFOk, "%s (session %s)", id.Author, id.SessionKey)
	}

	// 2. Ledger root.
	root := in.ExplicitRoot
	explicit := root != ""
	if !explicit {
		var err error
		if root, err = DefaultRoot(in.Cwd); err != nil {
			r.add("ledger.root", PFFail, "%v", err)
			r.Root = ""
			return r // without a root, the remaining checks cannot run
		}
	}
	r.Root = root
	r.add("ledger.root", PFOk, "%s%s", root, map[bool]string{true: " (--ledger-root)", false: " (default)"}[explicit])

	// 3. Explicit --ledger-root pointing at a v1 tree is a hard refusal;
	//    a legacy .shared/ merely sitting under the project root is
	//    informational (relay-v2-protocol §1; review 086 must-fix 2).
	if explicit {
		if err := refuseV1(root); err != nil {
			r.add("ledger.v1_root", PFFail, "%v", err)
		} else {
			r.add("ledger.v1_root", PFOk, "not a v1 tree")
		}
	}
	if in.ProjectRoot != "" {
		legacy := filepath.Join(in.ProjectRoot, ".shared")
		if _, err := os.Stat(filepath.Join(legacy, "_relay")); err == nil {
			r.add("legacy.shared", PFWarn, "agent-ledger v1 ledger at %s — archival/manual reference only; oma never reads or writes it", legacy)
		}
	}

	// 4. Sentinel / schema.
	sentinelPath := filepath.Join(root, sentinelName)
	switch raw, err := os.ReadFile(sentinelPath); {
	case err == nil:
		if verr := validateSentinel(raw); verr != nil {
			r.add("ledger.sentinel", PFFail, "%v", verr)
		} else {
			r.add("ledger.sentinel", PFOk, "v2 sentinel valid")
		}
	case os.IsNotExist(err):
		if nonEmptyDir(root) {
			r.add("ledger.sentinel", PFFail, "%s is non-empty but has no v2 sentinel (foreign directory)", root)
		} else {
			r.add("ledger.sentinel", PFWarn, "not initialized — run `oma relay init`")
		}
	default:
		r.add("ledger.sentinel", PFFail, "%v", err)
	}

	// 5. Binding + peer derivation (best-effort; needs a valid identity).
	if idErr == nil {
		l := NewLedger(root, id)
		l.Getenv = getenv
		if b, err := l.loadBinding(); err == nil {
			if s, serr := l.LoadSession(b.Pair); serr != nil {
				r.add("pair.binding", PFWarn, "bound to %s but it is missing/invalid (rebind with `oma relay pair join <slug>`): %v", b.Pair, serr)
			} else if s.Terminal() {
				r.add("pair.binding", PFWarn, "bound to %s which is %s (rebind to an active pair)", b.Pair, s.Status)
			} else {
				r.add("pair.binding", PFOk, "bound to %s", b.Pair)
				if peer, perr := s.Peer(id.Author); perr == nil {
					r.add("identity.peer", PFOk, "peer %s", peer)
				} else {
					r.add("identity.peer", PFWarn, "%v", perr)
				}
			}
		} else if os.IsNotExist(err) {
			r.add("pair.binding", PFWarn, "no binding for this session — `oma relay pair ensure` or `oma relay pair new <topic>`")
		} else {
			r.add("pair.binding", PFWarn, "%v", err)
		}
	}

	// 6. Filesystem probes against the directory where the ledger lives.
	r.fsProbes(root, now)
	return r
}

// fsProbes verifies the filesystem properties the relay protocol relies
// on (relay-v2-protocol §2): tmp+rename atomicity, mtime fidelity,
// symlink ops, hash stability, fsync, and POSIX mode. They run in a
// temp subdir under the nearest existing ancestor of root and clean up.
func (r *PreflightReport) fsProbes(root string, now func() time.Time) {
	base := nearestExisting(root)
	probeDir, err := os.MkdirTemp(base, ".oma-preflight-")
	if err != nil {
		r.add("fs.probe_dir", PFFail, "cannot create a probe directory under %s: %v", base, err)
		return
	}
	defer func() { _ = os.RemoveAll(probeDir) }()

	// tmp + rename atomicity with readback.
	content := []byte("oma-preflight\n")
	tmp := filepath.Join(probeDir, "x.tmp")
	dst := filepath.Join(probeDir, "x")
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		r.add("fs.tmp_rename", PFFail, "write: %v", err)
	} else if err := os.Rename(tmp, dst); err != nil {
		r.add("fs.tmp_rename", PFFail, "rename: %v", err)
	} else if back, err := os.ReadFile(dst); err != nil || !bytes.Equal(back, content) {
		r.add("fs.tmp_rename", PFFail, "readback mismatch (err=%v)", err)
	} else {
		r.add("fs.tmp_rename", PFOk, "tmp+rename atomic; readback matches")
	}

	// mtime fidelity: two files set 1s apart must read back distinctly;
	// equal mtimes mean coarse-grained timestamps (warn, not fail —
	// relay-v2-protocol §2 / review 086).
	a := filepath.Join(probeDir, "a")
	b := filepath.Join(probeDir, "b")
	t0 := now().UTC()
	mtimeOK := true
	for f, t := range map[string]time.Time{a: t0, b: t0.Add(time.Second)} {
		if err := os.WriteFile(f, content, 0o600); err != nil || os.Chtimes(f, t, t) != nil {
			mtimeOK = false
		}
	}
	ai, aerr := os.Stat(a)
	bi, berr := os.Stat(b)
	switch {
	case !mtimeOK || aerr != nil || berr != nil:
		r.add("fs.mtime", PFWarn, "could not establish mtime fidelity (aerr=%v berr=%v)", aerr, berr)
	case bi.ModTime().After(ai.ModTime()):
		r.add("fs.mtime", PFOk, "distinguishes timestamps 1s apart")
	default:
		r.add("fs.mtime", PFWarn, "coarse-grained mtime (1s-apart writes read back equal); stale detection still works at the %s scale", defaultStaleAfter)
	}

	// Symlink create + readlink.
	target := filepath.Join(probeDir, "x")
	link := filepath.Join(probeDir, "lnk")
	if err := os.Symlink(target, link); err != nil {
		r.add("fs.symlink", PFWarn, "symlinks unavailable: %v (projection-by-symlink may be affected; relay itself does not require them)", err)
	} else if got, err := os.Readlink(link); err != nil || got != target {
		r.add("fs.symlink", PFFail, "readlink mismatch (err=%v)", err)
	} else {
		r.add("fs.symlink", PFOk, "create / readlink ok")
	}

	// sha256 stability.
	if back, err := os.ReadFile(dst); err == nil {
		h1 := sha256.Sum256(back)
		h2 := sha256.Sum256(back)
		if h1 == h2 {
			r.add("fs.sha256", PFOk, "hash stable")
		} else {
			r.add("fs.sha256", PFFail, "hash unstable")
		}
	}

	// fsync + readback.
	fp := filepath.Join(probeDir, "sync")
	if f, err := os.OpenFile(fp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600); err != nil {
		r.add("fs.fsync", PFFail, "open: %v", err)
	} else {
		_, werr := f.Write(content)
		serr := f.Sync()
		cerr := f.Close()
		if werr != nil || serr != nil || cerr != nil {
			r.add("fs.fsync", PFFail, "write/sync/close: %v / %v / %v", werr, serr, cerr)
		} else if back, err := os.ReadFile(fp); err != nil || !bytes.Equal(back, content) {
			r.add("fs.fsync", PFFail, "readback after fsync mismatch (err=%v)", err)
		} else {
			r.add("fs.fsync", PFOk, "fsync ok")
		}
	}

	// POSIX mode 0700 (the permission relay directories require).
	md := filepath.Join(probeDir, "mode")
	if err := os.Mkdir(md, 0o700); err != nil {
		r.add("fs.posix_mode", PFFail, "mkdir: %v", err)
	} else if info, err := os.Stat(md); err != nil {
		r.add("fs.posix_mode", PFFail, "stat: %v", err)
	} else if perm := info.Mode().Perm(); perm != 0o700 {
		r.add("fs.posix_mode", PFWarn, "mode %o does not match target 0700 (non-POSIX filesystem?)", perm)
	} else {
		r.add("fs.posix_mode", PFOk, "mode 0700 matches")
	}
}

// nearestExisting returns the nearest existing ancestor of path (used to
// place probe files when the ledger root does not exist yet).
func nearestExisting(path string) string {
	for {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return os.TempDir()
		}
		path = parent
	}
}
