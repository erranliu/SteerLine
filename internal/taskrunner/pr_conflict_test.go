package taskrunner

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"os"
	"testing"
)

func TestPRConflictInputUsesOriginalBindingAndShortPrompt(t *testing.T) {
	r := &runnerclient.Run{RunID: "original", Revision: 7, Status: "pr_ready", Controls: runnerclient.Controls{CanMessage: true}, PR: &runnerclient.PRFact{URL: "https://example.test/repo/pull/1", BaseBranch: "main", State: "open"}}
	c := runnerbridge.Candidate{ID: "candidate", RunID: "original", Participation: "active", Projection: r}
	raw, err := conflictInput(c)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Revision uint64 `json:"expected_revision"`
		Text     string `json:"text"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../prompts/testdata/pr-conflict-resolution.golden.md")
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != 7 || got.Text != string(want) {
		t.Fatalf("actual caller body changed: %d bytes: %q", len(got.Text), got.Text)
	}
	for _, change := range []func(*runnerbridge.Candidate){
		func(c *runnerbridge.Candidate) { c.Participation = "abandoned" },
		func(c *runnerbridge.Candidate) { c.StopRequested = true },
		func(c *runnerbridge.Candidate) { c.Projection.Controls.CanMessage = false },
		func(c *runnerbridge.Candidate) { c.Projection.PR.Merged = true },
		func(c *runnerbridge.Candidate) { c.Projection.Status = "running" },
	} {
		copy := c
		runCopy := *r
		prCopy := *r.PR
		runCopy.PR = &prCopy
		copy.Projection = &runCopy
		change(&copy)
		if _, err := conflictInput(copy); err == nil {
			t.Fatal("inactive target accepted")
		}
	}
}
