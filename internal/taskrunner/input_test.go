package taskrunner

import (
	"encoding/json"
	"strings"
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
)

func TestRunnerExplicitInputKeepsOriginalRunAndDisplayedVersions(t *testing.T) {
	run := &runnerclient.Run{RunID: "native-run", Revision: 7, Status: "pr_ready", Controls: runnerclient.Controls{CanReply: true}, Execution: runnerclient.Execution{AttemptID: "original-attempt"}}
	candidate := runnerbridge.Candidate{ID: "candidate", RunID: run.RunID, Revision: 3, Participation: "active", Projection: run}
	decision := Decision(run)
	answer := api.DecisionAnswer{RunID: candidate.ID, DecisionID: decision.ID, Note: "preserve this feedback\n", ExpectedRunRevision: 3, ExpectedDecisionVersion: 7, SourceAttemptID: "original-attempt", ExpectedRecoveryVersion: 7}
	command, raw, err := InputForAnswer(candidate, answer)
	if err != nil || command != "reply" {
		t.Fatal(command, err)
	}
	var value struct {
		Revision   uint64 `json:"expected_revision"`
		Kind, Text string
	}
	if json.Unmarshal(raw, &value) != nil || value.Revision != 7 || value.Kind != "feedback" || value.Text != answer.Note {
		t.Fatal("feedback was changed or rebound", string(raw))
	}
	for _, change := range []func(*runnerbridge.Candidate, *api.DecisionAnswer){
		func(c *runnerbridge.Candidate, _ *api.DecisionAnswer) { c.StopRequested = true },
		func(_ *runnerbridge.Candidate, a *api.DecisionAnswer) { a.ExpectedRunRevision++ },
		func(_ *runnerbridge.Candidate, a *api.DecisionAnswer) { a.SourceAttemptID = "replacement" },
		func(_ *runnerbridge.Candidate, a *api.DecisionAnswer) { a.ExpectedDecisionVersion++ },
	} {
		c, a := candidate, answer
		change(&c, &a)
		if _, _, err := InputForAnswer(c, a); err == nil {
			t.Fatal("input ignored original candidate/Attempt/revision binding")
		}
	}
	run.Controls = runnerclient.Controls{CanResume: true}
	run.ResumeRequired = true
	resume := Decision(run)
	answer.DecisionID = resume.ID
	answer.Note = ""
	answer.OptionID = "resume"
	command, raw, err = InputForAnswer(candidate, answer)
	if err != nil || command != "resume" || string(raw) != `{"expected_revision":7}` {
		t.Fatal("resume manufactured a feedback message", command, string(raw), err)
	}
	answer.Note = "do not lose this business feedback"
	if _, _, err := InputForAnswer(candidate, answer); err == nil {
		t.Fatal("resume silently discarded business feedback")
	}
}

func TestRunnerNativeAnswersPreserveQuestionsAndCallerInput(t *testing.T) {
	var q runnerclient.Question
	if err := json.Unmarshal([]byte(`{"id":"native","native_questions":[{"id":"color","text":"Choose color","options":[{"id":"red","label":"Red"}]},{"id":"details","text":"Explain","allow_free_text":true}]}`), &q); err != nil {
		t.Fatal(err)
	}
	run := &runnerclient.Run{RunID: "original", Revision: 7, Question: &q, Controls: runnerclient.Controls{CanReply: true}}
	candidate := runnerbridge.Candidate{ID: "candidate", Revision: 3, Participation: "active", Projection: run}
	answer := api.DecisionAnswer{RunID: candidate.ID, DecisionID: q.ID, ExpectedRunRevision: 3, ExpectedDecisionVersion: 7, Answers: map[string][]string{"color": {"red"}, "details": {"because"}}}
	d := Decision(run)
	if !strings.Contains(d.Request.Context, "color: Choose color") || !strings.Contains(d.Request.Context, "red: Red") {
		t.Fatal("question IDs/options hidden", d)
	}
	command, raw, err := InputForAnswer(candidate, answer)
	if err != nil || command != "reply" || !strings.Contains(string(raw), `Red`) || answer.Answers["color"][0] != "red" {
		t.Fatal("answer changed or original map mutated", string(raw), err, answer)
	}
	for _, invalid := range []map[string][]string{{"color": {"blue"}, "details": {"why"}}, {"color": {"red"}}, {"color": {"red"}, "wrong": {"why"}}, {"color": {"red"}, "details": {" "}}} {
		bad := answer
		bad.Answers = invalid
		if _, _, err := InputForAnswer(candidate, bad); err == nil {
			t.Fatal("invalid answers accepted", invalid)
		}
	}
	bad := answer
	bad.Note = "ambiguous"
	if _, _, err := InputForAnswer(candidate, bad); err == nil {
		t.Fatal("ambiguous structured answer accepted")
	}
	for _, text := range []string{`{"color":["red"],"color":["red"],"details":["because"]}`, `{"color":["red"],"details":["because"]} {}`} {
		bad := answer
		bad.Answers = nil
		bad.Note = text
		if _, _, err := InputForAnswer(candidate, bad); err == nil {
			t.Fatal("ambiguous answer text accepted", text)
		}
	}
	q.NativeQuestions = q.NativeQuestions[:1]
	d = Decision(run)
	if d.Request.Question != "Choose color" || len(d.Request.Options) != 1 || d.Request.Options[0].ID != "red" {
		t.Fatal("single question options hidden", d)
	}
	answer.Answers = map[string][]string{"color": {"blue"}}
	if _, _, err := InputForAnswer(candidate, answer); err == nil {
		t.Fatal("unoffered single option accepted")
	}
	answer.Answers = map[string][]string{"color": {"red"}}
	_, raw, err = InputForAnswer(candidate, answer)
	if err != nil || !strings.Contains(string(raw), `"text":"Red"`) || answer.Answers["color"][0] != "red" {
		t.Fatal(string(raw), err, answer)
	}
}
