package steerline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decisionRun(t *testing.T) *Run {
	t.Helper()
	dir := t.TempDir()
	return &Run{
		TaskID:           "t-1",
		RunID:            "0123456789abcdef0123456789abcdef",
		Claimant:         "c",
		Host:             "orca",
		FinishTaskStatus: "waiting_decision",
		ResultPath:       ptr(filepath.Join(dir, "result.json")),
		WorktreePath:     ptr(filepath.Join(dir, "wt")),
	}
}

func TestDecisionRequestAndAnswerRoundTrip(t *testing.T) {
	run := decisionRun(t)
	st, e := WriteDecisionRequest(run, DecisionRequest{
		Question:    "pick",
		Options:     []DecisionOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
		Recommended: "a",
	})
	if e != nil {
		t.Fatal(e)
	}
	run.Decision = st
	if _, e := os.Stat(st.RequestPath); e != nil {
		t.Fatalf("request file missing: %v", e)
	}
	if e := WriteAnswer(run, DecisionAnswer{DecisionID: st.ID, OptionID: "b", AnsweredBy: "alice"}); e != nil {
		t.Fatal(e)
	}
	a, e := ReadDecisionAnswer(run)
	if e != nil {
		t.Fatal(e)
	}
	if a.OptionID != "b" || a.AnsweredBy != "alice" || a.DecisionID != st.ID {
		t.Fatalf("bad answer: %+v", a)
	}
}

func TestDecisionAnswerRejectsBadInput(t *testing.T) {
	run := decisionRun(t)
	st, e := WriteDecisionRequest(run, DecisionRequest{Question: "q", Options: []DecisionOption{{ID: "a", Label: "A"}}})
	if e != nil {
		t.Fatal(e)
	}
	run.Decision = st
	cases := map[string]DecisionAnswer{
		"missing option": {AnsweredBy: "x"},
		"unknown option": {OptionID: "z", AnsweredBy: "x"},
		"missing by":     {OptionID: "a"},
		"wrong id":       {DecisionID: "nope", OptionID: "a", AnsweredBy: "x"},
	}
	for name, a := range cases {
		if e := WriteAnswer(run, a); e == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestDecisionRequestValidation(t *testing.T) {
	cases := map[string]DecisionRequest{
		"empty":           {},
		"no options":      {Question: "q"},
		"empty option id": {Question: "q", Options: []DecisionOption{{ID: "", Label: "x"}}},
		"duplicate id":    {Question: "q", Options: []DecisionOption{{ID: "a", Label: "x"}, {ID: "a", Label: "y"}}},
		"bad recommend":   {Question: "q", Options: []DecisionOption{{ID: "a", Label: "x"}}, Recommended: "b"},
	}
	for name, req := range cases {
		c := req
		if e := validateDecisionRequest(&c); e == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func TestFreeTextDecisionRequiresNote(t *testing.T) {
	run := decisionRun(t)
	st, e := WriteDecisionRequest(run, DecisionRequest{Question: "explain", AllowFreeText: true})
	if e != nil {
		t.Fatal(e)
	}
	run.Decision = st
	if e := WriteAnswer(run, DecisionAnswer{AnsweredBy: "x"}); e == nil {
		t.Fatal("expected an error without a note")
	}
	if e := WriteAnswer(run, DecisionAnswer{Note: "because", AnsweredBy: "x"}); e != nil {
		t.Fatal(e)
	}
}

func TestAnswerDecisionRejectsV3RuntimeWithoutMutatingIt(t *testing.T) {
	dir := t.TempDir()
	run := decisionRun(t)
	st, e := WriteDecisionRequest(run, DecisionRequest{Question: "q", Options: []DecisionOption{{ID: "a", Label: "A"}}})
	if e != nil {
		t.Fatal(e)
	}
	run.Decision = st
	b, e := json.Marshal(&Runtime{SchemaVersion: 3, Runs: []*Run{run}})
	if e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "runtime.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	c := Config{RunDir: dir, RuntimeFile: "runtime.json"}
	if e := AnswerDecision(c, AnswerOptions{RunID: run.TaskID, Option: "a", By: "bob"}); e == nil || !strings.Contains(e.Error(), "unsupported runtime schema 3") {
		t.Fatalf("v3 answer was accepted: %v", e)
	}
	if _, e = os.Stat(run.Decision.AnswerPath); !os.IsNotExist(e) {
		t.Fatalf("rejected old runtime was mutated: %v", e)
	}
	if _, e = LoadRuntime(filepath.Join(dir, "runtime.json")); e == nil {
		t.Fatal("v3 runtime was readable")
	}
}

// TestOrcaDecisionResumesRun drives the full human-in-the-loop path against the
// protocol fake: the agent asks for a decision, a human answers, the service
// resumes the same run, and the agent then publishes the PR.
func TestOrcaDecisionResumesRun(t *testing.T) {
	f := newFixture(t, task("t-1"))
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	stateDir := filepath.Join(filepath.Dir(f.c.RunDir), "orca-state")
	if e := os.MkdirAll(stateDir, 0700); e != nil {
		t.Fatal(e)
	}
	f.c.Host = "orca"
	f.c.Agent = "codex"
	f.c.Orca = OrcaConfig{Executable: exe, Args: []string{"--fake", "orca", stateDir}, Shell: "cmd.exe", WaitTimeout: "5s", Fallback: "fail"}
	t.Setenv("STEERLINE_FAKE_ORCA_PR_BASE", f.server.URL)
	t.Setenv("STEERLINE_FAKE_ORCA_DECISION", "1")

	ctx := context.Background()
	s := f.service(t)
	require(t, s.dispatch(ctx))
	if s.worker == nil {
		t.Fatal("orca worker was not started")
	}
	finishWorker(t, s)

	run := s.State.Runs[0]
	if run.FinishTaskStatus != "waiting_decision" || run.Decision == nil {
		t.Fatalf("run did not pause for a decision: %+v", run)
	}
	if !fileExists(run.Decision.RequestPath) {
		t.Fatalf("decision request file missing: %s", run.Decision.RequestPath)
	}
	if run.PR.URL != nil {
		t.Fatalf("decision run must not bind a PR: %v", *run.PR.URL)
	}

	_, _, e = (CommandService{Store: s.domainStore, Project: s.Config.identity}).AnswerDecision(ctx, run.Decision.ID, run.Decision.Version, "orca-answer", "tester", mustJSON(DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: "a", Note: "keep compat", AnsweredBy: "tester"}))
	require(t, e)
	if run.Decision.Status != "pending" {
		t.Fatalf("answer must not mutate run state: %s", run.Decision.Status)
	}
	require(t, s.processCommandsAndDecisions(ctx))
	if run.Decision.Status != "reply_accepted" {
		t.Fatalf("decision not applied: %+v", run.Decision)
	}
	if s.worker == nil {
		t.Fatal("decision did not resume a worker")
	}
	finishWorker(t, s)

	run = s.State.Runs[0]
	if run.FinishTaskStatus != "waiting_pr_merge" || run.PR.URL == nil {
		t.Fatalf("resumed run did not bind a PR: %+v", run)
	}
	act, e := os.ReadFile(filepath.Join(f.c.RunDir, "runs", run.RunID, "activity.jsonl"))
	require(t, e)
	for _, want := range []string{"decision", "answered", "reply_accepted", "pr submitted"} {
		if !strings.Contains(string(act), want) {
			t.Fatalf("activity journal missing %q: %s", want, act)
		}
	}
}
