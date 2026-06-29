package cli

import (
	"strings"
	"testing"
)

// TEST-6: asset-name ExitUsage. asset.go:39-46 requireValidNames rejects
// invalid asset-name arguments with Errf(ExitUsage, "invalid asset name %q …").
// It runs FIRST in the install RunE (asset.go:99), before any engine or network
// work, so no fixture is needed; OMA_HOME is set to a temp dir only for hygiene.

// TestAssetInstallInvalidNameExitsUsage pins asset.go:42: clearly-invalid asset
// names must exit ExitUsage with "invalid asset name".
//
// fail-before rationale: asset.ValidName rejects uppercase, spaces, and
// punctuation; requireValidNames (asset.go:39-46) turns that into
// Errf(ExitUsage, "invalid asset name …"). Drop the requireValidNames(args)
// call at asset.go:99, or weaken ValidName, and these inputs would fall through
// to the engine/source resolution and exit with a different (non-Usage) code,
// failing the code==ExitUsage assertion.
func TestAssetInstallInvalidNameExitsUsage(t *testing.T) {
	t.Setenv("OMA_HOME", t.TempDir())

	cases := []struct {
		name string
		arg  string
	}{
		{"punctuation", "BadName!"},
		{"uppercase", "MixedCase"},
		{"space", "has space"},
		{"underscore", "under_score"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out := runOma(t, "asset", "install", tc.arg)
			if code != ExitUsage {
				t.Fatalf("install %q exit %d, want ExitUsage(%d): %s", tc.arg, code, ExitUsage, out)
			}
			if !strings.Contains(out, "invalid asset name") {
				t.Fatalf("install %q output missing %q: %s", tc.arg, "invalid asset name", out)
			}
		})
	}
}

// TestAssetInstallValidNamePassesNameCheck pins that a well-formed name does NOT
// trip the requireValidNames usage error. It fails later for a different reason
// (a "dev" build has no release to pin without --from/--ref ⇒ ExitState), which
// is exactly the point: name validation let it through.
//
// fail-before rationale: a valid lowercase/dash name satisfies asset.ValidName,
// so requireValidNames (asset.go:39-46) returns nil and the install proceeds to
// source resolution. If requireValidNames wrongly rejected valid names, this
// would see ExitUsage + "invalid asset name" and fail. The non-ExitUsage exit
// here confirms control reached past the name gate.
func TestAssetInstallValidNamePassesNameCheck(t *testing.T) {
	t.Setenv("OMA_HOME", t.TempDir())

	code, out := runOma(t, "asset", "install", "deep-interview")
	if code == ExitUsage {
		t.Fatalf("valid name unexpectedly hit a usage error (exit %d): %s", code, out)
	}
	if strings.Contains(out, "invalid asset name") {
		t.Fatalf("valid name wrongly rejected by name validation: %s", out)
	}
}
