package model

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTaskDeliveryValidatesFixedMergeEvidence(t *testing.T) {
	at := time.Date(2026, 10, 17, 1, 2, 3, 0, time.UTC)
	proof := TaskDeliveryEvidence{PRInstance: "https://example.invalid", URL: "https://example.invalid/org/repo/pulls/7", Repository: "org/repo", PRNumber: 7, BaseBranch: "main", State: "merged", Merged: true, MergedAt: at, MergeCommit: strings.Repeat("a", 40)}
	for _, tc := range []struct {
		name, code string
		change     func(*TaskDeliveryRecord, *TaskDeliveryEvidence)
	}{
		{"valid sha1", "", func(*TaskDeliveryRecord, *TaskDeliveryEvidence) {}},
		{"valid sha256", "", func(r *TaskDeliveryRecord, p *TaskDeliveryEvidence) {
			r.MergeCommit = strings.Repeat("b", 64)
			p.MergeCommit = r.MergeCommit
		}},
		{"invalid commit", "inconsistent_delivery", func(r *TaskDeliveryRecord, _ *TaskDeliveryEvidence) { r.MergeCommit = strings.Repeat("z", 40) }},
		{"foreign PR", "inconsistent_delivery", func(_ *TaskDeliveryRecord, p *TaskDeliveryEvidence) { p.Repository = "other/repo" }},
		{"wrong target", "inconsistent_delivery", func(_ *TaskDeliveryRecord, p *TaskDeliveryEvidence) { p.BaseBranch = "other" }},
		{"unconfirmed merge", "inconsistent_delivery", func(_ *TaskDeliveryRecord, p *TaskDeliveryEvidence) { p.Merged = false }},
		{"retired acceptance", "unsupported_schema", func(_ *TaskDeliveryRecord, p *TaskDeliveryEvidence) {
			raw := json.RawMessage(`{}`)
			p.ExistingAcceptance = &raw
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := proof
			r := TaskDeliveryRecord{ProjectIdentity: "project", TaskID: "1", SelectedWorkflowRunID: "candidate", PRInstance: p.PRInstance, PRRepository: p.Repository, PRNumber: p.PRNumber, PRURL: p.URL, MergeCommit: p.MergeCommit, MergedAt: at, RecordedAt: at}
			tc.change(&r, &p)
			r.Evidence, _ = json.Marshal(p)
			before, _ := json.Marshal(r)
			err := ValidateDelivery(r, "main")
			var rejected *Error
			if tc.code == "" && err != nil || tc.code != "" && (!errors.As(err, &rejected) || rejected.Code != tc.code) {
				t.Fatalf("want code %q, got %v", tc.code, err)
			}
			after, _ := json.Marshal(r)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("validation mutated fixed evidence")
			}
		})
	}
}
