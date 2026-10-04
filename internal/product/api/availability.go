package api

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// CandidateFreshness is a display/input guard, not center authorization. The
// command service still rechecks the fixed identity, current generation and version.
func CandidateFreshness(identity string, snapshot ProjectSnapshot, now time.Time) (bool, string) {
	if snapshot.Tasks == nil {
		if snapshot.TaskError != "" {
			return false, snapshot.TaskError
		}
		return false, "任务候选快照不可用"
	}
	if snapshot.TaskError != "" {
		return false, snapshot.TaskError
	}
	if snapshot.Connection.State != ConnectionConnected || !snapshot.Connection.CanControl {
		return false, connectionDiagnostic(snapshot.Connection.Diagnostic)
	}
	controlRuntime := snapshot.Connection.Snapshot.Page
	if controlRuntime == nil || controlRuntime.Service.InstanceID == "" {
		return false, "缺少当前服务实例身份"
	}
	if snapshot.Tasks.InstanceID != controlRuntime.Service.InstanceID || canonicalIdentity(snapshot.Tasks.RepositoryIdentity) != canonicalIdentity(identity) || canonicalIdentity(controlRuntime.Repository.Identity) != canonicalIdentity(identity) {
		return false, "候选缓存属于其他服务实例或仓库"
	}
	if snapshot.Tasks.LastSuccessAt == nil || snapshot.Tasks.LastSuccessAt.IsZero() {
		return false, "候选查询尚无成功记录"
	}
	if snapshot.Tasks.QueriedAt.After(now.Add(time.Minute)) || snapshot.Tasks.LastSuccessAt.After(now.Add(time.Minute)) {
		return false, "候选快照时间位于未来"
	}
	limit := snapshot.CandidateFreshnessLimit
	if limit < 30*time.Second {
		limit = 30 * time.Second
	}
	if now.Sub(snapshot.Tasks.LastSuccessAt.UTC()) > limit {
		return false, fmt.Sprintf("候选缓存已过期（上次成功 %s）", snapshot.Tasks.LastSuccessAt.Local().Format("15:04:05"))
	}
	return true, ""
}

func canonicalIdentity(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func connectionDiagnostic(value string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return "服务未连接；缓存任务不能作为启动许可"
}

const ExecutionConfigChanged = "服务与控制台加载的执行配置不同；重开控制台并核对服务配置"

func ExecutionConfigMismatch(ref ProjectRef, snapshot ProjectSnapshot) bool {
	rt := snapshot.Connection.Snapshot.Page
	return rt != nil && ref.Presentation.ConfigDigest != "" && rt.Service.ConfigDigest != ref.Presentation.ConfigDigest
}

// StartTaskReason is the sole product input guard, never command authorization.
// The center still rechecks identity, versions and actual task eligibility.
func StartTaskReason(ref ProjectRef, snapshot ProjectSnapshot, now time.Time) string {
	if valid, reason := CandidateFreshness(ref.Identity, snapshot, now); !valid {
		return reason
	}
	if ExecutionConfigMismatch(ref, snapshot) {
		return ExecutionConfigChanged
	}
	if ref.Conflict != "" {
		return ref.Conflict
	}
	rt := snapshot.Connection.Snapshot.Page
	if rt == nil {
		return "缺少当前服务快照"
	}
	if rt.Control.DispatchMode != "manual" {
		return "开始任务要求手动派发模式"
	}
	if rt.Control.DispatchPaused {
		return "派发已暂停，请先恢复派发"
	}
	for _, capability := range rt.Service.Capabilities {
		if capability == "start_task" {
			return ""
		}
	}
	return "服务未声明 start_task 能力"
}

func AutoDispatchReason(ref ProjectRef, snapshot ProjectSnapshot) string {
	if ref.Presentation.DefaultProfileID == "" {
		return "自动派发需要在全局配置中指定 default_execution_profile"
	}
	if ExecutionConfigMismatch(ref, snapshot) {
		return "不能切换自动派发：" + ExecutionConfigChanged
	}
	return ""
}
