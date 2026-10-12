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
	if info.State != "open" && info.State != "closed_unmerged" && info.State != "merged" {
		differences = append(differences, fmt.Sprintf("state: expected=open|closed_unmerged|merged, actual=%q", info.State))
	}
	if len(differences) > 0 {
		return fmt.Errorf("%w: %s", ErrPRIdentity, strings.Join(differences, "; "))
	}
	return nil
}
