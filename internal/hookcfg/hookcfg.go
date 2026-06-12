// Package hookcfg edits agent host configuration files — claude's
// ~/.claude/settings.json (events under a top-level "hooks" key) and
// codex's ~/.codex/hooks.json (the whole document is the event map) — to
// inject and remove oma-managed hook entries (docs/adapter-conformance.md
// §2, docs/security-contract.md §6).
//
// Ownership is per-entry: every injected entry carries Marker
// ("_oma_asset": "<asset-name>"). Injection replaces that asset's marked
// entries and appends the fresh set at the END of each event array
// (codex /hooks trust is keyed by entry index; existing entries keep
// their positions). Removal filters only matching marked entries and
// drops event keys / the hooks key when oma emptied them.
//
// Byte contract: the document is held as an ordered token tree — foreign
// keys, entries and scalar values are preserved byte-verbatim at the
// token level (numbers and strings are never re-encoded); whitespace is
// normalized to canonical form (2-space indent, trailing newline) on
// write. A canonical-form file therefore round-trips install→remove
// byte-identically (test-pinned); a non-canonical file round-trips
// semantically with its original bytes in the single-generation
// .oma-bak. Writes are atomic (tmp+rename) and happen only after the
// whole document parses: a damaged host file fails closed with zero
// filesystem changes. A host file that is a symlink is refused —
// rename-based writes would replace the link, and following it could
// escape the trusted agent root.
package hookcfg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Marker is the per-entry ownership key stamped on every injected entry.
const Marker = "_oma_asset"

// FragmentSchema is the only fragment schema major this build accepts
// (docs/schemas.md: unknown majors fail closed).
const FragmentSchema = "oma-hook-fragment/1"

// WrapKeySettings is the shape BOTH current hosts use: events nest under
// a top-level "hooks" key (claude settings.json AND codex hooks.json —
// real-host evidence, review 099; codex's sibling "state" trust map is
// preserved verbatim like any foreign top-level key). WrapKeyNone
// (document root IS the event map) remains an editor capability for
// future hosts; no current agent maps to it.
const (
	WrapKeySettings = "hooks"
	WrapKeyNone     = ""
)

var (
	// ErrHost marks a host config file that cannot be safely edited
	// (invalid JSON, symlink, non-object root). Fail-closed: no writes.
	ErrHost = errors.New("host config cannot be safely edited")
	// ErrFragment marks an invalid oma-hook-fragment/1 document.
	ErrFragment = errors.New("invalid hook fragment")
)

// Fragment is a parsed oma-hook-fragment/1 document: per-agent event
// entries in the host agent's native entry shape (oma adds Marker on
// injection; the fragment itself must not carry it).
type Fragment struct {
	Events map[string]map[string][]json.RawMessage // agent → event → entries
}

// LoadFragment reads and validates fragment.json from an asset directory.
func LoadFragment(path string) (*Fragment, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFragment, err)
	}
	return ParseFragment(raw)
}

