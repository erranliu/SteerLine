package testenv

import (
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/tests/e2e/internal/evidence"
)

func (p *Project) AssertFixedNativeLaunch(snapshot Snapshot) {
	p.Scenario.t.Helper()
	p.Scenario.Must(evidence.FixedNativeLaunch(snapshot.Domain))
}

func ExecutionSession(e executionmodel.ExecutionRecord) (string, error) { return evidence.Session(e) }
