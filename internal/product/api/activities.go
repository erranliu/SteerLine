package api

import "time"

// SteerAgentClassification describes content origin, not execution authority.
type SteerAgentClassification struct {
	Type      string `json:"type"`
	Operation string `json:"operation"`
}

// AsyncActivity is an observation, never an execution command or a Task.
type AsyncActivity struct {
	ID                 string                    `json:"activity_id"`
	ProjectID          string                    `json:"project_id"`
	Source             string                    `json:"source"`
	SourceID           string                    `json:"source_id"`
	Revision           uint64                    `json:"source_revision,omitempty"`
	Kind               string                    `json:"kind"`
	SteerAgent         *SteerAgentClassification `json:"steer_agent,omitempty"`
	RequestIDs         []string                  `json:"request_ids"`
	Title              string                    `json:"title"`
	State              string                    `json:"state"`
	Phase              string                    `json:"phase,omitempty"`
	Summary            string                    `json:"summary,omitempty"`
	NeedsAttention     bool                      `json:"needs_attention"`
	Diagnostic         string                    `json:"diagnostic,omitempty"`
	CreatedAt          *time.Time                `json:"created_at,omitempty"`
	BusinessFinishedAt *time.Time                `json:"business_finished_at,omitempty"`
	FinishedAt         *time.Time                `json:"finished_at,omitempty"`
	ObservedAt         *time.Time                `json:"source_observed_at,omitempty"`
	Observation        string                    `json:"observation"`
	Progress           *AsyncActivityProgress    `json:"progress,omitempty"`
	Details            AsyncActivityDetails      `json:"details_ref"`
	Execution          ExecutionReference        `json:"execution_ref"`
}

// ExecutionReference contains only public identities for locating original work.
type ExecutionReference struct {
	RequestID string `json:"request_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	AttemptID string `json:"attempt_id,omitempty"`
	Action    string `json:"action,omitempty"`
}
type AsyncActivityProgress struct {
	Label      string `json:"label"`
	Total      int    `json:"total"`
	Succeeded  int    `json:"succeeded"`
	Failed     int    `json:"failed"`
	Pending    int    `json:"pending"`
	Diagnostic string `json:"diagnostic,omitempty"`
}
type AsyncActivityDetails struct {
	CandidateID string `json:"candidate_id,omitempty"`
	Kind        string `json:"kind"`
	Target      string `json:"target"`
	TaskID      string `json:"task_id,omitempty"`
	Historical  bool   `json:"historical,omitempty"`
}
type AsyncActivitySource struct {
	Source     string `json:"source"`
	Complete   bool   `json:"complete"`
	Diagnostic string `json:"diagnostic,omitempty"`
}
type AsyncActivities struct {
	ProjectID   string                   `json:"project_id"`
	InstanceID  string                   `json:"instance_id"`
	SampledAt   time.Time                `json:"sampled_at"`
	Items       []AsyncActivity          `json:"items"`
	Links       map[string]AsyncActivity `json:"request_links"`
	Sources     []AsyncActivitySource    `json:"sources"`
	Counts      map[string]int           `json:"counts"`
	CountsKnown bool                     `json:"counts_known"`
	NextCursor  string                   `json:"next_cursor,omitempty"`
	HasMore     bool                     `json:"has_more"`
}

// Association-only reads carry no list, counts or pagination semantics.
type AsyncActivityLinks struct {
	ProjectID  string                   `json:"project_id"`
	InstanceID string                   `json:"instance_id"`
	SampledAt  time.Time                `json:"sampled_at"`
	Links      map[string]AsyncActivity `json:"request_links"`
	Sources    []AsyncActivitySource    `json:"sources"`
}
