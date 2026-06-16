package cli

import "testing"

func TestFuzzyStartAdvisory(t *testing.T) {
	vague := []string{"make it better", "improve the thing", "fix bugs", "clean up"}
	for _, g := range vague {
		if fuzzyStartAdvisory(g) == "" {
			t.Errorf("%q should be flagged vague", g)
		}
	}
	concrete := []string{
		"go test ./... passes",
		"fix the panic in internal/relay/hook.go",
		"resolve issue #42",
		"make handleRequest return early on nil",
		"rename the user_id column everywhere it appears",
		"a long enough goal that clearly describes exactly what done means across plenty of explicit descriptive words right here today",
	}
	for _, g := range concrete {
		if adv := fuzzyStartAdvisory(g); adv != "" {
			t.Errorf("%q should NOT be flagged: %s", g, adv)
		}
	}
}
