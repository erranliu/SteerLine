package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/hostcontract"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

func bindDeliveryCandidate(t *testing.T, s *SQLiteStore, c runnerbridge.Candidate, number int) runnerbridge.Candidate {
	t.Helper()
	c.RunID, c.InputDigest, c.Branch, c.SubmitState = "run-"+c.ID, "frozen-input", "head-"+c.ID, "accepted"
	c.PRURL = fmt.Sprintf("https://example.invalid/Org/Repo/pulls/%d", number)
	c.Projection = &runnerclient.Run{RunID: c.RunID, InputDigest: c.InputDigest, Revision: 8, Status: "running",
		Execution: runnerclient.Execution{Branch: c.Branch, Workspace: "workspace", SessionID: "session", AttemptID: "attempt", Generation: "generation"},
		PR:        &runnerclient.PRFact{Provider: "gitea", Number: number, URL: c.PRURL, Repository: "Org/Repo", HeadRepository: "Org/Repo", BaseBranch: "main", HeadBranch: c.Branch, HeadRevision: strings.Repeat("a", 40), State: "open"}}
	c.Revision++
	require(t, s.SaveCandidate(context.Background(), c, c.Revision-1, nil))
	return c
}

func TestAutomaticDeliveryActorKeepsStopPriority(t *testing.T) {
	for _, mode := range []string{"stop before result", "result signal before stop", "delivery before stop", "first unmerged", "provider timeout", "shutdown"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s, project, reserve := candidateStopFixture(t)
			first := bindDeliveryCandidate(t, s, reserve(strings.Repeat("a", 32), false), 84)
			second := bindDeliveryCandidate(t, s, reserve(strings.Repeat("b", 32), true), 85)
			_, err := s.db.ExecContext(ctx, `UPDATE project_task_authorities SET dispatch_mode='auto' WHERE project_identity=?`, project)
			require(t, err)
			observations := map[string]*blockedObservation{
				first.StateDir:  {run: *first.Projection, immediate: true},
				second.StateDir: {run: *second.Projection, immediate: true},
			}
			entered := make(chan string, 2)
			release := make(chan struct{})
			queryExited := make(chan struct{}, 2)
			svc := &taskrunner.Service{Project: project, Store: s, Bridge: &runnerbridge.Bridge{Store: s, Client: func(dir string) runnerbridge.CLI { return observations[dir] }}}
			dispatchChecks := 0
			svc.CheckExecution = func(context.Context, string, bool) error {
				dispatchChecks++
				return &taskmodel.Error{Code: "environment_not_ready", Message: "controlled dispatch boundary"}
			}
			svc.QueryPR = func(work context.Context, provider model.PRProviderConfig, url string) (model.PRInfo, error) {
				defer func() { queryExited <- struct{}{} }()
				if provider.Kind != "gitea" || provider.Repository != "Org/Repo" {
					return model.PRInfo{}, fmt.Errorf("query lost frozen provider")
				}
				entered <- url
				if url == first.PRURL {
					select {
					case <-release:
					case <-work.Done():
						return model.PRInfo{}, work.Err()
					}
					if mode == "provider timeout" {
						return model.PRInfo{}, context.DeadlineExceeded
					}
					if mode == "first unmerged" {
						return fixturePRObservation(first, time.Now().UTC(), "open", nil).Fact, nil
					}
					merge := strings.Repeat("b", 40)
					return fixturePRObservation(first, time.Now().UTC(), "merged", &merge).Fact, nil
				}
				if mode == "first unmerged" || mode == "provider timeout" {
					merge := strings.Repeat("b", 40)
					return fixturePRObservation(second, time.Now().UTC(), "merged", &merge).Fact, nil
				}
				return fixturePRObservation(second, time.Now().UTC(), "open", nil).Fact, nil
			}
			host := &stopHost{done: make(chan api.ControlReceipt, 1)}
			controls := taskrunner.Controls{Tasks: svc, Store: s, ResolveProfile: func(string, string) (string, error) {
				t.Fatal("stop consulted the execution profile")
				return "", nil
			}}
			var actor *taskrunner.Actor
			actor, err = taskrunner.NewActor(taskrunner.ActorOptions{Tasks: svc, Store: s, Host: host, Repository: hostcontract.Repository{Identity: project}, Mode: "manual", Heartbeat: time.Second, Control: func(ctx context.Context, r api.ControlRequest) (api.ControlReceipt, error) {
				return controls.Handle(ctx, r, actor.InstanceID(), 1)
			}})
			require(t, err)
			t.Cleanup(func() {
				cancel()
				actor.WaitSources()
			})
			require(t, actor.BeginActor(ctx))
			require(t, actor.Initialize(ctx, cancel))
			require(t, actor.RunWork(ctx, actor.TickPR))
			wantQuery := func(url string) {
				t.Helper()
				select {
				case got := <-entered:
					if got != url {
						t.Fatal("query order or binding changed", got, url)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("missing automatic query", url)
				}
			}
			wantQuery(first.PRURL)
			if mode == "shutdown" {
				cancel()
				require(t, actor.EndActor(nil))
				actor.WaitSources()
				select {
				case <-queryExited:
				default:
					t.Fatal("project returned before its platform query exited")
				}
				graph, err := s.TaskGraphSnapshot(context.Background(), project)
				require(t, err)
				if len(graph.Deliveries) != 0 {
					t.Fatal("shutdown published late delivery")
				}
				return
			}
			for range 3 {
				observations[first.StateDir].run.Revision++
				require(t, actor.RunWork(ctx, actor.TickDecision))
				require(t, actor.RunWork(ctx, func(work context.Context) error { return actor.TickTask(work, cancel) }))
				require(t, actor.RunWork(ctx, actor.TickPR))
			}
			observed, err := s.Candidate(ctx, project, first.ID)
			require(t, err)
			if observed.Projection.Revision != first.Projection.Revision+3 || dispatchChecks != 3 || len(entered) != 0 {
				t.Fatal("platform wait stalled observation/dispatch or allowed duplicate queries", observed.Revision, dispatchChecks)
			}
			advance := func(beforeHandle func()) {
				t.Helper()
				select {
				case signal := <-actor.Signals():
					if beforeHandle != nil {
						beforeHandle()
					}
					require(t, actor.RunWork(ctx, func(context.Context) error { return actor.HandleSignal(signal) }))
				case <-time.After(5 * time.Second):
					t.Fatal("query did not return an actor result")
				}
			}
			if mode == "first unmerged" || mode == "provider timeout" {
				close(release)
				advance(nil)
				wantQuery(second.PRURL)
				advance(nil)
				graph, err := s.TaskGraphSnapshot(ctx, project)
				require(t, err)
				if graph.Deliveries[first.TaskID].SelectedWorkflowRunID != second.ID {
					t.Fatal("first PR blocked delivery of a later candidate", graph.Deliveries)
				}
				return
			}
			publishStop := func() {
				payload, _ := json.Marshal(api.ControlPayload{RunID: first.ID, TaskID: first.TaskID, ExpectedRunRevision: observed.Revision})
				at := time.Now().UTC()
				host.request = &api.ControlRequest{ProtocolVersion: 1, RequestID: strings.Repeat("e", 32), RepositoryIdentity: project, InstanceID: actor.InstanceID(), Action: "stop_candidate", ExpectedControlRevision: 1, RequestedBy: "operator", CreatedAt: at, ExpiresAt: at.Add(time.Minute), Payload: payload}
				actor.NotifyControl()
			}
			if mode == "stop before result" {
				publishStop()
			} else {
				close(release)
				if mode == "result signal before stop" {
					advance(publishStop)
				} else {
					advance(nil)
					publishStop()
				}
			}
			require(t, actor.TickControl(ctx, cancel))
			var receipt api.ControlReceipt
			select {
			case receipt = <-host.done:
			case <-time.After(5 * time.Second):
				t.Fatal("stop receipt did not arrive")
			}
			if mode == "stop before result" {
				close(release)
				advance(nil)
			}
			graph, err := s.TaskGraphSnapshot(ctx, project)
			require(t, err)
			if mode == "delivery before stop" {
				if graph.Deliveries[first.TaskID].SelectedWorkflowRunID != first.ID || receipt.Status != "rejected" {
					t.Fatal("committed delivery was changed by later stop", receipt, graph.Deliveries)
				}
				return
			}
			if receipt.Status != "applied" || len(graph.Deliveries) != 0 {
				t.Fatal("late platform fact bypassed the stop", receipt, graph.Deliveries)
			}
			wantQuery(second.PRURL)
			advance(nil)
		})
	}
}

