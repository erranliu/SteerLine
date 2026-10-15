package testenv

import (
	"os"
	"path/filepath"
)

func LiveCoreRecovery(s *Scenario, chain string) {
	p := s.NewLiveProject("live-core-recovery", chain)
	p.CreateTasks()
	p.StartService()
	p.Claim("a")
	original := p.WaitRecovery("native-implementation-waiting", func(snap Snapshot) bool {
		if len(snap.Runner.Candidates) != 1 || snap.Runner.Candidates[0].Projection == nil {
			return false
		}
		e := snap.Runner.Candidates[0].Projection.Execution
		if !e.TurnStarted || e.Workspace == "" {
			return false
		}
		_, err := os.Stat(filepath.Join(e.Workspace, ".e2e-recovery-started.txt"))
		return err == nil
	})
	first := original.Runner.Candidates[0]
	e := first.Projection.Execution
	p.DisconnectWorker(original)
	p.StopService()
	s.Must(os.WriteFile(filepath.Join(e.Workspace, ".e2e-recovery-release.txt"), []byte("release original input\n"), 0600))
	p.StartService()
	open := p.FinishRecoveredFlow()
	last := open.Runner.Candidates[0]
	x := last.Projection.Execution
	if first.ID != last.ID || first.RunID != last.RunID || e.SessionID != x.SessionID || e.Workspace != x.Workspace || e.AttemptID == x.AttemptID {
		s.t.Fatal("real recovery lost original Run/thread/workspace or did not advance Attempt")
	}
	p.AssertRunnerBinding(open)
	p.AssertStagedFlow(open)
	usage, err := p.NativeUsage(open, "recovered-publish")
	s.Must(err)
	if usage.Session != e.SessionID {
		s.t.Fatal("native usage belongs to a replacement session")
	}
	s.usageVerified = true
	if got := p.git(x.Workspace, "show", open.Helper.PR.HeadRevision+":result.txt"); got != "real model delivery" {
		s.t.Fatal("real recovered artifact differs", got)
	}
	p.Merge()
	done := p.WaitPhase("completed")
	p.AssertOneDelivery(done)
	proof := done.Helper.PR
	p.RealPRProof = &proof
	if !proof.Merged || proof.Repository != "erranliu/SteerlineTest" || proof.HeadBranch != x.Branch {
		s.t.Fatal("original merged native candidate identity missing")
	}
	p.RecordRecoveryBranch("real-core-recovery", done, map[string]any{"chain": chain, "original_session": e.SessionID, "original_turn": e.TurnID})
	s.Must(s.finishBody())
	p.StopService()
}
