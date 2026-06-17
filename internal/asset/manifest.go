// Package asset implements the content-asset domain: manifest parsing,
// canonical placement, per-agent projection, backup and verification
// (docs/reference/adapter-conformance.md).
package asset

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ManifestSchema is the persisted schema this package writes and the only
// major it reads (docs/reference/schemas.md §1: unknown majors fail closed).
const ManifestSchema = "oma-asset/1"

// Asset types (docs/reference/adapter-conformance.md §1).
const (
	TypeSkill    = "skill"
	TypeSubagent = "subagent"
	TypeHook     = "hook"
	TypePrompt   = "prompt"
)

// Projection targets (docs/reference/adapter-conformance.md §1).
const (
	TargetClaude = "claude"
	TargetCodex  = "codex"
	TargetShared = "shared"
)

// Asset lifecycle status (A7 catalog). An empty status reads as active.
// merged/alias point to a successor via the manifest's canonical field.
const (
	StatusActive     = "active"
	StatusDeprecated = "deprecated"
	StatusMerged     = "merged"
	StatusAlias      = "alias"
)

var (
	// ErrUnknownSchema marks a manifest whose schema major is not supported.
	ErrUnknownSchema = errors.New("unknown manifest schema (fail-closed)")
	// ErrInvalid marks a manifest that violates the oma-asset/1 contract.
	ErrInvalid = errors.New("invalid manifest")
)

// nameRe keeps asset names path-traversal safe by construction.
var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Manifest is the oma-asset/1 document (docs/reference/adapter-conformance.md §1).
// Unknown JSON fields are tolerated on read (minor-additive policy).
type Manifest struct {
	Schema                  string   `json:"schema"`
	Name                    string   `json:"name"`
	Type                    string   `json:"type"`
	Version                 string   `json:"version,omitempty"`
	Targets                 []string `json:"targets"`
	DescriptionBudgetTokens int      `json:"description_budget_tokens,omitempty"`
	Fallback                string   `json:"fallback,omitempty"`
	// A7 catalog lifecycle. Status is active|deprecated|merged|alias (empty
	// = active); Canonical names the successor for merged/alias.
	Status    string `json:"status,omitempty"`
	Canonical string `json:"canonical,omitempty"`
}

// LoadManifest reads and validates a manifest file.
func LoadManifest(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	return ParseManifest(raw)
}

// ParseManifest decodes and validates an oma-asset/1 document.
func ParseManifest(raw []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%w: not valid JSON: %v", ErrInvalid, err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate enforces the oma-asset/1 contract.
func (m *Manifest) Validate() error {
	major, ok := schemaMajor(m.Schema, "oma-asset")
	if !ok || major != 1 {
		return fmt.Errorf("%w: got %q, want %s", ErrUnknownSchema, m.Schema, ManifestSchema)
	}
	if !nameRe.MatchString(m.Name) {
		return fmt.Errorf("%w: name %q (want %s)", ErrInvalid, m.Name, nameRe)
	}
	switch m.Type {
	case TypeSkill, TypeSubagent, TypeHook, TypePrompt:
	default:
		return fmt.Errorf("%w: type %q not in {skill,subagent,hook,prompt}", ErrInvalid, m.Type)
	}
	if len(m.Targets) == 0 {
		return fmt.Errorf("%w: targets must not be empty", ErrInvalid)
	}
	seen := map[string]bool{}
	for _, tgt := range m.Targets {
		switch tgt {
		case TargetClaude, TargetCodex, TargetShared:
		default:
			return fmt.Errorf("%w: target %q not in {claude,codex,shared}", ErrInvalid, tgt)
		}
		if seen[tgt] {
			return fmt.Errorf("%w: duplicate target %q", ErrInvalid, tgt)
		}
		seen[tgt] = true
	}
	// CC-only assets must document the codex degradation path
	// (docs/reference/adapter-conformance.md §1).
	if seen[TargetClaude] && !seen[TargetCodex] && strings.TrimSpace(m.Fallback) == "" {
		return fmt.Errorf("%w: claude-only asset %q requires a codex fallback note", ErrInvalid, m.Name)
	}
	switch m.Status {
	case "", StatusActive, StatusDeprecated, StatusMerged, StatusAlias:
	default:
		return fmt.Errorf("%w: status %q not in {active,deprecated,merged,alias}", ErrInvalid, m.Status)
	}
	if (m.Status == StatusMerged || m.Status == StatusAlias) && strings.TrimSpace(m.Canonical) == "" {
		return fmt.Errorf("%w: status %q requires a canonical successor", ErrInvalid, m.Status)
	}
	if m.Canonical != "" && !nameRe.MatchString(m.Canonical) {
		return fmt.Errorf("%w: canonical %q (want a valid asset name)", ErrInvalid, m.Canonical)
	}
	return nil
}

// StatusOrDefault returns the lifecycle status, defaulting an empty value
// to active.
func (m *Manifest) StatusOrDefault() string {
	if m.Status == "" {
		return StatusActive
	}
	return m.Status
}

// HasTarget reports whether the manifest projects to the given target.
func (m *Manifest) HasTarget(target string) bool {
	for _, tgt := range m.Targets {
		if tgt == target {
			return true
		}
	}
	return false
}

// schemaMajor parses "oma-<domain>/<major>" and returns the major version.
// The major must be a plain ASCII digit sequence with value >= 1 (no signs,
// no leading zeros): persisted-schema readers across registry/state/relay
// reuse this, and a permissive parse here would weaken their fail-closed
// guarantee (B2 review finding 1).
func schemaMajor(schema, wantDomain string) (int, bool) {
	domain, ver, found := strings.Cut(schema, "/")
	if !found || domain != wantDomain || ver == "" || ver[0] == '0' {
		return 0, false
	}
	for i := 0; i < len(ver); i++ {
		if ver[i] < '0' || ver[i] > '9' {
			return 0, false
		}
	}
	major, err := strconv.Atoi(ver)
	if err != nil || major < 1 {
		return 0, false
	}
	return major, true
}
