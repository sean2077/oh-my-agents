package relay

import (
	"testing"
	"time"
)

// S3 / DRIFT-1: staleAfter resolves from the config-injected Ledger.StaleAfter
// (relay.stale_after), not a relay-local env re-parse. Zero means default.
func TestStaleAfterUsesConfiguredValue(t *testing.T) {
	if got := (&Ledger{StaleAfter: 5 * time.Minute}).staleAfter(); got != 5*time.Minute {
		t.Fatalf("staleAfter = %v, want configured 5m", got)
	}
	if got := (&Ledger{}).staleAfter(); got != defaultStaleAfter {
		t.Fatalf("staleAfter = %v, want default %v when unset", got, defaultStaleAfter)
	}
}
