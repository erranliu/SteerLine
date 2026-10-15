package testenv

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/erranliu/SteerLine/tests/e2e/internal/evidence"
	"github.com/erranliu/SteerLine/tests/e2e/internal/native"
)

func LiveCoreRecovery(s *Scenario, chain string) {
	p := s.NewLiveProject("live-core-recovery", chain)
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	original := p.WaitRecovery("native-implementation-waiting", func(snap Snapshot) bool {
		if len(snap.Domain.Executions) != 1 {
			return false
		}
		e := snap.Domain.Executions[0]
		if e.CurrentTurnID == "" || e.CurrentTurnID == e.LastStoppedTurnID || e.Workspace.Worktree == "" {
			return false
		}
		if chain == "orca-terminal-local" && !evidence.OrcaOriginalWorkingObserved(e) {
			return false
		}
		_, err := os.Stat(filepath.Join(e.Workspace.Worktree, ".e2e-recovery-started.txt"))
		return err == nil
	})
	e := original.Domain.Executions[0]
	if chain == "local-codex" {
		p.DisconnectWorker(original)
		p.StopService() // Also proves/stops the remaining owned writer tree.
	} else {
		p.StopService() // Observation ends; the original terminal continues its input.
	}
	s.Must(os.WriteFile(filepath.Join(e.Workspace.Worktree, ".e2e-recovery-release.txt"), []byte("release original input\n"), 0600))
	if chain == "orca-terminal-local" {
		turn, err := evidence.OrcaOriginalTurn(e)
		s.Must(err)
		s.Wait("original terminal completed while center offline", func() (bool, error) {
			return native.OrcaOriginalTurnEnded(s.Context, e.Process.ExecutionHost, *e.Process, turn.Native)
		})
	}
	p.StartService()
	open := p.FinishRecoveredFlow()
	current := open.Domain.Executions[0]
	if current.SessionID != e.SessionID || current.ExecutionID != e.ExecutionID || current.Workspace.Worktree != e.Workspace.Worktree {
		s.t.Fatal("real recovery replaced original execution/session/workspace")
	}
	if chain == "local-codex" {
		if len(open.Domain.Attempts) != 2 || current.CurrentAttemptID == e.CurrentAttemptID {
			s.t.Fatal("dead app-server did not recover into a new Attempt")
		}
		usage, err := p.NativeUsage(open, "recovered-publish")
		s.Must(err)
		if usage.Session != e.SessionID {
			s.t.Fatal("usage belongs to a replacement thread")
		}
		s.usageVerified = true
	} else {
		if len(open.Domain.Attempts) != 1 || current.Process == nil || *current.Process != *e.Process {
			s.t.Fatal("real terminal was replaced")
		}
		s.orcaUsageUnavailable = true
		files, err := os.ReadDir(filepath.Join(s.Output, p.Name+"-orca"))
		s.Must(err)
		counts := map[string]int{}
		for _, file := range files {
			raw, err := os.ReadFile(filepath.Join(s.Output, p.Name+"-orca", file.Name()))
			s.Must(err)
			var call struct {
				Args []string `json:"args"`
			}
			s.Must(json.Unmarshal(raw, &call))
			if len(call.Args) >= 2 && call.Args[0] == "terminal" && call.Args[1] == "send" {
				for i, arg := range call.Args {
					if arg == "--retry-request" && i+1 < len(call.Args) {
						counts[call.Args[i+1]]++
					}
				}
			}
		}
		if len(counts) != 3 {
			s.t.Fatal("real terminal did not use three distinct authorized inputs")
		}
		for _, count := range counts {
			if count != 1 {
				s.t.Fatal("real original terminal input resent")
			}
		}
	}
	results := open.Domain.WorkflowRuns[0].StageResults
	if len(results) != 3 || results[0].CommandID == results[1].CommandID || results[1].CommandID == results[2].CommandID || !results[1].FinalVerified {
		s.t.Fatal("real managed implementation/report/publish sources missing")
	}
	if got := p.git(current.Workspace.Worktree, "show", open.Helper.PR.HeadRevision+":result.txt"); got != "real model delivery" {
		s.t.Fatalf("real recovered artifact=%q", got)
	}
	p.AssertFixedNativeLaunch(open)
	p.Merge()
	done := p.WaitPhase("completed")
	p.AssertOneDelivery(done)
	proof := done.Helper.PR
	p.RealPRProof = &proof
	if !proof.Merged || proof.Repository != "erranliu/SteerlineTest" || proof.HeadBranch != current.Workspace.Branch {
		s.t.Fatal("real merged candidate identity missing")
	}
	p.RecordRecoveryBranch("real-core-recovery", done, map[string]any{"chain": chain, "original_session": e.SessionID, "original_turn": e.CurrentTurnID})
	s.Must(s.FinishBody())
	p.StopService()
}
