package orcaworker

import (
	"context"
	"encoding/json"
	"time"
)

// A delayed start can become observable after worker-start's receipt is saved.
// Only fresh activity for the original Dispatch and process proves that start;
// input acceptance, terminal output and a live but idle process do not.
func ObserveWorkerStarted(ctx context.Context, call MetaCall, expected DispatchIdentity, intentAt time.Time) (bool, error) {
	envelope, err := call(ctx, "orchestration", "worker-show", "--dispatch", expected.DispatchID, "--json")
	if err != nil {
		return false, err
	}
	var result struct {
		Dispatch struct {
			ID     string `json:"id"`
			TaskID string `json:"taskId"`
			RunID  string `json:"runId"`
		} `json:"dispatch"`
		Worker struct {
			DispatchID    string `json:"dispatchId"`
			AgentTerminal string `json:"agentTerminalHandle"`
		} `json:"worker"`
		Terminal    Terminal `json:"terminal"`
		Observation struct {
			ExactWorker bool `json:"exactWorker"`
		} `json:"observation"`
		Projection struct {
			DispatchID string `json:"dispatchId"`
			Stage      struct {
				Activity string `json:"activity"`
			} `json:"stage"`
			Liveness struct {
				Verdict    string `json:"verdict"`
				Source     string `json:"source"`
				ObservedAt int64  `json:"observedAt"`
			} `json:"liveness"`
			Evidence struct {
				LiveStatus string `json:"liveStatus"`
			} `json:"evidence"`
		} `json:"projection"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil {
		return false, err
	}
	t := result.Terminal
	if envelope.RuntimeID != expected.RuntimeID || result.Dispatch.ID != expected.DispatchID || result.Dispatch.TaskID != expected.TaskID || result.Dispatch.RunID != expected.RunID || result.Worker.DispatchID != expected.DispatchID || result.Worker.AgentTerminal != expected.Terminal || t.Handle != expected.Terminal || t.IncarnationID != expected.Incarnation || t.WorktreeID != expected.WorktreeID {
		return false, ErrDispatchIdentity
	}
	p := result.Projection
	return !intentAt.IsZero() && result.Observation.ExactWorker && t.Connected && !t.Orphaned && !t.Exited() &&
		p.DispatchID == expected.DispatchID && p.Stage.Activity == "working" &&
		p.Liveness.Verdict == "live" && p.Liveness.Source == "agent_status" &&
		p.Evidence.LiveStatus == "fresh" && p.Liveness.ObservedAt >= intentAt.UnixMilli(), nil
}
