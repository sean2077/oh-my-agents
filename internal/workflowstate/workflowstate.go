// Package workflowstate centralizes reusable project-level workflow scoping.
//
// It intentionally does not know about any specific workflow. Callers supply a
// logical id or state key, and the package applies the optional session suffix
// used to let several host sessions share one project .oma tree without
// colliding.
package workflowstate

import (
	"strings"

	"github.com/sean2077/oh-my-agents/internal/session"
	"github.com/sean2077/oh-my-agents/internal/state"
)

// Scope applies the workflow-session setting. Session is the raw flag value:
// an explicit slug or "current".
type Scope struct {
	Session string
	Getenv  func(string) string
}

// Suffix resolves the path-safe session suffix.
func (s Scope) Suffix() (string, error) {
	getenv := s.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	return session.Resolve(s.Session, getenv)
}

// ID scopes a workflow id, such as an interview or ralph id.
func (s Scope) ID(id string) (string, error) {
	suffix, err := s.Suffix()
	if err != nil {
		return "", err
	}
	return session.ScopeName(id, suffix)
}

// StateKey scopes the namespace part of a "namespace/field" state key.
func (s Scope) StateKey(key string) (string, error) {
	ns, field, ok := strings.Cut(key, "/")
	if !ok {
		return key, nil
	}
	ns, err := s.ID(ns)
	if err != nil {
		return "", err
	}
	return ns + "/" + field, nil
}

// FilterEntries keeps only state entries belonging to this session suffix.
func (s Scope) FilterEntries(entries []state.Entry) ([]state.Entry, error) {
	suffix, err := s.Suffix()
	if err != nil {
		return nil, err
	}
	out := make([]state.Entry, 0, len(entries))
	for _, ent := range entries {
		if session.MatchesScope(ent.Namespace, suffix) {
			out = append(out, ent)
		}
	}
	return out, nil
}
