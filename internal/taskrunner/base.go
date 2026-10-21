package taskrunner

import (
	"context"
	"errors"
	"strings"

	"github.com/erranliu/SteerLine/internal/runnerbridge"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// BaseResolver acquires task input revisions. Runner alone creates execution
// workspaces; this adapter only fetches the task repository's fixed target ref.
type BaseResolver struct {
	Project string
	Store   interface {
		RunnerCandidates(context.Context, string, string) ([]runnerbridge.Candidate, error)
		TaskGraphSnapshot(context.Context, string) (storageports.TaskGraphSnapshot, error)
	}
	Git func(context.Context, ...string) ([]byte, error)
}

func (r BaseResolver) Resolve(ctx context.Context, work taskmodel.FixedWork, candidate bool) (string, error) {
	if r.Git == nil || r.Store == nil || work.Record.ProjectIdentity != r.Project {
		return "", errors.New("base resolution requires the fixed project repository")
	}
	graph, err := r.Store.TaskGraphSnapshot(ctx, r.Project)
	if err != nil {
		return "", err
	}
	if graph.Authority.AuthorityVersion != work.Authority.AuthorityVersion || graph.Authority.GraphRevision != work.Authority.GraphRevision {
		return "", failure("stale_version", "task graph changed before acquiring its base")
	}
	ref := "refs/remotes/" + work.Contract.Remote + "/" + work.Contract.Branch
	if _, err := r.Git(ctx, "fetch", "--no-tags", "--no-write-fetch-head", work.Contract.Remote, "+refs/heads/"+work.Contract.Branch+":"+ref); err != nil {
		return "", failure("task_not_ready", "cannot acquire the fixed target branch")
	}
	raw, err := r.Git(ctx, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	base := strings.TrimSpace(string(raw))

	if !taskmodel.ValidCommit(base) {
		return "", failure("base_missing_delivery", "target branch has no valid commit")
	}
	if _, err := r.Git(ctx, "merge-base", "--is-ancestor", base, ref); err != nil {
		return "", failure("base_missing_delivery", "fixed base is absent from the target branch")
	}
	for _, edge := range graph.Edges {
		if edge.SuccessorID != work.Record.TaskID {
			continue
		}
		pred, ok := graph.ByID[edge.PredecessorID]
		delivery, delivered := graph.Deliveries[edge.PredecessorID]
		if !ok || pred.Status != "closed" || !delivered || !taskmodel.ValidCommit(delivery.MergeCommit) {
			return "", failure("task_not_ready", "predecessor has no verified delivery")
		}
		if _, err := r.Git(ctx, "merge-base", "--is-ancestor", delivery.MergeCommit, base); err != nil {
			return "", failure("base_missing_delivery", "fixed base excludes a predecessor merge")
		}
	}
	return base, nil
}
