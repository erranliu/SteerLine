//go:build e2e

package recovery

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/testenv"
)

func TestOrcaStartupSlow(t *testing.T)       { runOrcaStartupScenario(t, "slow") }
func TestOrcaStartupUnobserved(t *testing.T) { runOrcaStartupScenario(t, "unobserved") }
func TestOrcaStartupLost(t *testing.T)       { runOrcaStartupScenario(t, "lost") }
func TestOrcaStartupFailed(t *testing.T)     { runOrcaStartupScenario(t, "failed") }

func runOrcaStartupScenario(t *testing.T, mode string) {
	s := testenv.NewScenario(t, "recovery.orca-startup."+mode)
	p := s.NewProject(mode)
	statePath := p.UseControlledOrcaStartup(mode)
	p.CreateTasks()
	p.StartService()
	if mode == "failed" {
		p.ClaimExpectedStatus("a", "failed")
	} else {
		p.Claim("a")
	}
	before := p.WaitPhase("waiting_decision")
	p.Record("startup", before)
	state, err := controlled.ReadControlledOrcaStartup(statePath)
	s.Must(err)
	e := before.Domain.Executions[0]
	startStatus := ""
	for _, operation := range before.Domain.Operations {
		if operation.Kind == "start_worker_turn" {
			startStatus = operation.Status
		}
	}
	if state.Dispatches != 1 || len(before.Domain.Attempts) != 1 || e.PendingCommandID != "" || e.PendingTurnID != "" {
		t.Fatalf("%s did not settle its original startup: %+v / %+v", mode, state, e)
	}
	if mode == "failed" {
		if startStatus != "rejected" || !state.Stopped || !state.Released || !e.RecoveryRequired {
			t.Fatalf("released startup remained unknown: %s / %+v / %+v", startStatus, state, e)
		}
		for _, decision := range before.Domain.Decisions {
			if decision.Status == "pending" {
				var request struct {
					Options []json.RawMessage `json:"options"`
				}
				s.Must(json.Unmarshal(decision.Request, &request))
				if len(request.Options) != 0 {
					t.Fatal("released startup offered automatic continuation")
				}
			}
		}
	} else if e.Process == nil || e.LastStoppedTurnID == "" || startStatus == "outcome_unknown" || state.Stopped || state.Released {
		t.Fatalf("%s lost or prematurely closed accepted input: %s / %+v / %+v", mode, startStatus, state, e)
	}
	p.RestartService()
	after := p.WaitPhase("waiting_decision")
	p.Record("after-restart", after)
	s.Wait("restarted service observed original execution", func() (bool, error) {
		next, err := p.Snapshot()
		if err != nil {
			return false, err
		}
		if next.Domain.Executions[0].RecoveryVersion <= e.RecoveryVersion {
			return false, nil
		}
		after = next
		return true, nil
	})
	state, err = controlled.ReadControlledOrcaStartup(statePath)
	s.Must(err)
	if state.Dispatches != 1 || len(after.Domain.Attempts) != 1 || after.Domain.Executions[0].ExecutionID != e.ExecutionID || after.Domain.WorkflowRuns[0].WorkflowRunID != e.WorkflowRunID || after.Page.DatabaseID != before.Page.DatabaseID {
		t.Fatalf("%s restart duplicated or replaced startup: %+v", mode, state)
	}
	p.Record("reconciled", after)
	p.StopService()
}
