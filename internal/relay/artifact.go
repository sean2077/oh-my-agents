package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Artifact kinds and statuses (protocol §5).
var (
	Kinds    = []string{"plan", "review", "fix", "note", "question", "decision", "correction", "addendum"}
	Statuses = []string{"ready", "closed", "cancelled", "failed", "timed_out"}
)

// ValidKind / ValidStatus report membership in the §5 sets.
func ValidKind(k string) bool   { return contains(Kinds, k) }
func ValidStatus(s string) bool { return contains(Statuses, s) }

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// Frontmatter is the oma-relay/2 artifact header (protocol §5).
type Frontmatter struct {
	Schema        string
	Seq           int
	Author        string
	Peer          string
	Kind          string
	Status        string
	Created       time.Time
	InReplyTo     *int
	PromptForNext string
	TouchedPaths  []string
	Corrects      *int
}

// Validate enforces the §5 contract on a parsed or about-to-render header.
func (f *Frontmatter) Validate() error {
	if major, ok := schemaMajor(f.Schema, "oma-relay"); !ok || major != 2 {
		return fmt.Errorf("%w: artifact schema %q, want %s", ErrRelay, f.Schema, Schema)
	}
	if f.Seq < 1 || f.Seq > 999 {
		return fmt.Errorf("%w: seq %d out of range 1..999", ErrRelay, f.Seq)
	}
	if !authorRe.MatchString(f.Author) || !authorRe.MatchString(f.Peer) {
		return fmt.Errorf("%w: author/peer %q/%q invalid", ErrRelay, f.Author, f.Peer)
	}
	if f.Author == f.Peer {
		return fmt.Errorf("%w: author equals peer (%s)", ErrRelay, f.Author)
	}
	if !ValidKind(f.Kind) {
		return fmt.Errorf("%w: kind %q not in %v", ErrRelay, f.Kind, Kinds)
	}
	if !ValidStatus(f.Status) {
		return fmt.Errorf("%w: status %q not in %v", ErrRelay, f.Status, Statuses)
	}
	if f.Created.IsZero() {
		return fmt.Errorf("%w: created timestamp missing", ErrRelay)
	}
	for _, p := range f.TouchedPaths {
		clean := filepath.ToSlash(filepath.Clean(p))
		if p == "" || strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("%w: touched path %q must be repo-relative without escapes", ErrRelay, p)
		}
	}
	return nil
}

// ArtifactName is the canonical published filename for a header.
func ArtifactName(seq int, author, kind string) string {
	return fmt.Sprintf("%03d-%s-%s.md", seq, author, kind)
}

var nameParseRe = regexp.MustCompile(`^(\d{3})-([a-z0-9][a-z0-9-]{0,31})-([a-z]+)\.md$`)

// ParseArtifactName splits NNN-<author>-<kind>.md.
func ParseArtifactName(name string) (seq int, author, kind string, ok bool) {
	m := nameParseRe.FindStringSubmatch(name)
	if m == nil {
		return 0, "", "", false
	}
	seq, err := strconv.Atoi(m[1])
	if err != nil || seq < 1 || !ValidKind(m[3]) {
		return 0, "", "", false
	}
	return seq, m[2], m[3], true
}

// Render produces the full artifact bytes: strict YAML frontmatter in a
// fixed key order, then the body. Parse reads back exactly this subset
// and nothing more (fail-closed on anything fancier).
func Render(f *Frontmatter, body string) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "schema: %s\n", f.Schema)
	fmt.Fprintf(&b, "seq: %d\n", f.Seq)
	fmt.Fprintf(&b, "author: %s\n", f.Author)
	fmt.Fprintf(&b, "peer: %s\n", f.Peer)
	fmt.Fprintf(&b, "kind: %s\n", f.Kind)
	fmt.Fprintf(&b, "status: %s\n", f.Status)
	fmt.Fprintf(&b, "created: %s\n", f.Created.UTC().Format(time.RFC3339))
	if f.InReplyTo != nil {
		fmt.Fprintf(&b, "in_reply_to: %d\n", *f.InReplyTo)
	} else {
		b.WriteString("in_reply_to: null\n")
	}
	if f.PromptForNext == "" {
		b.WriteString("prompt_for_next: \"\"\n")
	} else {
		b.WriteString("prompt_for_next: |\n")
		for _, line := range strings.Split(strings.TrimRight(f.PromptForNext, "\n"), "\n") {
			b.WriteString("  " + line + "\n")
		}
	}
	if len(f.TouchedPaths) == 0 {
		b.WriteString("touched_paths: []\n")
	} else {
		b.WriteString("touched_paths:\n")
		for _, p := range f.TouchedPaths {
			b.WriteString("  - " + p + "\n")
		}
	}
	if f.Corrects != nil {
		fmt.Fprintf(&b, "corrects: %d\n", *f.Corrects)
	} else {
		b.WriteString("corrects: null\n")
	}
	b.WriteString("---\n")
	b.WriteString(body)
	return []byte(b.String())
}

