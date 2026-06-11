package checks

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// CommandSet is the registered command surface: every full cobra command
// path mapped to whether it is RUNNABLE (has Run/RunE). Groups like
// "oma relay pair" are present with false. The distinction drives
// validation: trailing tokens after a runnable command are arguments;
// after a group they are invalid leaves (docs/adapter-conformance.md §4).
type CommandSet map[string]bool

// Refcheck scans markdown files under root for `oma ...` references inside
// code spans/blocks and validates each against the command set with
// longest-prefix matching (docs/adapter-conformance.md §4). Zero
// exemptions: a valid prefix with an invalid leaf token fails.
func Refcheck(root string, commands CommandSet) []Finding {
	var findings []Finding
	mdFiles, err := collectMarkdown(root)
	if err != nil {
		return []Finding{{Check: "refcheck", Level: LevelFail, Message: err.Error()}}
	}
	for _, path := range mdFiles {
		raw, err := os.ReadFile(path)
		if err != nil {
			findings = append(findings, Finding{Check: "refcheck", Level: LevelFail, Message: fmt.Sprintf("%s: %v", path, err)})
			continue
		}
		for _, ref := range ExtractOmaRefs(string(raw)) {
			if bad := validateRef(ref, commands); bad != "" {
				findings = append(findings, Finding{
					Check: "refcheck", Level: LevelFail,
					Message: fmt.Sprintf("%s references unknown command %q", path, bad),
				})
			}
		}
	}
	return findings
}

// collectMarkdown lists SKILL.md and references/**.md under root; a missing
// root is fine (nothing installed yet).
func collectMarkdown(root string) ([]string, error) {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}
	var out []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(p, ".md") {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

var fenceRe = regexp.MustCompile("(?s)```.*?```")
var inlineRe = regexp.MustCompile("`[^`\n]+`")

// ExtractOmaRefs pulls candidate `oma <tokens>` command references from
// markdown code blocks and inline code spans. Each ref is the token list
// after (and including) "oma", cut at flags, redirection, pipes,
// semicolons, and line ends (docs/adapter-conformance.md §4).
func ExtractOmaRefs(md string) [][]string {
	var refs [][]string
	var codeChunks []string
	codeChunks = append(codeChunks, fenceRe.FindAllString(md, -1)...)
	// strip fenced blocks before scanning inline spans to avoid doubles
	rest := fenceRe.ReplaceAllString(md, "")
	codeChunks = append(codeChunks, inlineRe.FindAllString(rest, -1)...)

	for _, chunk := range codeChunks {
		chunk = strings.Trim(chunk, "`")
		sc := bufio.NewScanner(strings.NewReader(chunk))
		for sc.Scan() {
			for _, segment := range splitCommands(sc.Text()) {
				toks := strings.Fields(segment)
				for i, tok := range toks {
					if tok != "oma" {
						continue
					}
					ref := []string{"oma"}
					for _, t := range toks[i+1:] {
						// stop only at flags; every other token reaches
						// longest-prefix validation so unknown leaves with
						// digits/underscores cannot slip through
						// (review 038 blocker 2)
						if strings.HasPrefix(t, "-") {
							break
						}
						ref = append(ref, t)
					}
					refs = append(refs, ref)
					break // one ref per segment: rest was consumed or stopped
				}
			}
		}
	}
	return refs
}

// splitCommands cuts a line at shell separators: ; | & && || > < $( )
func splitCommands(line string) []string {
	return regexp.MustCompile(`[;|&<>()]`).Split(line, -1)
}

// validateRef resolves the longest registered prefix. Empty return = valid.
// Rules (docs/adapter-conformance.md §4):
//   - prefix ends at a RUNNABLE command → remaining tokens are arguments, valid
//     (`oma asset install deep-interview`)
//   - prefix ends at a GROUP with tokens left → invalid leaf, fail
//     (`oma relay pair typo`)
//   - ref ends exactly on a group → documented command-group example, valid
func validateRef(ref []string, commands CommandSet) (bad string) {
	if len(ref) == 1 {
		return "" // bare "oma" in prose examples is fine
	}
	deepest := 1 // "oma" itself
	for i := 2; i <= len(ref); i++ {
		if _, known := commands[strings.Join(ref[:i], " ")]; known {
			deepest = i
		} else {
			break
		}
	}
	if deepest == len(ref) {
		return "" // exact command or documented group
	}
	if commands[strings.Join(ref[:deepest], " ")] {
		return "" // runnable command: the rest are arguments
	}
	return strings.Join(ref[:deepest+1], " ")
}
