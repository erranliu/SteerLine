package query

import (
	"encoding/json"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// ProjectPage builds the client page from one sampled storage ProjectView. All sections
// in ProjectView share its pinned read transaction; this function only maps
// sampled values and never opens storage or reconstructs actor state.
func ProjectPage(view storageports.ProjectView, selected string, freshness time.Duration, observation api.ObservationSnapshot, connection api.ServiceConnection, decisions map[string]string, executableTypes []string) api.ProjectSnapshot {
	page := api.ProjectSnapshot{SelectedTaskID: selected, CandidateFreshnessLimit: freshness, Observation: observation, Connection: connection, DecisionReasons: cloneStrings(decisions)}
	if view.Tasks != nil {
		native := TaskPage(*view.Tasks)
		page.NativeTasks = &native
	}
	if view.SelectedTask != nil {
		selected := TaskDetail(*view.SelectedTask)
		page.SelectedNative = &selected
	}
	if view.Ready != nil {
		page.Tasks = ReadyCandidates(*view.Ready, observation.SampledAt, connection, executableTypes)
	}
	if view.TaskError != nil {
		page.TaskError = view.TaskError.Error()
		if view.Tasks == nil {
			page.NativeTaskError = page.TaskError
		}
	}
	if view.SelectedTaskError != nil {
		page.NativeTaskError = view.SelectedTaskError.Error()
	}
	if view.Candidates != nil && view.HistoryError == nil {
		history := RunnerHistory(view.Candidates)
		page.History = &history
	}
	if view.HistoryError != nil {
		page.HistoryError = view.HistoryError.Error()
	}
	if page.TaskError == "" && page.NativeTaskError == "" && page.HistoryError == "" && page.Observation.Error == "" {
		page.FullReadAt = view.SampledAt
	}
	return page
}

// ReadyCandidates renders the exact read order. It retains cached candidates
// when control is offline, leaving start eligibility to the product input rule.
func ReadyCandidates(source storageports.NativeReadyTaskSnapshot, sampled time.Time, connection api.ServiceConnection, executableTypes []string) *api.TaskCandidates {
	last := source.SampledAt
	if last.IsZero() {
		last = sampled
	}
	p := &api.TaskCandidates{Version: 1, RepositoryIdentity: source.ProjectID, QueriedAt: sampled, LastSuccessAt: &last, Tasks: []api.TaskCandidate{}}
	if connection.State == api.ConnectionConnected && connection.CanControl && connection.Snapshot.Page != nil {
		p.InstanceID = connection.Snapshot.Page.Service.InstanceID
	}
	for _, t := range source.Tasks {
		if t.Status != "open" || !allowedType(t.Type, executableTypes) {
			continue
		}
		p.Tasks = append(p.Tasks, api.TaskCandidate{TaskID: t.TaskID, Title: t.Title, Type: t.Type})
	}
	return p
}

func allowedType(value string, accepted []string) bool {
	if accepted == nil {
		return true
	}
	for _, item := range accepted {
		if item == value {
			return true
		}
	}
	return false
}

func CommandReceipt(r storageports.CommandRecord) api.CommandReceipt {
	if r.Kind == "runner_discard" {
		r.Kind = "discard_task"
	}
	p := api.CommandReceipt{ProjectIdentity: r.ProjectIdentity, CommandID: r.CommandID, Kind: r.Kind, TargetID: r.TargetID, ExpectedVersion: r.ExpectedVersion, RequestedBy: r.RequestedBy, Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	switch r.Kind {
	case "answer_decision":
		var answer api.DecisionAnswer
		if json.Unmarshal(r.Payload, &answer) == nil {
			p.DecisionAnswer = &answer
		}
	case "discard_task":
		var request api.DiscardInput
		if json.Unmarshal(r.Payload, &request) == nil {
			request.RequestID = r.CommandID
			p.DiscardRequest = &request
		}
	}

	if len(r.Result) > 0 {
		var result struct {
			Code  string `json:"code"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(r.Result, &result); err == nil {
			p.ResultCode, p.ResultError = result.Code, result.Error
		}
		if r.Kind == "discard_task" {
			var discard api.TaskDiscardReceipt
			if err := json.Unmarshal(r.Result, &discard); err == nil && discard.RequestID == r.CommandID {
				p.Discard = &discard
			}
		}
	}
	return p
}

func cloneStrings(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	cloned := make(map[string]string, len(src))
	for key, value := range src {
		cloned[key] = value
	}
	return cloned
}