func TestAutomaticDeliveryTransactionSurvivesObservation(t *testing.T) {
	ctx := context.Background()
	s, project, reserve := candidateStopFixture(t)
	c := bindDeliveryCandidate(t, s, reserve("original", false), 84)
	sibling := reserve("sibling", true)
	graph, err := s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	binding, _, err := ports.BindAutomaticDelivery(c, graph.Authority.Branch)
	require(t, err)
	started := time.Now().UTC()
	merge := strings.Repeat("b", 40)
	in := RunnerResult{Project: project, TaskID: c.TaskID, CandidateID: c.ID, TaskVersion: graph.ByID[c.TaskID].Version, CandidateRevision: c.Revision, AuthorityVersion: graph.Authority.AuthorityVersion, GraphRevision: graph.Authority.GraphRevision, VerificationStarted: started, AutomaticBinding: &binding}
	for i := 0; i < 3; i++ {
		c.Revision++
		c.Projection.Revision++
		c.Cursor = fmt.Sprintf("page-%d", i)
		require(t, s.SaveCandidate(ctx, c, c.Revision-1, []runnerclient.Event{{EventID: c.Cursor, RunID: c.RunID, Revision: c.Projection.Revision}}))
	}
	in.PR = fixturePRObservation(c, time.Now().UTC(), "merged", &merge)
	in.PR.CandidateRevision = in.CandidateRevision
	strict := in
	strict.AutomaticBinding = nil
	if _, err := s.CommitRunnerTaskResult(ctx, strict); err == nil {
		t.Fatal("strict synchronous delivery accepted a changed candidate revision")
	}
	reject := func(proof RunnerResult) {
		t.Helper()
		if _, err := s.CommitRunnerTaskResult(ctx, proof); err == nil {
			t.Fatal("invalid evidence committed a delivery")
		}
	}
	expired := in
	expired.PR.CheckedAt = started.Add(-time.Minute)
	expired.PR.StartedAt = expired.PR.CheckedAt
	expired.VerificationStarted = expired.PR.StartedAt
	reject(expired)
	foreign := in
	foreign.PR.Fact.URL += "0"
	reject(foreign)
	stale := in
	stale.GraphRevision++
	reject(stale)
	stale = in
	stale.TaskVersion++
	reject(stale)
	for _, mutate := range []func(*runnerbridge.Candidate){
		func(v *runnerbridge.Candidate) { v.Claimant = "replacement" },
		func(v *runnerbridge.Candidate) { v.Submit.RequestID = "different-original" },
		func(v *runnerbridge.Candidate) { v.Submit.Input = append([]byte{0x20}, v.Submit.Input...) },
		func(v *runnerbridge.Candidate) { v.InputDigest, v.Projection.InputDigest = "different", "different" },
		func(v *runnerbridge.Candidate) { v.Projection.Execution.Generation = "replacement" },
		func(v *runnerbridge.Candidate) { v.Projection.PR.HeadRevision = strings.Repeat("c", 40) },
	} {
		other := c
		projection, pr := *c.Projection, *c.Projection.PR
		other.Projection, projection.PR = &projection, &pr
		mutate(&other)
		changedBinding, _, err := ports.BindAutomaticDelivery(other, graph.Authority.Branch)
		require(t, err)
		changed := in
		changed.AutomaticBinding = &changedBinding
		reject(changed)
	}
	c.Projection.Execution.SessionID = "replacement"
	c.Revision++
	require(t, s.SaveCandidate(ctx, c, c.Revision-1, nil))
	reject(in)
	unchanged, err := s.TaskGraphSnapshot(ctx, project)
	require(t, err)
	other, err := s.Candidate(ctx, project, sibling.ID)
	require(t, err)
	if len(unchanged.Deliveries) != 0 || unchanged.Authority.GraphRevision != graph.Authority.GraphRevision || other.StopRequested {
		t.Fatal("failed verification partially applied delivery")
	}
	c.Projection.Execution.SessionID = "session"
	c.Revision++
	require(t, s.SaveCandidate(ctx, c, c.Revision-1, nil))
	// Take fresh evidence after the session changed. More observations can
	// arrive without invalidating this query's now stable binding.
	in.CandidateRevision = c.Revision
	in.VerificationStarted = time.Now().UTC()
	in.PR = fixturePRObservation(c, in.VerificationStarted, "merged", &merge)
	c.Projection.Revision++
	c.Projection.Step = "publish.report"
	c.Revision++
	require(t, s.SaveCandidate(ctx, c, c.Revision-1, nil))
	receipt, err := s.CommitRunnerTaskResult(ctx, in)
	require(t, err)
	replay, err := s.CommitRunnerTaskResult(ctx, in)
	require(t, err)
	if receipt.Status != "closed" || digestValue(receipt) != digestValue(replay) {
		t.Fatal("automatic delivery changed its original receipt", receipt, replay)
	}
}
