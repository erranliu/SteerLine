//go:build e2e

package testenv

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
)

func TestEnvironmentPublicationRequiredChecksAndIndependentApprovals(t *testing.T) {
	for _, state := range []string{"missing", "pending", "failure", "success"} {
		statuses := []environmentStatus{}
		if state != "missing" {
			statuses = append(statuses, environmentStatus{Context: "required", State: state})
		}
		if (requireEnvironmentChecks([]string{"required"}, statuses) == nil) != (state == "success") {
			t.Fatal("required check gate", state)
		}
	}
	review := func(login, state string, dismissed bool) environmentReview {
		r := environmentReview{State: state, Dismissed: dismissed}
		r.User.Login = login
		return r
	}
	for _, tc := range []struct {
		name    string
		reviews []environmentReview
		pass    bool
	}{
		{"missing", nil, false},
		{"author-only", []environmentReview{review("author", "APPROVED", false)}, false},
		{"dismissed", []environmentReview{review("peer", "APPROVED", true)}, false},
		{"withdrawn", []environmentReview{review("peer", "APPROVED", false), review("peer", "CHANGES_REQUESTED", false)}, false},
		{"approved", []environmentReview{review("peer", "APPROVED", false)}, true},
	} {
		if (requireEnvironmentReviews(1, "author", tc.reviews) == nil) != tc.pass {
			t.Fatal("independent approval gate", tc.name)
		}
	}
}

func TestEnvironmentPublicationExternalMergeKeepsOriginalReport(t *testing.T) {
	for _, mode := range []string{"success", "lost-receipt", "unknown", "identity-changed", "closed", "read-failed", "already-merged"} {
		t.Run(mode, func(t *testing.T) {
			expected := env.PR{URL: "https://example.invalid/repo/pulls/1", Base: "owned/base", Head: "owned/head", HeadRevision: strings.Repeat("a", 40), CleanupCompleted: true}
			before, _ := json.Marshal(expected)
			current := environmentPR{URL: expected.URL, State: "open"}
			current.Base.Ref = expected.Base
			current.Head.Ref = expected.Head
			current.Head.SHA = expected.HeadRevision
			if mode == "identity-changed" {
				current.Head.SHA = strings.Repeat("b", 40)
			}
			if mode == "closed" {
				current.State = "closed"
			}
			if mode == "already-merged" {
				current.Merged = true
				current.Merge = strings.Repeat("c", 40)
			}
			writes := 0
			read := func() (environmentPR, error) {
				if mode == "read-failed" {
					return current, errors.New("provider read unavailable")
				}
				return current, nil
			}
			merge := func() error {
				writes++
				if mode == "unknown" {
					return errors.New("receipt lost")
				}
				current.Merged = true
				current.Merge = strings.Repeat("c", 40)
				if mode == "lost-receipt" {
					return errors.New("receipt lost")
				}
				return nil
			}
			intent := filepath.Join(t.TempDir(), "merge-intent.json")
			_, err := settleEnvironmentPublication(intent, expected, read, merge)
			pass := mode == "success" || mode == "lost-receipt" || mode == "already-merged"
			if (err == nil) != pass {
				t.Fatalf("outcome %s: %v", mode, err)
			}
			wantWrites := 0
			if mode == "success" || mode == "lost-receipt" || mode == "unknown" {
				wantWrites = 1
			}
			if writes != wantWrites {
				t.Fatal("unexpected external mutation", writes)
			}
			// The same intent is reconcile-only even when a lost write still looks open.
			_, _ = settleEnvironmentPublication(intent, expected, read, merge)
			if writes != wantWrites {
				t.Fatal("external merge was repeated")
			}
			after, _ := json.Marshal(expected)
			if string(before) != string(after) {
				t.Fatal("sealed Agent publication report changed")
			}
		})
	}
}
