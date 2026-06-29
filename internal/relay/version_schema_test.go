package relay

import (
	"testing"

	"github.com/sean2077/oh-my-agents/internal/version"
)

// TestVersionSchemasTrackEmbeddedRelaySchemas pins that the version schema
// registry (version.Schemas) tracks the two embedded-in-artifact relay schemas
// added in S12: the completion receipt (carried in kind:decision frontmatter)
// and the review-evidence block (carried in review bodies). They are persisted
// inside relay artifacts and versioned, so a reader's fail-closed major check
// must find them registered. Asserting against the source constants makes the
// registry fail the build if either schema string ever drifts.
func TestVersionSchemasTrackEmbeddedRelaySchemas(t *testing.T) {
	if got := version.Schemas["relay_receipt"]; got != ReceiptSchema {
		t.Errorf("version.Schemas[relay_receipt] = %q, want %q (ReceiptSchema)", got, ReceiptSchema)
	}
	if got := version.Schemas["relay_evidence"]; got != EvidenceSchema {
		t.Errorf("version.Schemas[relay_evidence] = %q, want %q (EvidenceSchema)", got, EvidenceSchema)
	}
}
