package model

import (
	"fmt"
	"strings"
)

type BlockerCategory string
type BlockerCode string

const (
	Remote       BlockerCategory = "remote"
	Worktree     BlockerCategory = "worktree"
	TaskMaterial BlockerCategory = "task_material"
	TestPlan     BlockerCategory = "test_plan"
)

const (
	RemoteUnchecked          BlockerCode = "remote_unchecked"
	RemoteCheckFailed        BlockerCode = "remote_check_failed"
	RemoteObservationChanged BlockerCode = "remote_observation_changed"
	RemoteHistoryRewritten   BlockerCode = "remote_history_rewritten"
	WorktreeUnverified       BlockerCode = "worktree_unverified"
	WorktreeDirty            BlockerCode = "worktree_dirty"
	GitOperationInProgress   BlockerCode = "git_operation_in_progress"
	DetachedHEAD             BlockerCode = "detached_head"
	WrongBranch              BlockerCode = "wrong_branch"
	LocalBehindRemote        BlockerCode = "local_behind_remote"
	LocalAheadRemote         BlockerCode = "local_ahead_remote"
	LocalDiverged            BlockerCode = "local_diverged"
	TaskMaterialUnprepared   BlockerCode = "task_material_unprepared"
	TaskMaterialStale        BlockerCode = "task_material_stale"
	TaskMaterialInvalid      BlockerCode = "task_material_invalid"
	TaskMaterialUnverified   BlockerCode = "task_material_unverified"
	TestPlanMissing          BlockerCode = "test_plan_missing"
	TestPlanInvalid          BlockerCode = "test_plan_invalid"
	TestCatalogueInvalid     BlockerCode = "test_catalogue_invalid"
)

// Category and Code are stable identifiers. Diagnostic is for display only.
type Blocker struct {
	Category     BlockerCategory `json:"category"`
	Code         BlockerCode     `json:"code"`
	Diagnostic   string          `json:"diagnostic"`
	TaskID       string          `json:"task_id,omitempty"`
	Path         string          `json:"path,omitempty"`
	TargetBranch string          `json:"target_branch,omitempty"`
}

// Readiness is the project-wide admission result for new executions only.
type Readiness struct {
	Status   string    `json:"status"`
	Commit   string    `json:"commit"`
	Total    int       `json:"total"`
	Ready    int       `json:"ready"`
	Waiting  int       `json:"waiting"`
	Blockers []Blocker `json:"blockers"`
}

func (r Readiness) Diagnostic() string {
	messages := make([]string, 0, len(r.Blockers))
	for _, blocker := range r.Blockers {
		messages = append(messages, blocker.Diagnostic)
	}
	return strings.Join(messages, "；")
}

// MaterialsReadiness consumes one material for every unfinished task, including
// an unprepared placeholder when that task has no saved material.
func MaterialsReadiness(commit string, materials []Material) Readiness {
	r := Readiness{Status: "ready", Commit: commit, Total: len(materials), Blockers: []Blocker{}}
	for _, material := range materials {
		if material.Status == "waiting_tests" && material.Commit == commit && commit != "" {
			r.Waiting++
			continue
		}
		if material.Status == "ready" && material.Commit == commit && commit != "" && material.Digest != "" && len(material.TestPlan) > 0 {
			r.Ready++
		} else {
			r.Blockers = append(r.Blockers, materialBlocker(commit, material))
		}
	}
	if len(r.Blockers) > 0 {
		r.Status = "not_ready"
	}
	return r
}

func materialBlocker(commit string, material Material) Blocker {
	blocker := Blocker{Category: TaskMaterial, Code: TaskMaterialInvalid, TaskID: material.TaskID}
	switch {
	case material.Status == "unprepared":
		blocker.Code = TaskMaterialUnprepared
	case material.Commit != "" && commit != "" && material.Commit != commit:
		blocker.Code = TaskMaterialStale
	case material.Status == "unknown":
		blocker.Code = TaskMaterialUnverified
	case material.Failure != nil:
		blocker = *material.Failure
		blocker.TaskID = material.TaskID
	}
	if blocker.Path == "" {
		blocker.Path = material.Contract.PlanRef.Path
	}
	blocker.Diagnostic = fmt.Sprintf("任务 #%s 的材料与测试安排未就绪", material.TaskID)
	if material.Diagnostic != "" && blocker.Code != TaskMaterialStale {
		blocker.Diagnostic += "：" + material.Diagnostic
	}
	return blocker
}
