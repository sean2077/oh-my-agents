package cli

import (
	"os"
	"strings"

	workflowsession "github.com/sean2077/oh-my-agents/internal/session"
	"github.com/sean2077/oh-my-agents/internal/state"
)

func sessionSuffix() (string, error) {
	suffix, err := workflowsession.Resolve(WorkflowSession(), os.Getenv)
	if err != nil {
		return "", Errf(ExitState, "%v", err)
	}
	return suffix, nil
}

func scopeWorkflowID(id string) (string, error) {
	suffix, err := sessionSuffix()
	if err != nil {
		return "", err
	}
	return workflowsession.ScopeName(id, suffix)
}

func scopeStateKey(key string) (string, error) {
	ns, field, ok := strings.Cut(key, "/")
	if !ok {
		return key, nil
	}
	suffix, err := sessionSuffix()
	if err != nil {
		return "", err
	}
	ns, err = workflowsession.ScopeName(ns, suffix)
	if err != nil {
		return "", Errf(ExitState, "%v", err)
	}
	return ns + "/" + field, nil
}

func filterSessionEntries(entries []state.Entry) ([]state.Entry, error) {
	suffix, err := sessionSuffix()
	if err != nil {
		return nil, err
	}
	if suffix == "" {
		return entries, nil
	}
	out := entries[:0]
	for _, ent := range entries {
		if ent.Namespace == suffix || strings.HasSuffix(ent.Namespace, "-"+suffix) {
			out = append(out, ent)
		}
	}
	return out, nil
}
