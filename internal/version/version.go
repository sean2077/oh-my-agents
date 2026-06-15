// Package version holds build metadata, the schema version registry and
// pinned algorithm versions.
package version

// Set via -ldflags at release build time; defaults identify dev builds.
var (
	Version = "dev"
	Commit  = "none"
)

// Schemas registers every persisted-data schema this binary ships
// (docs/schemas.md, format oma-<domain>/<major>). Readers fail closed on
// unknown majors. Schema-only: algorithm versions live in Algorithms.
var Schemas = map[string]string{
	"registry":         "oma-registry/1",
	"state":            "oma-state/1",
	"relay":            "oma-relay/2",
	"relay_binding":    "oma-relay-binding/1",
	"relay_preflight":  "oma-relay-preflight/1",
	"interview":        "oma-interview/1",
	"interview_scores": "oma-interview-scores/1",
	"ralph":            "oma-ralph/1",
	"asset":            "oma-asset/1",
	"config":           "oma-config/1",
}

// Algorithms registers pinned algorithm versions that affect reproducible
// outputs (docs/adapter-conformance.md §5).
var Algorithms = map[string]string{
	"budget": "approx-b4/1",
}
