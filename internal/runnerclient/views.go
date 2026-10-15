package runnerclient

import (
	"context"
	"encoding/json"
	"time"
)

// These values are the public CLI contract, owned by the caller. There is no
// Go module dependency on the independently versioned Runner implementation.
type PRFact struct {
	URL            string     `json:"url"`
	Provider       string     `json:"provider"`
	Repository     string     `json:"repository"`
	Number         int        `json:"number"`
	BaseBranch     string     `json:"base_branch"`
	HeadRepository string     `json:"head_repository"`
	HeadBranch     string     `json:"head_branch"`
	HeadRevision   string     `json:"head_revision,omitempty"`
	State          string     `json:"state"`
	Merged         bool       `json:"merged"`
	MergedAt       *time.Time `json:"merged_at,omitempty"`
	MergeCommit    *string    `json:"merge_commit,omitempty"`
	CheckedAt      time.Time  `json:"checked_at"`
	QueryError     string     `json:"query_error,omitempty"`
}
type Final struct {
	CommandID     string `json:"command_id"`
	AttemptID     string `json:"attempt_id"`
	SessionID     string `json:"session_id"`
	TurnID        string `json:"turn_id"`
	NativeOutcome string `json:"native_outcome"`
	Verified      bool   `json:"verified"`
	Source        string `json:"source,omitempty"`
	ItemID        string `json:"item_id,omitempty"`
	Text          string `json:"text,omitempty"`
	Diagnostic    string `json:"diagnostic,omitempty"`
}
type Execution struct {
	WorkspaceScope string `json:"workspace_scope,omitempty"`
	TurnStarted    bool   `json:"turn_started"`
	AttemptID      string `json:"attempt_id"`
	Generation     string `json:"generation"`
	Backend        string `json:"backend"`
	Protocol       string `json:"protocol"`
	SessionID      string `json:"session_id"`
	TurnID         string `json:"turn_id"`
	NativeOutcome  string `json:"native_outcome"`
	FinalAvailable bool   `json:"final_available"`
	Workspace      string `json:"workspace"`
	Branch         string `json:"branch"`
}
type Question struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Options []struct {
		ID     string `json:"id"`
		Label  string `json:"label"`
		Detail string `json:"detail"`
	} `json:"options"`
	AllowFreeText   bool       `json:"allow_free_text"`
	NativeQuestions []Question `json:"native_questions"`
}
type Controls struct {
	CanReply  bool   `json:"can_reply"`
	CanResume bool   `json:"can_resume"`
	CanStop   bool   `json:"can_stop"`
	Reason    string `json:"reason"`
}
type Run struct {
	RunID            string    `json:"run_id"`
	Revision         uint64    `json:"revision"`
	Title            string    `json:"title"`
	InputDigest      string    `json:"input_digest"`
	ResourceDigest   string    `json:"resource_digest"`
	Status           string    `json:"status"`
	Phase            string    `json:"phase"`
	Step             string    `json:"step"`
	Diagnostic       string    `json:"diagnostic"`
	NewInputsAllowed bool      `json:"new_inputs_allowed"`
	ResumeRequired   bool      `json:"resume_required"`
	StopStatus       string    `json:"stop_status"`
	Execution        Execution `json:"execution"`
	Question         *Question `json:"question"`
	Controls         Controls  `json:"controls"`
	PR               *PRFact   `json:"pr"`
	Final            *Final    `json:"final"`
	Summary          string    `json:"summary"`
	UpdatedAt        time.Time `json:"updated_at"`
}
type Command struct {
	CommandID     string  `json:"command_id"`
	RequestID     string  `json:"request_id"`
	RunID         *string `json:"run_id"`
	Command       string  `json:"command"`
	ReceiptStatus string  `json:"receipt_status"`
	ActionStatus  string  `json:"action_status"`
}
type Inspection struct {
	Command *Command `json:"command"`
	Run     *Run     `json:"run"`
}
type Event struct {
	EventID   string    `json:"event_id"`
	RunID     string    `json:"run_id"`
	Revision  uint64    `json:"revision"`
	Type      string    `json:"type"`
	CommandID string    `json:"command_id"`
	AttemptID string    `json:"attempt_id"`
	Final     *Final    `json:"final"`
	CreatedAt time.Time `json:"created_at"`
}
type EventPage struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
}

func (c Client) Inspect(ctx context.Context, selector, id string, refresh bool) (Inspection, error) {
	switch selector {
	case "run-id", "request-id", "command-id":
	default:
		return Inspection{}, &Failure{Code: "invalid_arguments", Message: "Invalid inspection selector", Exit: 2}
	}
	args := []string{"--" + selector, id}
	if refresh {
		if selector != "run-id" {
			return Inspection{}, &Failure{Code: "invalid_arguments", Message: "PR refresh requires Run identity", Exit: 2}
		}
		args = append(args, "--refresh-pr")
	}
	response, err := c.Call(ctx, "inspect", args, nil)
	if err != nil {
		return Inspection{}, err
	}
	var result Inspection
	if json.Unmarshal(response.Data, &result) != nil || result.Run != nil && (result.Run.RunID == "" || result.Run.Revision == 0) || selector == "run-id" && (result.Run == nil || result.Run.RunID != id) || selector == "request-id" && (result.Command == nil || result.Command.RequestID != id) || selector == "command-id" && (result.Command == nil || result.Command.CommandID != id) || result.Command != nil && result.Command.RunID != nil && (result.Run == nil || result.Run.RunID != *result.Command.RunID) {
		return Inspection{}, &Failure{Code: "runner_unavailable", Message: "Runner inspection identity could not be verified", Exit: 4}
	}
	return result, nil
}
func (c Client) Events(ctx context.Context, id, after string) (EventPage, error) {
	args := []string{"--run-id", id, "--limit", "100"}
	if after != "" {
		args = append(args, "--after", after)
	}
	response, err := c.Call(ctx, "events", args, nil)
	if err != nil {
		return EventPage{}, err
	}
	var result EventPage
	if json.Unmarshal(response.Data, &result) != nil || result.NextCursor == "" {
		return EventPage{}, &Failure{Code: "runner_unavailable", Message: "Runner event page is invalid", Exit: 4}
	}
	for _, event := range result.Events {
		if event.EventID == "" || event.RunID != id || event.Revision == 0 {
			return EventPage{}, &Failure{Code: "runner_unavailable", Message: "Runner event identity is invalid", Exit: 4}
		}
	}
	return result, nil
}
