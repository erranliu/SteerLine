package testmanagement

import (
	"context"
	"github.com/erranliu/SteerLine/internal/testmanagement/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readyApply(t *testing.T) (*GenerationService, *memoryGenerations, *generationCLI, model.ApplyCommand) {
	t.Helper()
	s, m, f, in := generationFixture(t)
	s.Workspace = func(context.Context) (string, error) { return "head\nbranch\ntarget", nil }
	ctx := context.Background()
	if _, e := s.Generate(ctx, in); e != nil {
		t.Fatal(e)
	}
	f.finish(`{"tests":["unit.rules"]}`)
	if e := s.Observe(ctx); e != nil {
		t.Fatal(e)
	}
	r, _ := m.Generation(ctx, s.Project, in.RequestID)
	v, e := s.Preview(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	return s, m, f, model.ApplyCommand{RequestID: strings.Repeat("d", 32), GenerationID: r.ID, ExpectedRevision: r.Revision, ReviewDigest: v.ReviewDigest}
}
func TestPMApplyReviewBindsWorkspaceAndTarget(t *testing.T) {
	for _, which := range []string{"head", "input", "target"} {
		t.Run(which, func(t *testing.T) {
			s, m, f, in := readyApply(t)
			ctx := context.Background()
			r, _ := m.Generation(ctx, s.Project, in.GenerationID)
			switch which {
			case "head":
				s.Workspace = func(context.Context) (string, error) { return "other-head\nbranch\ntarget", nil }
			case "input":
				_ = os.WriteFile(filepath.Join(s.Repository, r.Command.Plan), []byte("# changed"), 0600)
			case "target":
				_ = os.WriteFile(filepath.Join(s.Repository, r.Target), []byte("user edit"), 0600)
			}
			if _, e := s.Apply(ctx, in); e == nil {
				t.Fatal("stale review overwrote files")
			}
			if f.submits != 1 {
				t.Fatal("application called model")
			}
			if which == "head" {
				r, _ = m.Generation(ctx, s.Project, in.GenerationID)
				v, e := s.Preview(ctx, r)
				if e != nil {
					t.Fatal(e)
				}
				in.RequestID = strings.Repeat("e", 32)
				in.ReviewDigest = v.ReviewDigest
				if _, e = s.Apply(ctx, in); e != nil {
					t.Fatal("explicit fresh review could not reuse output", e)
				}
			}
		})
	}
}
func TestPMRecoveryCancelledIntentNeverStartsNewWrite(t *testing.T) {
	s, m, f, in := readyApply(t)
	ctx := context.Background()
	m.failSaved = true
	if _, e := s.Apply(ctx, in); e == nil {
		t.Fatal("expected lost receipt")
	}
	r, _ := m.Generation(ctx, s.Project, in.GenerationID)
	b, e := os.ReadFile(filepath.Join(s.Repository, r.Target))
	if e != nil {
		t.Fatal(e)
	}
	// Cancellation cannot erase a write already completed before receipt loss.
	m.failSaved = false
	if _, e = s.Stop(ctx, r.ID, strings.Repeat("f", 32)); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(filepath.Join(s.Repository, r.Target))
	if string(after) != string(b) {
		t.Fatal("cancellation rolled back file")
	}
	if _, e = s.Apply(ctx, in); e != nil {
		t.Fatal("already completed write lost its fact", e)
	}
	if e = os.WriteFile(filepath.Join(s.Repository, r.Target), []byte("later user edit"), 0600); e != nil {
		t.Fatal(e)
	}
	in.RequestID = strings.Repeat("a", 32)
	if _, e = s.Apply(ctx, in); e == nil {
		t.Fatal("cancelled generation began another write")
	}
	if f.submits != 1 {
		t.Fatal("recovery regenerated")
	}
}
func TestPMRecoveryPersistenceFailureZeroFileWrite(t *testing.T) {
	s, m, _, in := readyApply(t)
	m.fail = true
	if _, e := s.Apply(context.Background(), in); e == nil {
		t.Fatal("expected denied intent")
	}
	r, _ := m.Generation(context.Background(), s.Project, in.GenerationID)
	if _, e := os.Stat(filepath.Join(s.Repository, r.Target)); !os.IsNotExist(e) {
		t.Fatal("write preceded intent")
	}
}

func TestPMApplyRejectsChangedRepositoryTargetEvenAfterReview(t *testing.T) {
	s, m, _, in := readyApply(t)
	s.Workspace = func(context.Context) (string, error) { return "head\nbranch\nother-target", nil }
	r, _ := m.Generation(context.Background(), s.Project, in.GenerationID)
	if _, e := s.Preview(context.Background(), r); e == nil {
		t.Fatal("new review adopted another repository target")
	}
}
func TestPMRecoveryUnwrittenIntentNeverAutoApplies(t *testing.T) {
	s, m, f, in := readyApply(t)
	ctx := context.Background()
	original := s.Workspace
	s.Workspace = func(ctx context.Context) (string, error) {
		r, _ := m.Generation(ctx, s.Project, in.GenerationID)
		if len(r.Applications) > 0 {
			return "moved\nbranch\ntarget", nil
		}
		return original(ctx)
	}
	if _, e := s.Apply(ctx, in); e == nil {
		t.Fatal("changed post-intent workspace was overwritten")
	}
	r, _ := m.Generation(ctx, s.Project, in.GenerationID)
	if len(r.Applications) != 1 || r.Applications[0].State != "intent" {
		t.Fatal("lost durable intent")
	}
	s.Workspace = original
	if e := s.Observe(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(s.Repository, r.Target)); !os.IsNotExist(e) {
		t.Fatal("restart observation wrote file")
	}
	if _, e := s.Stop(ctx, r.ID, strings.Repeat("f", 32)); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Apply(ctx, in); e == nil {
		t.Fatal("cancelled old intent wrote file")
	}
	if f.submits != 1 {
		t.Fatal("recovery generated replacement")
	}
}
func TestPMGenerationUnknownBlocksReplacement(t *testing.T) {
	s, _, f, in := generationFixture(t)
	f.lost = true
	ctx := context.Background()
	_, _ = s.Generate(ctx, in)
	replacement := in
	replacement.RequestID = strings.Repeat("e", 32)
	if _, e := s.Generate(ctx, replacement); e == nil || f.submits != 1 {
		t.Fatal("unknown generation was replaced")
	}
	f.finish(`{"tests":["unit.rules"]}`)
	f.lost = false
	if e := s.Observe(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Generate(ctx, replacement); e != nil || f.submits != 2 {
		t.Fatal("explicit new attempt refused after settlement", e)
	}
}

func TestPMGenerationAppliedResultStillObservesSettlement(t *testing.T) {
	s, m, f, in := readyApply(t)
	ctx := context.Background()
	f.run.StopStatus = ""
	f.run.Revision++
	if err := s.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	r, _ := m.Generation(ctx, s.Project, in.GenerationID)
	in.ExpectedRevision = r.Revision
	r, err := s.Apply(ctx, in)
	if err != nil || r.State != "applied" {
		t.Fatal("verified output could not be applied before settlement", r, err)
	}
	next := r.Command
	next.RequestID = strings.Repeat("e", 32)
	if _, err := s.Generate(ctx, next); err == nil {
		t.Fatal("unsettled original was replaced")
	}
	target := filepath.Join(s.Repository, r.Target)
	if err := os.WriteFile(target, []byte("user changed applied file"), 0600); err != nil {
		t.Fatal(err)
	}
	f.run.StopStatus = "stopped"
	f.run.Revision++
	if err := s.Observe(ctx); err != nil {
		t.Fatal(err)
	}
	settled, _ := m.Generation(ctx, s.Project, r.ID)
	if settled.State != "applied" || settled.Projection.StopStatus != "stopped" || string(settled.Result) != string(r.Result) {
		t.Fatal("applied file suppressed settlement or lost its original result", settled)
	}
	body, _ := os.ReadFile(target)
	if string(body) != "user changed applied file" || f.submits != 1 || f.stops != 0 {
		t.Fatal("settlement observation performed a new operation")
	}
	if _, err := s.Generate(ctx, next); err != nil || f.submits != 2 {
		t.Fatal("explicit new attempt refused after observed settlement", err)
	}
}
