package publication

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

var ErrPRIdentity = errors.New("PR identity mismatch")

var commitHash = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func validCommit(value string) bool { return commitHash.MatchString(value) }

type Config executionmodel.PRProviderConfig

func (c Config) ValidateRemote(remote string) error {
	return executionmodel.PRProviderConfig(c).ValidateRemote(remote)
}

func (c Config) PRNumber(raw string) (int, error) {
	return executionmodel.PRProviderConfig(c).PRNumber(raw)
}

// ValidateWorkerPRIdentity binds a provider-confirmed PR to the execution's
// branch. A URL or title supplied by a worker is only a clue until this check
// succeeds. The head SHA deliberately remains free to advance as the worker
// pushes more commits to the same PR.
func ValidateWorkerPRIdentity(info executionmodel.PRInfo, url, repository, baseBranch, executionBranch string) error {
	var differences []string
	for _, field := range []struct{ name, expected, actual string }{
		{"url", url, info.URL},
		{"repository", repository, info.Repository},
		{"base_branch", baseBranch, info.BaseBranch},
		{"head_repository", repository, info.HeadRepository},
		{"head_branch", executionBranch, info.HeadBranch},
	} {
		if field.expected == "" || field.actual != field.expected {
			differences = append(differences, fmt.Sprintf("%s: expected=%q, actual=%q", field.name, field.expected, field.actual))
		}
	}
	if !validCommit(info.HeadRevision) {
		differences = append(differences, fmt.Sprintf("head_revision: expected=valid commit SHA, actual=%q", info.HeadRevision))
	}
	if info.State != "open" && info.State != "closed_unmerged" && info.State != "merged" {
		differences = append(differences, fmt.Sprintf("state: expected=open|closed_unmerged|merged, actual=%q", info.State))
	}
	if len(differences) > 0 {
		return fmt.Errorf("%w: %s", ErrPRIdentity, strings.Join(differences, "; "))
	}
	return nil
}

// ValidateWorkerPublication also binds the published revision and state.
func ValidateWorkerPublication(info executionmodel.PRInfo, url, repository, baseBranch, executionBranch, candidateSHA string) error {
	var differences []string
	if err := ValidateWorkerPRIdentity(info, url, repository, baseBranch, executionBranch); err != nil {
		differences = append(differences, strings.TrimPrefix(err.Error(), ErrPRIdentity.Error()+": "))
	}
	if info.HeadRevision != candidateSHA {
		differences = append(differences, fmt.Sprintf("head_revision: expected=%q, actual=%q", candidateSHA, info.HeadRevision))
	}
	if info.State != "open" && info.State != "merged" {
		differences = append(differences, fmt.Sprintf("state: expected=open|merged, actual=%q", info.State))
	}
	if len(differences) > 0 {
		return fmt.Errorf("%w: %s", ErrPRIdentity, strings.Join(differences, "; "))
	}
	return nil
}
