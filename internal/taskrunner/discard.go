package taskrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type DiscardStore interface {
	Store
	LookupCommand(context.Context, string, string) (ports.CommandRecord, bool, error)
	CommitRunnerDiscard(context.Context, ports.RunnerDiscard) (taskmodel.TaskDiscardReceipt, error)
}

// Discard verifies all known PRs through the public CLI. An unknown original
// submission/publication cannot be interpreted as proof that no PR exists.
func (s *Service) Discard(ctx context.Context, store DiscardStore, requestID string, intent taskmodel.DiscardIntent) (taskmodel.TaskDiscardReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := taskmodel.ValidateDiscardIntent(intent); err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	body, _ := json.Marshal(intent)
	if old, found, err := store.LookupCommand(ctx, s.Project, requestID); err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	} else if found {
		var receipt taskmodel.TaskDiscardReceipt
		if old.Kind != "runner_discard" || !bytes.Equal(old.Payload, body) || json.Unmarshal(old.Result, &receipt) != nil {
			return receipt, failure("idempotency_conflict", "discard request belongs to different original input")
		}
		return receipt, nil
	}
	graph, err := store.TaskGraphSnapshot(ctx, s.Project)
	if err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	task, ok := graph.ByID[intent.TaskID]
	if !ok {
		return taskmodel.TaskDiscardReceipt{}, failure("task_not_found", "task does not exist")
	}
	if task.Version != intent.ExpectedVersion {
		return taskmodel.TaskDiscardReceipt{}, failure("stale_version", "task changed before discard")
	}
	rows, err := store.RunnerCandidates(ctx, s.Project, intent.TaskID)
	if err != nil {
		return taskmodel.TaskDiscardReceipt{}, err
	}
	commit := ports.RunnerDiscard{Project: s.Project, RequestID: requestID, Intent: intent, AuthorityVersion: graph.Authority.AuthorityVersion, GraphRevision: graph.Authority.GraphRevision, CandidateRevisions: map[string]uint64{}, VerificationStarted: time.Now().UTC()}
	for _, candidate := range rows {
		if candidate.RunID != "" {
			candidate, _, err = s.Bridge.ObservePR(ctx, s.Project, candidate.ID)
			if err != nil {
				return taskmodel.TaskDiscardReceipt{}, err
			}
		}
		commit.CandidateRevisions[candidate.ID] = candidate.Revision
	}
	return store.CommitRunnerDiscard(ctx, commit)
}
