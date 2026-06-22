package cli

import (
	"os"

	"github.com/sean2077/oh-my-agents/internal/state"
	"github.com/sean2077/oh-my-agents/internal/workflowstate"
)

func workflowScope() workflowstate.Scope {
	return workflowstate.Scope{Session: WorkflowSession(), Getenv: os.Getenv}
}

func scopeStateKey(key string) (string, error) {
	scoped, err := workflowScope().StateKey(key)
	if err != nil {
		return "", Errf(ExitState, "%v", err)
	}
	return scoped, nil
}

func filterSessionEntries(entries []state.Entry) ([]state.Entry, error) {
	filtered, err := workflowScope().FilterEntries(entries)
	if err != nil {
		return nil, Errf(ExitState, "%v", err)
	}
	return filtered, nil
}
