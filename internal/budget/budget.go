// Package budget implements the resident-context measurement gate
// (docs/adapter-conformance.md §5): a deterministic, pinned approximation
// of the always-loaded prompt surface created by installed assets.
package budget

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sean2077/oh-my-agents/internal/asset"
)

// AlgoVersion pins the token approximation; it is reported in every output
// so measurements are comparable across builds (version.Algorithms).
const AlgoVersion = "approx-b4/1"

// ErrBudget marks measurement errors (unreadable assets, unknown profile).
var ErrBudget = errors.New("budget measurement failed")

// Tokens approximates token count as ceil(utf8_bytes/4) — approx-b4/1.
func Tokens(s string) int { return (len(s) + 3) / 4 }

// Profiles maps profile names to the asset sets they measure. core4 is the
// release gate set (docs/adapter-conformance.md §5); "all" measures every
// installed asset (dogfood convenience).
var Profiles = map[string][]string{
	"core4": {"deep-interview", "autopilot", "ralph", "pair-delivery"},
	"all":   nil,
}

// Item is one measured resident-surface field.
type Item struct {
	Asset  string `json:"asset"`
	Field  string `json:"field"`
	Tokens int    `json:"tokens"`
}

// Report is a full measurement.
type Report struct {
	Algo    string   `json:"algo"`
	Agent   string   `json:"agent"`
	Profile string   `json:"profile"`
	Items   []Item   `json:"items"`
	Total   int      `json:"total"`
	Max     int      `json:"max"`
	Missing []string `json:"missing,omitempty"` // profile members not installed yet
	Notes   []string `json:"notes,omitempty"`
}

// Measure computes the resident surface for one agent and profile.
// Counted per docs/adapter-conformance.md §5 (claude profile): skill
// frontmatter name+description; subagent name+description+whenToUse; hook
// command strings (lands with B4b injection). Only assets actually
// projected to the agent contribute.
func Measure(eng *asset.Engine, agent, profile string, max int) (*Report, error) {
	// An unknown agent must fail closed: with a typo every projectsTo check
	// is false and the gate would pass on a zero count (review 042 blocker).
	if agent != "claude" && agent != "codex" {
		return nil, fmt.Errorf("%w: unknown agent %q (want claude|codex)", ErrBudget, agent)
	}
	members, ok := Profiles[profile]
	if !ok {
		return nil, fmt.Errorf("%w: unknown profile %q (want core4|all)", ErrBudget, profile)
	}
	entries, err := eng.List()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBudget, err)
	}
	byName := map[string]*asset.Entry{}
	for i := range entries {
		byName[entries[i].Name] = &entries[i]
	}

	rep := &Report{Algo: AlgoVersion, Agent: agent, Profile: profile, Max: max}
	if members == nil { // "all"
		for i := range entries {
			members = append(members, entries[i].Name)
		}
	}
	for _, name := range members {
		e, installed := byName[name]
		if !installed {
			rep.Missing = append(rep.Missing, name)
			continue
		}
		if !projectsTo(e, agent) {
			continue
		}
		items, notes, err := residentSurface(e)
		if err != nil {
			return nil, err
		}
		rep.Items = append(rep.Items, items...)
		rep.Notes = append(rep.Notes, notes...)
	}
	for _, it := range rep.Items {
		rep.Total += it.Tokens
	}
	return rep, nil
}

func projectsTo(e *asset.Entry, agent string) bool {
	for _, pr := range e.Projections {
		if pr.Agent == agent {
			return true
		}
	}
	return false
}

// residentSurface extracts the always-loaded fields for one entry.
func residentSurface(e *asset.Entry) ([]Item, []string, error) {
	switch e.Type {
	case asset.TypeSkill:
		fm, err := ReadFrontmatterFile(e.CanonicalPath + "/SKILL.md")
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s: %v", ErrBudget, e.Name, err)
		}
		items, err := requiredFieldItems(e.Name, fm, "name", "description")
		return items, nil, err
	case asset.TypeSubagent:
		fm, err := ReadFrontmatterFile(e.CanonicalPath)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s: %v", ErrBudget, e.Name, err)
		}
		items, err := requiredFieldItems(e.Name, fm, "name", "description", "whenToUse")
		return items, nil, err
	case asset.TypeHook:
		return nil, []string{fmt.Sprintf("%s: hook command surface counts after B4b injection lands", e.Name)}, nil
	default: // prompts are slash-invoked, not resident
		return nil, nil, nil
	}
}

// requiredFieldItems counts the A4-defined resident fields; a missing or
// blank counted field fails closed — an undercounted gate is worse than a
// loud one (review 042 major).
func requiredFieldItems(assetName string, fm map[string]string, fields ...string) ([]Item, error) {
	var items []Item
	for _, f := range fields {
		v, ok := fm[f]
		if !ok || strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("%w: %s: frontmatter field %q is required for budget measurement (A4 §5)", ErrBudget, assetName, f)
		}
		items = append(items, Item{Asset: assetName, Field: f, Tokens: Tokens(v)})
	}
	return items, nil
}
