package model

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// Context is solely a preparation projection. Never use it to construct a Run.
func Context(execution runnercontract.ExecutionInput) (runnercontract.ExecutionInput, error) {
	if Target(execution) == "" {
		return execution, errors.New("unsupported environment execution target")
	}
	var settings map[string]json.RawMessage
	canonical, err := runnercontract.CanonicalJSON(execution.Settings)
	if err != nil || json.Unmarshal(canonical, &settings) != nil || settings == nil || len(settings) == 0 {
		return execution, errors.New("environment settings must be a nonempty object")
	}
	switch execution.Adapter {
	case "codex-app-server":
		delete(settings, "model")
		delete(settings, "reasoning_effort")
	case "orca-terminal":
		if raw, ok := settings["agent_launch"]; ok {
			var agent map[string]json.RawMessage
			if json.Unmarshal(raw, &agent) != nil || agent == nil {
				return execution, errors.New("invalid environment agent settings")
			}
			delete(agent, "model")
			delete(agent, "reasoning_effort")
			settings["agent_launch"], _ = json.Marshal(agent)
		}
	default:
		return execution, errors.New("unsupported environment adapter")
	}
	execution.Settings, err = json.Marshal(settings)
	return execution, err
}

func Key(execution runnercontract.ExecutionInput) (string, error) {
	context, err := Context(execution)
	if err != nil {
		return "", err
	}
	return ExecutionDigest(context), nil
}

// Current contains selection and revocation only. Record owns execution state.
type Current struct {
	Project     string    `json:"project"`
	Key         string    `json:"environment_key"`
	Revision    uint64    `json:"revision"`
	ActivityID  string    `json:"activity_id,omitempty"`
	Invalidated bool      `json:"invalidated"`
	Reason      string    `json:"reason,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Admission struct {
	Key        string `json:"environment_key"`
	Revision   uint64 `json:"current_revision"`
	ActivityID string `json:"activity_id"`
	Basis      string `json:"basis"`
}

// Qualify is shared by preflight, queries and the candidate transaction.
func Qualify(current Current, record Record, basis string) (Admission, error) {
	fail := func(reason string) (Admission, error) { return Admission{}, errors.New("环境未就绪：" + reason) }
	if current.Revision == 0 {
		return fail("尚未准备当前项目环境")
	}
	if current.Invalidated {
		return fail("已标记失效：" + current.Reason)
	}
	key, err := Key(record.Execution)
	if err != nil || current.Project == "" || current.Project != record.Project || current.Key != key || current.ActivityID == "" || current.ActivityID != record.ID || record.Kind != "adaptation" {
		return fail("当前准备引用与原活动不符")
	}
	if record.Status() != "ready" {
		return fail("当前验收未通过或原执行尚未结算")
	}
	if basis == "" || record.Prepared.Inputs != basis {
		return fail("准备依据已变化，须重新验收")
	}
	return Admission{Key: current.Key, Revision: current.Revision, ActivityID: record.ID, Basis: basis}, nil
}

func View(current Current, record Record, basis string, materialError error) (status, stage, reason string) {
	status = "not_ready"
	switch {
	case current.Invalidated:
		return status, "stale", current.Reason
	case record.ID != "" && !record.Settled():
		reason = record.Diagnostic
		if strings.TrimSpace(reason) == "" {
			reason = "原验收正在执行或停止待核实"
		}
		return status, "verifying", reason
	case current.ActivityID == "":
		if materialError != nil {
			return status, "unprepared", materialError.Error()
		}
		return status, "awaiting_user", "请按项目清单准备环境并开始验收"
	case record.Status() != "ready":
		return status, "blocked", "当前验收未通过：" + record.Diagnostic
	case materialError != nil:
		return status, "blocked", materialError.Error()
	default:
		if _, err := Qualify(current, record, basis); err != nil {
			return status, "stale", err.Error()
		}
		return "ready", "ready", "当前项目环境已通过验收"
	}
}
