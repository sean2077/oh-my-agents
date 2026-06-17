// Package checks implements oma doctor's check registry
// (docs/reference/command-tree.md §4): each check inspects one aspect of the
// installation and reports findings at ok/warn/fail levels.
package checks

// Levels order by severity; doctor exit codes derive from the worst level
// (ok→0, warn→1, fail→4 per docs/reference/command-tree.md §1).
const (
	LevelOK   = "ok"
	LevelWarn = "warn"
	LevelFail = "fail"
)

// Finding is one check observation.
type Finding struct {
	Check   string `json:"check"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// Check inspects one aspect and returns findings (empty = silently ok).
type Check struct {
	Name string
	Run  func() []Finding
}

// Result aggregates a doctor run.
type Result struct {
	Findings []Finding `json:"findings"`
	Worst    string    `json:"worst"`
}

// RunAll executes all checks and aggregates the worst level. A check that
// returns no findings contributes an explicit ok finding so the report
// shows what was examined.
func RunAll(cks []Check) Result {
	res := Result{Worst: LevelOK}
	for _, c := range cks {
		fs := c.Run()
		if len(fs) == 0 {
			fs = []Finding{{Check: c.Name, Level: LevelOK, Message: "ok"}}
		}
		for _, f := range fs {
			if f.Check == "" {
				f.Check = c.Name
			}
			res.Findings = append(res.Findings, f)
			if worse(f.Level, res.Worst) {
				res.Worst = f.Level
			}
		}
	}
	return res
}

func worse(a, b string) bool {
	rank := map[string]int{LevelOK: 0, LevelWarn: 1, LevelFail: 2}
	return rank[a] > rank[b]
}
