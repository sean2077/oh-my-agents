package budget

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ReadFrontmatterFile parses the YAML frontmatter block of a markdown file
// into a flat key→value map. Deliberately minimal (no YAML dependency):
// top-level `key: value` pairs with indented continuation lines folded by
// a space. oma's own asset-writing rules keep frontmatter in this subset
// (single-line or folded descriptions); anything fancier fails loudly.
func ReadFrontmatterFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !sc.Scan() || strings.TrimRight(sc.Text(), " \t") != "---" {
		return nil, fmt.Errorf("no frontmatter block (file must start with ---)")
	}
	fm := map[string]string{}
	var lastKey string
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimRight(line, " \t") == "---" {
			return fm, nil
		}
		switch {
		case strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t"):
			// continuation of a folded value
			if lastKey == "" {
				return nil, fmt.Errorf("frontmatter continuation line without a key: %q", line)
			}
			fm[lastKey] = strings.TrimSpace(fm[lastKey] + " " + strings.TrimSpace(line))
		default:
			key, value, ok := strings.Cut(line, ":")
			if !ok || strings.TrimSpace(key) == "" {
				return nil, fmt.Errorf("frontmatter line is not key: value: %q", line)
			}
			lastKey = strings.TrimSpace(key)
			v := strings.TrimSpace(value)
			// fold the common block-scalar markers into "value follows"
			if v == "|" || v == ">" || v == ">-" || v == "|-" {
				v = ""
			}
			fm[lastKey] = strings.Trim(v, `"'`)
		}
	}
	return nil, fmt.Errorf("frontmatter block never closed with ---")
}
