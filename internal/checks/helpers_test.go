package checks

import (
	"testing"
	"time"

	"github.com/sean2077/oh-my-agents/internal/asset"
)

func newEngine(t *testing.T, home string) *asset.Engine {
	t.Helper()
	e := asset.NewEngine(home)
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	n := 0
	e.Now = func() time.Time { n++; return base.Add(time.Duration(n) * time.Second) }
	return e
}

func installOptions() asset.Options { return asset.Options{Agents: []string{"claude"}} }
