package app

import (
	"context"
	"encoding/json"
	"errors"

	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// roadmapTaskCommands adapts the Task transaction and its durable receipt to
// the Roadmap's original request identity.
type roadmapTaskCommands struct {
	commands taskapp.Commands
	store    taskStorage
	execute  func(context.Context, func(context.Context) error) error
}

func newRoadmapTaskCommands(store taskStorage, config Config, execute func(context.Context, func(context.Context) error) error) roadmapTaskCommands {
	return roadmapTaskCommands{commands: NewTaskCommands(store, config), store: store, execute: execute}
}

func roadmapTaskRequestID(requestID string) string {
	return taskPayloadDigest([]byte("roadmap-admission\n" + requestID))[:32]
}

func (a roadmapTaskCommands) submit(ctx context.Context, sourceID, requestID string, batch taskmodel.TaskBatchInput) (taskmodel.TaskBatchReceipt, error) {
	graph, err := a.store.TaskGraphSnapshot(ctx, a.commands.Input.Project)
	if err != nil {
		return taskmodel.TaskBatchReceipt{}, err
	}
	// The actor can finish after the caller's context is cancelled.
	receipts := make(chan taskmodel.TaskBatchReceipt, 1)
	err = a.execute(ctx, func(work context.Context) error {
		receipt, err := a.commands.AdmitRoadmap(work, sourceID, batch, roadmapTaskRequestID(requestID), graph.Authority.AuthorityVersion, "local-user")
		if err == nil {
			receipts <- receipt
		}
		return err
	})
	if err != nil {
		return taskmodel.TaskBatchReceipt{}, err
	}
	return <-receipts, nil
}

func (a roadmapTaskCommands) receipt(ctx context.Context, requestID string) (taskmodel.TaskBatchReceipt, error) {
	var receipt taskmodel.TaskBatchReceipt
	id := roadmapTaskRequestID(requestID)
	raw, err := a.commands.ReadReceipt(ctx, id, "create_task_batch", a.commands.Input.Project)
	if err != nil {
		return receipt, err
	}
	if err = json.Unmarshal(raw, &receipt); err != nil {
		return receipt, err
	}
	if receipt.Status != "accepted" || receipt.RequestID != id || len(receipt.TaskIDs) == 0 {
		return receipt, errors.New("原入图回执未核实")
	}
	return receipt, nil
}