// ParseFragment decodes and validates an oma-hook-fragment/1 document.
func ParseFragment(raw []byte) (*Fragment, error) {
	var doc struct {
		Schema string                       `json:"schema"`
		Claude map[string][]json.RawMessage `json:"claude"`
		Codex  map[string][]json.RawMessage `json:"codex"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%w: not valid JSON: %v", ErrFragment, err)
	}
	if major, ok := schemaMajor(doc.Schema, "oma-hook-fragment"); !ok || major != 1 {
		return nil, fmt.Errorf("%w: schema %q, want %s (fail-closed)", ErrFragment, doc.Schema, FragmentSchema)
	}
	f := &Fragment{Events: map[string]map[string][]json.RawMessage{}}
	for agent, events := range map[string]map[string][]json.RawMessage{"claude": doc.Claude, "codex": doc.Codex} {
		if events == nil {
			continue
		}
		if len(events) == 0 {
			return nil, fmt.Errorf("%w: agent %q section has no events", ErrFragment, agent)
		}
		for event, entries := range events {
			if strings.TrimSpace(event) == "" {
				return nil, fmt.Errorf("%w: agent %q has an empty event name", ErrFragment, agent)
			}
			if len(entries) == 0 {
				return nil, fmt.Errorf("%w: %s.%s has no entries", ErrFragment, agent, event)
			}
			for i, entry := range entries {
				o, err := parseObj(entry)
				if err != nil {
					return nil, fmt.Errorf("%w: %s.%s[%d] is not a JSON object: %v", ErrFragment, agent, event, i, err)
				}
				if _, found := o.find(Marker); found {
					return nil, fmt.Errorf("%w: %s.%s[%d] carries reserved key %q", ErrFragment, agent, event, i, Marker)
				}
				// Fragment entries are content oma will WRITE into host
				// files: duplicate names at any depth are rejected so oma
				// never authors first-wins/last-wins ambiguity itself
				// (review 046 blocker 1).
				if err := validateUniqueKeys(entry); err != nil {
					return nil, fmt.Errorf("%w: %s.%s[%d]: %v", ErrFragment, agent, event, i, err)
				}
				if len(CommandStrings(entry)) == 0 {
					return nil, fmt.Errorf("%w: %s.%s[%d] has no command string (budget surface requires one, docs/adapter-conformance.md §5)", ErrFragment, agent, event, i)
				}
			}
		}
		f.Events[agent] = events
	}
	if len(f.Events) == 0 {
		return nil, fmt.Errorf("%w: no agent sections (want claude and/or codex)", ErrFragment)
	}
	return f, nil
}

// Inject merges asset's entries into the host file with replace-own
// semantics: existing entries marked for this asset are stripped, the
// fresh set is appended at the end of each event array. Idempotent: a
// byte-identical result skips the write entirely (no .oma-bak churn).
func Inject(path, wrapKey, asset string, events map[string][]json.RawMessage) error {
	return edit(path, wrapKey, func(ev obj) (obj, error) {
		ev, err := stripOwn(ev, asset)
		if err != nil {
			return nil, err
		}
		for _, event := range sortedKeys(events) {
			items, err := eventItems(ev, event)
			if err != nil {
				return nil, err
			}
			for _, entry := range events[event] {
				marked, err := markEntry(entry, asset)
				if err != nil {
					return nil, err
				}
				items = append(items, marked)
			}
			ev = ev.set(event, renderArrRaw(items))
		}
		return ev, nil
	})
}

// Remove strips asset's marked entries from the host file. Event keys
// emptied by the strip are dropped (an event array left empty held only
// oma entries — foreign entries would keep it non-empty), restoring the
// pre-injection document for canonical-form files.
func Remove(path, wrapKey, asset string) error {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil // nothing injected anywhere
	}
	return edit(path, wrapKey, func(ev obj) (obj, error) {
		return stripOwn(ev, asset)
	})
}

// OwnCommands returns the command strings of the entries marked for
// asset in the host file: the asset's actual injected resident surface
// (docs/adapter-conformance.md §5). A missing file yields zero commands.
func OwnCommands(path, wrapKey, asset string) ([]string, error) {
	raw, err := readHost(path)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	_, ev, err := splitDoc(raw, wrapKey)
	if err != nil {
		return nil, err
	}
	var cmds []string
	for _, m := range ev {
		items, err := parseArr(m.raw)
		if err != nil {
			continue // non-array event value: foreign layout, not ours to read
		}
		for _, item := range items {
			owner, owned, err := entryOwner(item)
			if err != nil {
				return nil, fmt.Errorf("event %q: %w", m.key, err)
			}
			if owned && owner == asset {
				cmds = append(cmds, CommandStrings(item)...)
			}
		}
	}
	return cmds, nil
}

// CheckEditable validates that the host file can be safely edited and
// attributed: absent, or a regular non-symlink file whose document,
// event map and entry ownership all parse strictly (duplicate keys
// anywhere on those boundaries fail). It is the shared zero-write
// precheck for remove and rollback (review 046 blocker 2: dry-run and
// real destructive paths must run the same validation).
func CheckEditable(path, wrapKey string) error {
	// An empty asset name matches no marker (validated names are
	// non-empty), so this walks every probe without attributing.
	_, err := OwnCommands(path, wrapKey, "")
	return err
}

// CheckInjectable validates — with zero writes — that
// Inject(path, wrapKey, asset, events) would succeed: everything
// CheckEditable covers, PLUS every planned target event must be absent
// or an array. OwnCommands deliberately tolerates non-array event values
// as foreign layout, but an event the fragment injects INTO is not
// foreign: it must fail the install precheck and dry-run, never the
// post-checkpoint apply (review 048). This mirrors Inject exactly:
// stripOwn's attribution walk plus eventItems on each planned event.
func CheckInjectable(path, wrapKey, asset string, events map[string][]json.RawMessage) error {
	raw, err := readHost(path)
	if err != nil {
		return err
	}
	if raw == nil {
		return nil
	}
	_, ev, err := splitDoc(raw, wrapKey)
	if err != nil {
		return err
	}
	if _, err := stripOwn(ev, asset); err != nil {
		return err
	}
	for _, event := range sortedKeys(events) {
		if _, err := eventItems(ev, event); err != nil {
			return err
		}
	}
	return nil
}

// CommandStrings recursively collects every string value under a
// "command" key inside one JSON value (both hosts nest commands inside
// a matcher group's hooks[] — review 099; recursion also tolerates any
// future flat entry shape).
func CommandStrings(raw json.RawMessage) []string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	var out []string
	var walk func(any)
	walk = func(n any) {
		switch t := n.(type) {
		case map[string]any:
			for k, val := range t {
				if k == "command" {
					if s, ok := val.(string); ok {
						out = append(out, s)
					}
				}
				walk(val)
			}
		case []any:
			for _, item := range t {
				walk(item)
			}
		}
	}
	walk(v)
	return out
}

// edit runs the read→validate→mutate→render→atomic-write cycle on the
// event map of one host file. All parsing happens before any filesystem
// change (review 044 forward note: damaged host JSON must fail closed
// with zero writes).
func edit(path, wrapKey string, mutate func(obj) (obj, error)) error {
	raw, err := readHost(path)
	if err != nil {
		return err
	}
	doc, ev, err := splitDoc(raw, wrapKey)
	if err != nil {
		return err
	}
	ev, err = mutate(ev)
	if err != nil {
		return err
	}
	out := joinDoc(doc, ev, wrapKey)
	if raw != nil && bytes.Equal(out, raw) {
		return nil
	}
	return writeAtomic(path, raw, out)
}

// readHost loads the host file bytes. Missing file → nil (treated as an
// empty document); whitespace-only → empty document; symlink → refused.
func readHost(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %s is a symlink (rename-based writes would replace it; resolve manually)", ErrHost, path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular file", ErrHost, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// splitDoc parses the host document into (top-level obj minus the event
// map, event obj). wrapKey="" means the document root IS the event map,
// and the returned doc obj is nil.
func splitDoc(raw []byte, wrapKey string) (doc obj, events obj, err error) {
	root := obj{}
	if len(bytes.TrimSpace(raw)) > 0 {
		root, err = parseObj(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: not a JSON object: %v", ErrHost, err)
		}
	}
	if wrapKey == WrapKeyNone {
		return nil, root, nil
	}
	if m, found := root.find(wrapKey); found {
		events, err = parseObj(m)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %q value is not a JSON object: %v", ErrHost, wrapKey, err)
		}
		return root, events, nil
	}
	return root, obj{}, nil
}

// joinDoc renders the document back to canonical bytes, re-seating the
// event map (dropping the wrap key when oma emptied it).
func joinDoc(doc, events obj, wrapKey string) []byte {
	if wrapKey == WrapKeyNone {
		return append(renderObj(events, ""), '\n')
	}
	if len(events) == 0 {
		doc = doc.drop(wrapKey)
	} else {
		doc = doc.set(wrapKey, renderObjRaw(events))
	}
	return append(renderObj(doc, ""), '\n')
}

// stripOwn filters out entries marked for asset; events emptied by the
// strip are dropped. Foreign entries and other assets' marked entries
// are byte-untouched. An entry whose ownership cannot be attributed
// unambiguously aborts the whole edit (review 046 blocker 1).
func stripOwn(ev obj, asset string) (obj, error) {
	var out obj
	for _, m := range ev {
		items, err := parseArr(m.raw)
		if err != nil {
			out = append(out, m) // foreign non-array layout: leave verbatim
			continue
		}
		kept := items[:0]
		removed := false
		for _, item := range items {
			owner, owned, err := entryOwner(item)
			if err != nil {
				return nil, fmt.Errorf("event %q: %w", m.key, err)
			}
			if owned && owner == asset {
				removed = true
				continue
			}
			kept = append(kept, item)
		}
		switch {
		case !removed:
			out = append(out, m) // untouched event: keep original bytes
		case len(kept) > 0:
			out = append(out, member{m.key, renderArrRaw(kept)})
		}
	}
	return out, nil
}

// eventItems returns the entry list for one event, creating it when
// absent. A non-array value under an event we must inject into is a
// host layout oma does not understand: fail closed.
func eventItems(ev obj, event string) ([]json.RawMessage, error) {
	m, found := ev.find(event)
	if !found {
		return nil, nil
	}
	items, err := parseArr(m)
	if err != nil {
		return nil, fmt.Errorf("%w: event %q is not an array: %v", ErrHost, event, err)
	}
	return items, nil
}

// markEntry appends the ownership marker to an entry object, preserving
// the entry's own token order.
func markEntry(entry json.RawMessage, asset string) (json.RawMessage, error) {
	o, err := parseObj(entry)
	if err != nil {
		return nil, fmt.Errorf("%w: entry is not a JSON object: %v", ErrFragment, err)
	}
	name, err := json.Marshal(asset)
	if err != nil {
		return nil, err
	}
	o = o.set(Marker, json.RawMessage(name))
	return renderObjRaw(o), nil
}

// entryOwner probes one entry for the ownership marker via the same
// ordered parser used everywhere else — never json.Unmarshal, whose
// last-wins duplicate resolution would disagree with the tree's
// first-wins view (review 046 blocker 1). Non-object elements cannot
// carry a marker and are reported as unowned; an OBJECT entry that fails
// strict parsing (duplicate keys) has ambiguous ownership and is a hard
// error.
func entryOwner(entry json.RawMessage) (owner string, owned bool, err error) {
	trimmed := bytes.TrimSpace(entry)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return "", false, nil // scalar/array element: foreign, unowned
	}
	o, err := parseObj(entry)
	if err != nil {
		return "", false, fmt.Errorf("%w: hook entry unsafe to attribute: %v", ErrHost, err)
	}
	raw, found := o.find(Marker)
	if !found {
		return "", false, nil
	}
	var name string
	if err := json.Unmarshal(raw, &name); err != nil {
		return "", false, fmt.Errorf("%w: %s marker is not a string: %v", ErrHost, Marker, err)
	}
	return name, true, nil
}

// writeAtomic writes out via tmp+rename (0600) with a single-generation
// .oma-bak of the previous bytes (suffix distinct from oma's own .bak
// files: host files live in foreign directories and the user may keep
// their own .bak there).
func writeAtomic(path string, prev, out []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if prev != nil {
		if err := os.WriteFile(path+".oma-bak", prev, 0o600); err != nil {
			return fmt.Errorf("write host config backup: %w", err)
		}
	}
	tmp := path + ".oma-tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("write host config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// validateUniqueKeys walks one JSON value and rejects duplicate object
// member names at ANY depth (decoded comparison, same rule as parseObj).
func validateUniqueKeys(raw json.RawMessage) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	type frame struct {
		isObj     bool
		expectKey bool
		seen      map[string]bool
	}
	var stack []*frame
	top := func() *frame {
		if len(stack) == 0 {
			return nil
		}
		return stack[len(stack)-1]
	}
	valueDone := func() {
		if f := top(); f != nil && f.isObj {
			f.expectKey = true
		}
	}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{':
				stack = append(stack, &frame{isObj: true, expectKey: true, seen: map[string]bool{}})
			case '[':
				stack = append(stack, &frame{isObj: false})
			default: // '}' or ']'
				stack = stack[:len(stack)-1]
				valueDone()
			}
		case string:
			if f := top(); f != nil && f.isObj && f.expectKey {
				if f.seen[t] {
					return fmt.Errorf("duplicate object key %q", t)
				}
				f.seen[t] = true
				f.expectKey = false
			} else {
				valueDone()
			}
		default:
			valueDone()
		}
	}
}

func sortedKeys(m map[string][]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := range keys {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

// schemaMajor parses "oma-<domain>/<major>" with the strict digit-only
// rule shared by every persisted-schema reader (no signs, no leading
// zeros — B2 review finding 1; per-package copy by convention).
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
