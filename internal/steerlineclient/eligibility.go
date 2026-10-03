package steerlineclient

import (
	"fmt"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// CandidateFreshness is a display/input guard, not center authorization. The
// command service still rechecks the fixed identity, current generation and version.
func CandidateFreshness(identity string, snapshot api.ProjectSnapshot, now time.Time) (bool, string) {
	if snapshot.Tasks == nil {
		if snapshot.TaskError != "" {
			return false, snapshot.TaskError
		}
		return false, "任务候选快照不可用"
	}
	if snapshot.TaskError != "" {
		return false, snapshot.TaskError
	}
	if snapshot.Connection.State != api.ConnectionConnected || !snapshot.Connection.CanControl {
		return false, connectionDiagnostic(snapshot.Connection.Diagnostic)
	}
	controlRuntime := snapshot.Connection.Snapshot.Runtime
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
