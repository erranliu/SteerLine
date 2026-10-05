package publication

import (
	"errors"
	"regexp"

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
	if url == "" || repository == "" || baseBranch == "" || executionBranch == "" ||
		info.URL != url || info.Repository != repository || info.BaseBranch != baseBranch ||
		info.HeadRepository != repository || info.HeadBranch != executionBranch ||
		!validCommit(info.HeadRevision) {
		return ErrPRIdentity
	}
	if info.State != "open" && info.State != "closed_unmerged" && info.State != "merged" {
		return ErrPRIdentity
	}
	return nil
}
