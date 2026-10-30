package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEnvironmentPublicationReportRejectsRetiredMergeContract(t *testing.T) {
	r := Record{ID: "activity", Prepared: Prepared{Commit: "fixed", ScriptDigest: "script", L2: []string{"one"}}}
	a := Round{Number: 1}
	p := Report{Version: ReportVersion, ActivityID: r.ID, Round: a.Number, Commit: r.Prepared.Commit, ScriptDigest: r.Prepared.ScriptDigest, Completed: true, Checks: []Check{{ID: "terminal.tty", Status: "passed", Message: "fixture"}}, L2: []Item{{ID: "one", Status: "passed"}}, PR: PR{URL: "https://example.invalid/pulls/1", Base: "validation", Head: "head", HeadRevision: strings.Repeat("a", 40), CleanupCompleted: true}}
	body, _ := json.Marshal(p)
	if _, err := ParseReport(body, r, a); err != nil || !p.Passed("validation", "head") {
		t.Fatal("valid unmerged publication rejected", err)
	}
	for _, field := range []string{"checks_passed", "reviews_passed", "merged", "merge_commit"} {
		var value map[string]any
		_ = json.Unmarshal(body, &value)
		value["pr"].(map[string]any)[field] = true
		bad, _ := json.Marshal(value)
		if _, err := ParseReport(bad, r, a); err == nil {
			t.Fatal("retired merge field accepted", field)
		}
	}
	p.Version = 1
	body, _ = json.Marshal(p)
	if _, err := ParseReport(body, r, a); err == nil {
		t.Fatal("old report version accepted")
	}
}
