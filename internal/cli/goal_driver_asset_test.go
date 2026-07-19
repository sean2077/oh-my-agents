package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestHostNativeGoalDriverUsesObservedCapabilityNotVersion(t *testing.T) {
	minimumVersion := regexp.MustCompile(`(?i)(claude code|codex)[^\n]{0,12}[≥><=]+\s*v?\d`)
	for _, name := range []string{"ralph", "autopilot"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "assets", "skills", name, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		if minimumVersion.MatchString(text) {
			t.Fatalf("%s infers a host-native /goal capability from a minimum version", name)
		}
		if !strings.Contains(text, "current interactive surface") || !strings.Contains(text, "exposes") {
			t.Fatalf("%s must gate /goal on capability exposed by the current interactive surface", name)
		}
	}
}
