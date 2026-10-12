package execution

import (
	"context"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	m "github.com/erranliu/SteerLine/internal/execution/model"
	"os/exec"
	"strings"
	"testing"
)

func TestAsyncSavedAnswerKeepsReportAndBindsSourceContinuation(t *testing.T) {
	s, run, port := workerStageFixture(t)
	t.Cleanup(func() { _ = s.Close() })
	s.State.Service.Ready = true
	w, e := s.workflowRecords(run)
	fixed := *workerSessionContext(run, e)
	fixed.ProjectID = s.Config.identity
	request := neutralDecisionFixture(t, fixed, "async-item", []b.Question{{ID: "publisher", Header: "Publisher", Question: "Publisher?", Options: []b.DecisionOption{{ID: "worker", Label: "worker"}}, AllowFreeText: true}})
	require(t, s.eventWorkerFlowFor(run, WorkerEvent{RunID: run.RunID, AttemptID: run.AttemptID, Fact: b.EventFact{InteractionKind: "async", Questions: request.Questions, QuestionRoute: request.QuestionRoute}}))
	answer := m.DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, Answers: map[string][]string{"publisher": {"worker"}}, AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: s.State.Service.InstanceID, SourceAttemptID: run.AttemptID}
	commands := newTestCommandService(s.commandStore, s.Config)
	answerCommand := newID()
	command, _, err := commands.AnswerDecisionWithID(context.Background(), answerCommand, answer.DecisionID, answer.ExpectedDecisionVersion, DecisionAnswerKey(answer), answer.AnsweredBy, mustJSON(answer))
	require(t, err)
	_, _, err = commands.AcceptAnswer(context.Background(), command, answer)
	require(t, err)
	require(t, s.refreshNativeSnapshot(context.Background()))
	w, e = s.workflowRecords(run)
	if w.Status != "running" || run.Decision.Status != "answer_saved" {
		t.Fatal("saved asynchronous answer stopped worker")
	}
	require(t, acceptTestStage(t, s, run, port, "partial result", true))
	w, e = s.workflowRecords(run)
	reportCommand := e.PendingCommandID
	_, err = s.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+e.ExecutionID+":"+reportCommand, b.TurnInput{CommandID: reportCommand, Prompt: stageInput(w, reportCommand).Prompt})
	require(t, err)
	e.PendingTurnID = "report-turn"
	_, err = s.recordWorkerTurnStarted(run, w, e, e.PendingTurnID)
	require(t, err)
	require(t, s.commitWorker(run))
	if run.Decision.Status != "answer_saved" {
		t.Fatal("automatic report falsely acknowledged saved answer")
	}
	require(t, acceptTestStage(t, s, run, port, `{"outcome":"needs_input","summary":"partial","question":"Publisher?"}`, true))
	original := run.Decision.Request
	step, err := asyncContinuationStep(w, original)
	require(t, err)
	if step != m.StepImplement {
		t.Fatal("async continuation selected report instead of original implementation")
	}
	saved, err := ReadDecisionAnswer(run)
	require(t, err)
	if out, err := exec.Command("git", "-C", s.Config.Repository, "worktree", "add", "-b", e.Workspace.Branch, e.Workspace.Worktree, "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("prepare isolated continuation worktree: %v %s", err, out)
	}
	require(t, s.startStageFeedback(context.Background(), run, saved, answerCommand, original))
	w, e = s.workflowRecords(run)
	next := stageInput(w, answerCommand)
	if next == nil || next.StepID != m.StepImplement || next.FeedbackDecisionID != answer.DecisionID || !strings.Contains(next.Prompt, `"source_turn_id":"native-turn-1"`) || !strings.Contains(next.Prompt, `"question_id":"async-item"`) {
		t.Fatalf("continuation lost source binding: %+v", next)
	}
}

func TestAsyncQuestionWorkerRunningReportRetainsOrExpires(t *testing.T) {
	for _, outcome := range []string{"needs_input", "done"} {
		t.Run(outcome, func(t *testing.T) {
			s, run, port := workerStageFixture(t)
			w, e := s.workflowRecords(run)
			fixed := *workerSessionContext(run, e)
			fixed.ProjectID = s.Config.identity
			req := neutralDecisionFixture(t, fixed, "original-question", []b.Question{{ID: "publisher", Header: "Publisher", Question: "Publisher?", Options: []b.DecisionOption{{ID: "worker", Label: "worker"}, {ID: "host", Label: "host_service"}}, AllowFreeText: true}})
			fact := b.EventFact{InteractionKind: "async", Questions: req.Questions, QuestionRoute: req.QuestionRoute}
			require(t, s.eventWorkerFlowFor(run, WorkerEvent{RunID: run.RunID, AttemptID: run.AttemptID, Fact: fact}))
			id := run.Decision.ID
			if w.Status != "running" || run.Decision.Request.BlocksProgress || run.Decision.Request.InteractionKind != "async" {
				t.Fatalf("question stopped worker: %+v", run)
			}
			// Replayed native events must not create another Decision.
			require(t, s.eventWorkerFlowFor(run, WorkerEvent{RunID: run.RunID, AttemptID: run.AttemptID, Fact: fact}))
			if run.Decision.ID != id {
				t.Fatal("duplicate decision")
			}
			require(t, acceptTestStage(t, s, run, port, "partial work; publisher needed", true))
			if w.Status != "queued" || w.StepID != m.StepReport || run.Decision.ID != id {
				t.Fatal("async question blocked report or was replaced")
			}
			startQueuedStage(t, s, run)
			body := `{"outcome":"` + outcome + `","summary":"facts","outputs":{},"question":"Publisher?"}`
			if outcome == "done" {
				body = `{"outcome":"done","summary":"facts","outputs":{}}`
			}
			require(t, acceptTestStage(t, s, run, port, body, true))
			if run.Decision.ID != id || run.Decision.Request.Options[1].Label != "host_service" {
				t.Fatal("original question or options lost")
			}
			if outcome == "needs_input" && (w.Status != "waiting_input" || run.Decision.Status != "pending") {
				t.Fatalf("unanswered required question did not wait: %+v", run)
			}
			if outcome == "done" && run.Decision.Status != "superseded" {
				t.Fatal("obsolete question stayed pending")
			}
			loaded, err := loadFixtureRuntime(s.Config)
			require(t, err)
			if len(loaded.Runs) != 1 || loaded.Runs[0].Decision == nil || loaded.Runs[0].Decision.ID != id || loaded.Runs[0].Decision.Request.Options[1].Label != "host_service" || loaded.Runs[0].Decision.Request.InteractionKind != "async" {
				t.Fatal("SQLite lost original question identity or options")
			}

		})
	}
}