// Parse reads artifact bytes rendered by Render (or hand-written within
// the same strict subset). Unknown keys, unknown shapes and a missing
// closing fence fail closed.
func Parse(raw []byte) (*Frontmatter, string, error) {
	lines := strings.Split(string(raw), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != "---" {
		return nil, "", fmt.Errorf("%w: artifact must start with ---", ErrRelay)
	}
	f := &Frontmatter{}
	i := 1
	for ; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimRight(line, " \t") == "---" {
			body := strings.Join(lines[i+1:], "\n")
			if err := f.Validate(); err != nil {
				return nil, "", err
			}
			return f, body, nil
		}
		key, value, found := strings.Cut(line, ":")
		if !found || strings.HasPrefix(line, " ") {
			return nil, "", fmt.Errorf("%w: unexpected frontmatter line %q", ErrRelay, line)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		var err error
		switch key {
		case "schema":
			f.Schema = value
		case "seq":
			f.Seq, err = strconv.Atoi(value)
		case "author":
			f.Author = value
		case "peer":
			f.Peer = value
		case "kind":
			f.Kind = value
		case "status":
			f.Status = value
		case "created":
			f.Created, err = time.Parse(time.RFC3339, value)
		case "in_reply_to":
			f.InReplyTo, err = parseOptInt(value)
		case "corrects":
			f.Corrects, err = parseOptInt(value)
		case "prompt_for_next":
			switch value {
			case "|", "|-":
				var block []string
				for i+1 < len(lines) && (strings.HasPrefix(lines[i+1], "  ") || strings.TrimSpace(lines[i+1]) == "") {
					if strings.TrimRight(lines[i+1], " \t") == "---" {
						break
					}
					block = append(block, strings.TrimPrefix(lines[i+1], "  "))
					i++
				}
				f.PromptForNext = strings.TrimRight(strings.Join(block, "\n"), "\n")
			case `""`, "''", "":
				f.PromptForNext = ""
			default:
				f.PromptForNext = strings.Trim(value, `"'`)
			}
		case "touched_paths":
			if value == "[]" {
				f.TouchedPaths = []string{}
				continue
			}
			if value != "" {
				return nil, "", fmt.Errorf("%w: touched_paths must be [] or a block sequence", ErrRelay)
			}
			for i+1 < len(lines) && strings.HasPrefix(lines[i+1], "  - ") {
				f.TouchedPaths = append(f.TouchedPaths, strings.TrimSpace(strings.TrimPrefix(lines[i+1], "  - ")))
				i++
			}
		default:
			return nil, "", fmt.Errorf("%w: unknown frontmatter key %q (fail-closed)", ErrRelay, key)
		}
		if err != nil {
			return nil, "", fmt.Errorf("%w: frontmatter %s: %v", ErrRelay, key, err)
		}
	}
	return nil, "", fmt.Errorf("%w: frontmatter never closed with ---", ErrRelay)
}

func parseOptInt(v string) (*int, error) {
	if v == "null" || v == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// ReadArtifact loads and verifies one published artifact: the .ready
// sidecar must exist (the only publication criterion) and the .sha256
// sidecar must match the content — anything else is reported corrupt and
// the content is not returned (protocol §7).
func ReadArtifact(path string) (*Frontmatter, string, error) {
	if _, err := os.Stat(path + ".ready"); err != nil {
		return nil, "", fmt.Errorf("%w: %s has no .ready sidecar (unpublished or interrupted)", ErrRelay, filepath.Base(path))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	wantHex, err := os.ReadFile(path + ".sha256")
	if err != nil {
		return nil, "", fmt.Errorf("%w: %s has no .sha256 sidecar", ErrRelay, filepath.Base(path))
	}
	sum := sha256.Sum256(raw)
	if got, want := hex.EncodeToString(sum[:]), strings.TrimSpace(string(wantHex)); got != want {
		return nil, "", fmt.Errorf("%w: %s content does not match .sha256 (corrupt or tampered; content withheld)", ErrRelay, filepath.Base(path))
	}
	return parseAndReturn(raw)
}

func parseAndReturn(raw []byte) (*Frontmatter, string, error) {
	f, body, err := Parse(raw)
	if err != nil {
		return nil, "", err
	}
	return f, body, nil
}

// publishedArtifacts lists formal artifacts in a pair dir (sorted by
// filename => by seq). readyOnly filters to those with .ready sidecars.
func publishedArtifacts(pairDir string, readyOnly bool) ([]string, error) {
	entries, err := os.ReadDir(pairDir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		if _, _, _, ok := ParseArtifactName(ent.Name()); !ok {
			continue
		}
		if readyOnly {
			if _, err := os.Stat(filepath.Join(pairDir, ent.Name()+".ready")); err != nil {
				continue
			}
		}
		names = append(names, ent.Name())
	}
	sort.Strings(names)
	return names, nil
}
