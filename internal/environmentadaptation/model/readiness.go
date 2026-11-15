package model

import (
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// Context is solely a preparation projection. Never use it to construct a Run.
func Context(execution runnercontract.ExecutionInput, identity runnercontract.ExecutionIdentity) (runnercontract.ExecutionInput, error) {
	if (execution.Adapter != "codex-app-server" && execution.Adapter != "orca-terminal") || execution.Target.Kind != "local" && execution.Target.Kind != "orca_environment" || execution.Target.Kind == "local" && execution.Target.Environment != "" || execution.Target.Kind == "orca_environment" && (execution.Adapter != "orca-terminal" || execution.Target.Environment == "") {
		return runnercontract.ExecutionInput{}, errors.New("unsupported project environment target")
	}
	if err := runnercontract.ValidateExecutionIdentity(execution, identity); err != nil {
		return runnercontract.ExecutionInput{}, err
	}
	return identity.Context, nil
}
func Key(execution runnercontract.ExecutionInput, identity runnercontract.ExecutionIdentity) (string, error) {
	value, err := Context(execution, identity)
	if err != nil {
		return "", err
	}
	return runnercontract.ConfigurationValueDigest(value), nil
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

type QualificationError struct{ Reason string }

func (e *QualificationError) Error() string { return "环境未就绪：" + e.Reason }

// Qualify is shared by preflight, queries and the candidate transaction.
func Qualify(current Current, record Record, basis string) (Admission, error) {
	fail := func(reason string) (Admission, error) { return Admission{}, &QualificationError{Reason: reason} }
	if current.Revision == 0 {
		return fail("尚未准备当前项目环境")
	}
	if current.Invalidated {
		return fail("已标记失效：" + current.Reason)
	}
	key, err := Key(record.Execution, record.Prepared.Identity)
	if err != nil || current.Project == "" || current.Project != record.Project || current.Key != key || current.ActivityID == "" || current.ActivityID != record.ID || record.Kind != "adaptation" {
		return fail("当前准备引用与原活动不符")
	}
	if record.Status() != "ready" {
		return fail("当前项目检测未就绪或原执行尚未结算")
	}
	if basis == "" || record.Prepared.Inputs != basis {
		return fail("准备依据已变化，须重新检测")
	}
	return Admission{Key: current.Key, Revision: current.Revision, ActivityID: record.ID, Basis: basis}, nil
}

func View(current Current, record Record, basis string, materialError error) (status, stage, reason string) {
	status = "not_ready"
	switch {
	case record.Closing():
		reason, _ = record.ClosingStatus()
		return status, "closing", reason
	case current.Invalidated:
		return status, "stale", current.Reason
	case record.ID != "" && !record.Settled():
		reason = record.DiagnosticText()
		if strings.TrimSpace(reason) == "" {
			reason = "等待本次检测结果"
		}
		return status, "verifying", reason
	case current.ActivityID == "":
		if materialError != nil {
			return status, "unprepared", materialError.Error()
		}
		return status, "awaiting_user", "请按项目要求准备环境并开始检测"
	case record.Status() != "ready":
		if record.ID == "" {
			return status, "blocked", "无法读取当前检测记录"
		}
		return status, record.DetectionStage(), record.DiagnosticText()
	case materialError != nil:
		return status, "blocked", materialError.Error()
	default:
		if _, err := Qualify(current, record, basis); err != nil {
			return status, "stale", err.Error()
		}
		return "ready", "ready", "项目报告环境就绪，原调用已结算"
	}
}
