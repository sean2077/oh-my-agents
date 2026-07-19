package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrackedShellSourcesHavePortableLintContract(t *testing.T) {
	repoRoot := filepath.Join("..", "..")

	attributes := mustReadRepositoryFile(t, filepath.Join(repoRoot, ".gitattributes"))
	if !hasExactLine(attributes, "*.sh text eol=lf") {
		t.Fatal(".gitattributes must pin every tracked .sh source to LF")
	}

	workflow := mustReadRepositoryFile(t, filepath.Join(repoRoot, ".github", "workflows", "build.yml"))
	for _, required := range []string{
		"git ls-files -z -- '*.sh' tools/git-hooks/pre-commit",
		"xargs -0 shellcheck",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("build workflow must ShellCheck the tracked shell surface using %q", required)
		}
	}

	makefile := mustReadRepositoryFile(t, filepath.Join(repoRoot, "Makefile"))
	if !strings.Contains(makefile, "\t\"$(BASH)\" tools/release/build-release.sh \"$(VERSION)\"") {
		t.Fatal("Makefile release target must invoke build-release.sh through the resolved Git Bash path")
	}

	statusline := mustReadRepositoryFile(t, filepath.Join(repoRoot, "docs", "examples", "statusline-command.sh"))
	if !strings.Contains(statusline, `cd "$cwd" 2>/dev/null || exit 0`) {
		t.Fatal("statusline example must omit the oma segment when its requested cwd is unavailable")
	}
	if strings.Contains(statusline, "timeout 1") {
		t.Fatal("statusline example must rely on oma's portable internal deadline, not GNU timeout")
	}
	for _, required := range []string{
		`jq_bin=$(command -v jq 2>/dev/null) || exit 0`,
		`date -r "$1" "+$2"`,
		`while IFS= read -r -d '' value; do`,
		`| .[] + "\u0000"`,
		`[ "${#fields[@]}" -eq 15 ] || exit 0`,
	} {
		if !strings.Contains(statusline, required) {
			t.Fatalf("statusline example is missing portable dependency/date guard %q", required)
		}
	}
	if got := strings.Count(statusline, `"$jq_bin"`); got != 1 {
		t.Fatalf("statusline example invokes jq %d times per refresh, want one host-input parse", got)
	}
	if got := strings.Count(statusline, `"$oma_bin" statusline`); got != 1 || !strings.Contains(statusline, `"$oma_bin" statusline --active-only`) {
		t.Fatalf("statusline example must use one active-only native oma render; got %d statusline calls", got)
	}
}

func mustReadRepositoryFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func hasExactLine(content, want string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		if line == want {
			return true
		}
	}
	return false
}
