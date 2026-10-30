//go:build e2e

package testenv

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
)

type environmentStatus struct{ Context, State, Status string }
type environmentReview struct {
	State     string
	Dismissed bool
	User      struct{ Login string }
}

func requireEnvironmentChecks(required []string, statuses []environmentStatus) error {
	for _, name := range required {
		passed := false
		for _, status := range statuses {
			if status.Context == name {
				passed = status.State == "success" || status.Status == "success"
				break
			}
		}
		if !passed {
			return fmt.Errorf("required PR check not passed: %s", name)
		}
	}
	return nil
}

func requireEnvironmentReviews(minimum int, author string, reviews []environmentReview) error {
	if len(reviews) >= 100 {
		return errors.New("PR review evidence may be truncated")
	}
	latest := map[string]bool{}
	for _, review := range reviews {
		if review.User.Login != "" && review.User.Login != author {
			switch strings.ToUpper(review.State) {
			case "APPROVED":
				latest[review.User.Login] = !review.Dismissed
			case "CHANGES_REQUESTED", "REQUEST_CHANGES", "DISMISSED":
				latest[review.User.Login] = false
			}
		}
	}
	count := 0
	for _, approved := range latest {
		if approved {
			count++
		}
	}
	if count < minimum {
		return errors.New("required independent PR approvals missing")
	}
	return nil
}

// L4 owns this external merge after independently verifying the original L2,
// project rules and approvals. The sealed Agent report remains unchanged.
func settleEnvironmentPublication(intentPath string, expected env.PR, read func() (environmentPR, error), merge func() error) (environmentPR, error) {
	current, err := read()
	if err != nil {
		return current, err
	}
	match := func(pr environmentPR) bool {
		return expected.URL != "" && pr.URL == expected.URL && pr.Base.Ref == expected.Base && pr.Head.Ref == expected.Head && pr.Head.SHA == expected.HeadRevision
	}
	if !match(current) {
		return current, errors.New("external PR identity differs from sealed publication")
	}
	if current.Merged {
		if !regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`).MatchString(current.Merge) {
			return current, errors.New("external merge commit missing")
		}
		return current, nil
	}
	if current.State != "open" {
		return current, errors.New("original validation PR is not open")
	}
	body, err := json.Marshal(expected)
	if err != nil {
		return current, err
	}
	f, err := os.OpenFile(intentPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		// An existing intent may already have reached the provider. Never issue a
		// second mutation, even when the observed PR still appears open.
		return current, fmt.Errorf("original merge intent already exists or cannot be saved; reconcile only: %w", err)
	}
	_, writeErr := f.Write(body)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		return current, errors.Join(writeErr, closeErr)
	}
	mergeErr := merge()
	current, err = read()
	if err != nil {
		return current, fmt.Errorf("original merge outcome unknown; no retry: %w", err)
	}
	if !match(current) || !current.Merged || !regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`).MatchString(current.Merge) {
		return current, fmt.Errorf("original PR merge not verified; no retry: %v", mergeErr)
	}
	return current, nil
}
