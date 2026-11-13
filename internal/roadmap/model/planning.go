package model

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

const SingleTaskPlanning = "task"

var ErrPlanningChanged = errors.New("规划任务已变化，请刷新")
var ErrPlanningPathOccupied = errors.New("同一路径已有活动规划任务")

func TaskCardPath(id string) string { return "SteerlineFlow/task-planning/" + id + ".json" }

type TaskCard struct {
	Goal          string   `json:"goal"`
	Scope         string   `json:"scope"`
	Constraints   string   `json:"constraints"`
	Acceptance    string   `json:"acceptance"`
	Predecessors  []string `json:"predecessors"`
	ProvidesTests []string `json:"provides_tests"`
}

type PlanningTask struct {
	ID            string                  `json:"id"`
	Project       string                  `json:"project"`
	Title         string                  `json:"title"`
	Goal          string                  `json:"goal"`
	Kind          string                  `json:"kind,omitempty"`
	Path          string                  `json:"path"`
	Base          string                  `json:"base"`
	Revision      uint64                  `json:"revision"`
	Status        string                  `json:"status"`
	StopRequested bool                    `json:"stop_requested,omitempty"`
	ModelSlot     bool                    `json:"model_slot,omitempty"`
	Diagnostic    string                  `json:"diagnostic,omitempty"`
	Body          string                  `json:"body,omitempty"`
	SourceID      string                  `json:"source_id,omitempty"`
	Run           *runnercontract.Run     `json:"run,omitempty"`
	Build         runnercontract.BuildPin `json:"build"`
	Submission    runnercontract.Request  `json:"submission"`
	Material      string                  `json:"material"`
	MaterialSHA   string                  `json:"material_sha"`
	Operation     *PlanningOperation      `json:"operation,omitempty"`
	Conflict      *PlanningConflict       `json:"conflict,omitempty"`
	Review        *PlanningReview         `json:"review,omitempty"`
	Publication   *PlanningPublication    `json:"publication,omitempty"`
	Merged        string                  `json:"merged,omitempty"`
}

type PlanningOperation struct {
	QueuedAt   time.Time               `json:"queued_at,omitempty"`
	Kind       string                  `json:"kind"`
	Stage      string                  `json:"stage"`
	Target     string                  `json:"target,omitempty"`
	Command    string                  `json:"command,omitempty"`
	Request    *runnercontract.Request `json:"request,omitempty"`
	Answer     *runnercontract.Request `json:"answer,omitempty"`
	Lease      string                  `json:"lease,omitempty"`
	Checkpoint string                  `json:"checkpoint,omitempty"`
	Previous   string                  `json:"previous,omitempty"`
	Result     string                  `json:"result,omitempty"`
	Selected   []string                `json:"selected,omitempty"`
	Digest     string                  `json:"digest,omitempty"`
	Diagnostic string                  `json:"diagnostic,omitempty"`
	Writer     json.RawMessage         `json:"writer,omitempty"`
	IndexTree  string                  `json:"index_tree,omitempty"`
}

type PlanningConflict struct {
	Target     string   `json:"target"`
	Previous   string   `json:"previous"`
	Checkpoint string   `json:"checkpoint"`
	IndexTree  string   `json:"index_tree"`
	Files      []string `json:"files"`
}

type PlanningReview struct {
	Digest  string           `json:"digest"`
	Changes []PlanningChange `json:"changes"`
}

type PlanningChange struct {
	Path        string `json:"path"`
	Before      []byte `json:"before,omitempty"`
	After       []byte `json:"after,omitempty"`
	OldMode     string `json:"old_mode,omitempty"`
	Mode        string `json:"mode,omitempty"`
	Unavailable string `json:"unavailable,omitempty"`
}

type PlanningPublication struct {
	Branch   string            `json:"branch"`
	URL      string            `json:"url,omitempty"`
	Head     string            `json:"head,omitempty"`
	Versions []PlanningVersion `json:"versions"`
}

type PlanningVersion struct {
	Base    string           `json:"base"`
	Head    string           `json:"head"`
	Changes []PlanningChange `json:"changes"`
}
