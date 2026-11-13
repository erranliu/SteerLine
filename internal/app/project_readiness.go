package app

import (
	"context"
	"errors"
	"strings"

	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type maintenanceState struct {
	Observation pm.Observation
	Local       localBaseline
	Materials   []pm.Material
	Readiness   pm.Readiness
}

// readyExecutionReserver rechecks readiness and saves under the original baseline lock.
type readyExecutionReserver interface {
	reserveReadyExecution(ctx context.Context, commit string, save func() error) error
}

// Both the UI and every new execution use this projection. It never fetches,
// prepares documents, generates tests or changes existing candidates.
func (m *projectMaintenance) readinessState(ctx context.Context) (maintenanceState, error) {
	state := maintenanceState{Observation: m.current(), Materials: []pm.Material{}}
	store, ok := m.store.(materialStore)
	if !ok {
		return state, errors.New("material store unavailable")
	}
	graph, err := m.store.TaskGraphSnapshot(ctx, m.config.Identity())
	if err != nil {
		return state, err
	}
	for _, task := range graph.Tasks {
		if task.Status != "open" && task.Status != "in_progress" {
			continue
		}
		material, err := store.Material(ctx, m.config.Identity(), task.TaskID)
		if errors.Is(err, storageports.ErrNotFound) {
			contract, contractErr := m.store.TaskContract(ctx, m.config.Identity(), task)
			if contractErr != nil {
				return state, contractErr
			}
			material = pm.Material{TaskID: task.TaskID, Contract: contract, Status: "unprepared", Diagnostic: "等待检查 Git 任务材料"}
		} else if err != nil {
			return state, err
		}
		if material.Commit != "" && (material.Commit != state.Observation.Commit || state.Observation.Diagnostic != "") {
			material.Status = "unknown"
		}
		waiting, err := m.waitingForPlannedTests(ctx, task, graph, material)
		if err != nil {
			return state, err
		}
		if waiting {
			material.Status = "waiting_tests"
			material.Diagnostic = "等待同批前置任务交付并登记测试：" + strings.Join(material.MissingTests, ", ")
		}
		state.Materials = append(state.Materials, material)
	}
	state.Local = m.localBaseline(ctx)
	state.Readiness = pm.MaterialsReadiness(state.Observation.Commit, state.Materials)
	var blockers []pm.Blocker
	add := func(category pm.BlockerCategory, code pm.BlockerCode, diagnostic string) {
		blockers = append(blockers, pm.Blocker{Category: category, Code: code, Diagnostic: diagnostic, TargetBranch: m.config.Branch})
	}
	remoteVerified := state.Observation.Commit != "" && state.Observation.Diagnostic == ""
	if state.Observation.Diagnostic != "" {
		add(pm.Remote, pm.RemoteCheckFailed, "远端检查失败："+state.Observation.Diagnostic)
	} else if state.Observation.Commit == "" {
		add(pm.Remote, pm.RemoteUnchecked, "远端尚未检查")
	}
	local := state.Local
	if local.ReasonCode == pm.RemoteHistoryRewritten {
		add(pm.Remote, local.ReasonCode, local.Reason)
	} else if local.ReasonCode != "" {
		add(pm.Worktree, local.ReasonCode, local.Reason)
	}
	if remoteVerified && local.Branch == m.config.Branch {
		switch local.Status {
		case "behind":
			add(pm.Worktree, pm.LocalBehindRemote, "本地尚未与目标分支同步")
		case "ahead":
			add(pm.Worktree, pm.LocalAheadRemote, "本地存在未发布提交，请人工处理后重新检查")
		case "diverged":
			add(pm.Worktree, pm.LocalDiverged, "本地与目标分支已分叉，请人工处理后重新检查")
		}
	}
	// A concurrent observation cannot turn an older projection into permission.
	if current := m.current(); current.Commit != state.Observation.Commit || current.Diagnostic != state.Observation.Diagnostic || current.Rewritten != state.Observation.Rewritten {
		add(pm.Remote, pm.RemoteObservationChanged, "远端观察已变化，请重新核实")
	}
	// Keep the baseline gate even when a read raced an observation or checkout.
	if len(blockers) == 0 && (local.Status != "synced" || local.Reason != "" || local.Commit != state.Observation.Commit) {
		add(pm.Worktree, pm.WorktreeUnverified, "本地基线待核实")
	}
	if len(blockers) > 0 {
		state.Readiness.Blockers = append(blockers, state.Readiness.Blockers...)
		state.Readiness.Status = "not_ready"
	}
	return state, nil
}

func (m *projectMaintenance) projectReadiness(ctx context.Context) (pm.Readiness, error) {
	state, err := m.readinessState(ctx)
	return state.Readiness, err
}

func requireProjectReadiness(readiness pm.Readiness) error {
	if readiness.Status == "ready" {
		return nil
	}
	return &taskmodel.Error{Code: "project_not_ready", Message: "项目未就绪：" + readiness.Diagnostic() + "。请前往项目状态处理"}
}

func (m *projectMaintenance) reserveReadyExecution(ctx context.Context, commit string, save func() error) error {
	readiness, err := m.projectReadiness(ctx)
	if err != nil {
		return err
	}
	if err = requireProjectReadiness(readiness); err != nil {
		return err
	}
	return m.reserveBaseline(commit, save)
}
