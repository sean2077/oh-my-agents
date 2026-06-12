package hookcfg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Top-level single-key editing reuses the ordered token tree, so the same
// byte contract holds (foreign tokens preserved verbatim, canonical files
// round-trip byte-identically, atomic tmp+rename, symlink hosts refused).
// Used by relay statusline install to set/remove `~/.claude/settings.json`'s
// `statusLine` key without disturbing the rest of the document.

// GetTopLevel returns the raw value of a top-level key. ok=false when the
// file is absent or the key is missing.
func GetTopLevel(path, key string) (json.RawMessage, bool, error) {
	raw, err := readHost(path)
	if err != nil {
		return nil, false, err
	}
	if raw == nil {
		return nil, false, nil
	}
	root, err := parseRoot(raw)
	if err != nil {
		return nil, false, err
	}
	v, found := root.find(key)
	return v, found, nil
}

// SetTopLevel sets (or replaces) a top-level key. A byte-identical result
// skips the write (no .oma-bak churn).
func SetTopLevel(path, key string, value json.RawMessage) error {
	return editRoot(path, func(root obj) (obj, error) {
		return root.set(key, value), nil
	})
}

// DeleteTopLevel removes a top-level key (no-op if absent).
func DeleteTopLevel(path, key string) error {
	if _, err := os.Lstat(path); err != nil { // missing file: nothing to delete
		return nil
	}
	return editRoot(path, func(root obj) (obj, error) {
		return root.drop(key), nil
	})
}

// parseRoot parses the whole host document as an ordered object (empty
// document → empty obj).
func parseRoot(raw []byte) (obj, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return obj{}, nil
	}
	root, err := parseObj(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: not a JSON object: %v", ErrHost, err)
	}
	return root, nil
}

// editRoot runs read → parse → mutate → render → atomic-write over the
// whole document, with the same fail-closed/atomic guarantees as edit().
func editRoot(path string, mutate func(obj) (obj, error)) error {
	raw, err := readHost(path)
	if err != nil {
		return err
	}
	root, err := parseRoot(raw)
	if err != nil {
		return err
	}
	root, err = mutate(root)
	if err != nil {
		return err
	}
	out := append(renderObj(root, ""), '\n')
	if raw != nil && bytes.Equal(out, raw) {
		return nil
	}
	return writeAtomic(path, raw, out)
}
