package testenv

import (
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/tests/e2e/internal/evidence"
)

func fullAccessPolicy() map[string]any {
	return map[string]any{"filesystem": map[string]any{"read_scope": "host", "write_scope": "host", "git_metadata_write": false}, "network": "allow", "escalation": "deny"}
}

func (p *Project) AssertFullAccess(snapshot Snapshot) {
	p.Scenario.t.Helper()
	p.Scenario.Must(evidence.FullAccess(snapshot.Domain))
}

func ExecutionSession(e executionmodel.ExecutionRecord) (string, error) { return evidence.Session(e) }
