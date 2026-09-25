package console

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/viewer"
)

// ProjectRef is the identity of a console view. ConfigPath and RunDir are both
// part of the key so two configs for one repository keep separate view state.
type ProjectRef struct {
	Key        string
	Name       string
	ConfigPath string
	Config     steerline.Config
	Identity   string
	RunDir     string
	Error      string
	Conflict   string
}

type ProjectSnapshot struct {
	Connection  steerline.ServiceConnection
	Observation steerline.ObservationSnapshot
	Tasks       *steerline.TaskSnapshot
	TaskError   string
	Domain      *steerline.DomainSnapshot
	DomainError string
}

type QuerySource interface {
	ReadProject(context.Context, ProjectRef) ProjectSnapshot
	ReadActivity(context.Context, ProjectRef, string, string, string, string, steerline.JournalCursor) (viewer.LogBatch, error)
	ReadConversation(context.Context, ProjectRef, steerline.ConversationFilter, string) (steerline.ConversationPage, error)
	ReadBody(context.Context, ProjectRef, steerline.ConversationBodyRef, int64, int) (steerline.ConversationBodyPage, error)
	ReadCapabilities(context.Context, ProjectRef, steerline.BackendKind, steerline.ExecutionProtocol, string, string, string, string, string) ([]steerline.ConversationCapabilityReport, error)
}

type ControlClient interface {
	Submit(context.Context, ProjectRef, string, any, string, string, *uint64) (steerline.ControlRequest, error)
	Receipt(context.Context, ProjectRef, string) (*steerline.ControlReceipt, error)
	Request(context.Context, ProjectRef, string) (*steerline.ControlRequest, error)
}

type FileQuerySource struct{}

func (FileQuerySource) ReadProject(ctx context.Context, ref ProjectRef) ProjectSnapshot {
	result := ProjectSnapshot{}
	if ref.Error != "" {
		result.TaskError, result.DomainError = ref.Error, ref.Error
		return result
	}
	c := ref.Config
	if err := c.ResolveIdentity(ctx); err != nil {
		result.TaskError = "resolve repository identity: " + err.Error()
		result.Connection = steerline.ServiceConnection{State: steerline.ConnectionIdentityError, Diagnostic: result.TaskError}
		return result
	}
	result.Connection = steerline.ConnectService(ctx, c)
	result.Observation = result.Connection.Snapshot
	// Offline and damaged runtime.json must not hide a newer current-format
	// SQLite projection from read-only project history. Connection eligibility
	// remains the independent ConnectService result above.
	databaseObservation := viewer.NewFileSource(c).Snapshot()
	if databaseObservation.Runtime != nil && (result.Observation.Runtime == nil || databaseObservation.Runtime.Revision > result.Observation.Runtime.Revision || (databaseObservation.Runtime.Revision == result.Observation.Runtime.Revision && databaseObservation.Runtime.HeartbeatAt.After(result.Observation.Runtime.HeartbeatAt))) {
		result.Observation = databaseObservation
	} else if result.Observation.Runtime == nil && result.Observation.State == steerline.SnapshotNone {
		result.Observation = steerline.ReadObservationSnapshot(filepath.Join(c.RunDir, c.RuntimeFile), c.Heartbeat, nil)
	}
	if snapshot, err := steerline.ReadTaskSnapshot(c); err != nil {
		result.TaskError = err.Error()
	} else {
		result.Tasks = snapshot
		if snapshot.Error != nil && strings.TrimSpace(*snapshot.Error) != "" {
			result.TaskError = *snapshot.Error
		}
	}
	dbPath := filepath.Join(c.RunDir, "steerline.db")
	if _, err := os.Stat(dbPath); err == nil {
		store, openErr := steerline.OpenSQLiteReader(dbPath)
		if openErr != nil {
			result.DomainError = openErr.Error()
		} else {
			domain, queryErr := store.Query(ctx, c.Identity())
			_ = store.Close()
			if queryErr != nil {
				result.DomainError = queryErr.Error()
			} else {
				result.Domain = &domain
			}
		}
	} else if !os.IsNotExist(err) {
		result.DomainError = err.Error()
	}
	return result
}

func (FileQuerySource) ReadActivity(ctx context.Context, ref ProjectRef, runID, executionID, attemptID, currentPath string, cursor steerline.JournalCursor) (viewer.LogBatch, error) {
	path := currentPath
	if path == "" {
		store, err := steerline.OpenSQLiteReader(filepath.Join(ref.RunDir, "steerline.db"))
		if err != nil {
			return viewer.LogBatch{}, err
		}
		domain, err := store.Query(ctx, ref.Identity)
		_ = store.Close()
		if err != nil {
			return viewer.LogBatch{}, err
		}
		for _, attempt := range domain.Attempts {
			if attempt.ExecutionID == executionID && attempt.AttemptID == attemptID {
				path = attempt.EventPath
				break
			}
		}
		if path == "" {
			return viewer.LogBatch{}, fmt.Errorf("attempt %s has no queryable activity record", attemptID)
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(ref.RunDir, path)
		}
	}
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(path) == "" {
		return viewer.LogBatch{}, errors.New("activity query identity or record path is missing")
	}
	return viewer.NewFileSource(ref.Config).ReadLog(path, cursor, false)
}

func (FileQuerySource) ReadConversation(ctx context.Context, ref ProjectRef, filter steerline.ConversationFilter, cursor string) (steerline.ConversationPage, error) {
	if ref.Error != "" {
		return steerline.ConversationPage{}, errors.New(ref.Error)
	}
	store, err := steerline.OpenSQLiteReader(filepath.Join(ref.RunDir, "steerline.db"))
	if err != nil {
		return steerline.ConversationPage{}, err
	}
	defer store.Close()
	query := steerline.ConversationQueryService{Store: store, Project: ref.Identity}
	return query.ReadConversation(ctx, filter, cursor, steerline.MaxEventPageSize)
}

func (FileQuerySource) ReadBody(ctx context.Context, ref ProjectRef, body steerline.ConversationBodyRef, offset int64, limit int) (steerline.ConversationBodyPage, error) {
	store, err := steerline.OpenSQLiteReader(filepath.Join(ref.RunDir, "steerline.db"))
	if err != nil {
		return steerline.ConversationBodyPage{}, err
	}
	defer store.Close()
	return (steerline.ConversationQueryService{Store: store, Project: ref.Identity}).ReadMessageBody(ctx, body, offset, limit)
}

func (FileQuerySource) ReadCapabilities(ctx context.Context, ref ProjectRef, backend steerline.BackendKind, protocol steerline.ExecutionProtocol, targetID, executionID, attemptID, source, sourceIdentity string) ([]steerline.ConversationCapabilityReport, error) {
	store, err := steerline.OpenSQLiteReader(filepath.Join(ref.RunDir, "steerline.db"))
	if err != nil {
		return nil, err
	}
	defer store.Close()
	reports, err := store.ConversationCapabilityReports(ctx, backend, protocol, targetID, 100)
	if err != nil {
		return nil, err
	}
	filtered := make([]steerline.ConversationCapabilityReport, 0, len(reports))
	for _, report := range reports {
		if executionID != "" && report.ExecutionID != executionID {
			continue
		}
		if attemptID != "" && report.AttemptID != attemptID {
			continue
		}
		if source != "" && report.Source != source {
			continue
		}
		if sourceIdentity != "" && report.SourceIdentity != sourceIdentity {
			continue
		}
		filtered = append(filtered, report)
	}
	return filtered, nil
}

type FileControlClient struct{}

func (FileControlClient) Submit(ctx context.Context, ref ProjectRef, action string, payload any, requestID, instance string, revision *uint64) (steerline.ControlRequest, error) {
	if err := ctx.Err(); err != nil {
		return steerline.ControlRequest{}, err
	}
	if ref.Error != "" {
		return steerline.ControlRequest{}, errors.New(ref.Error)
	}
	return steerline.SubmitControlTargeted(ref.Config, action, payload, steerline.DefaultRequestedBy(), requestID, instance, revision)
}

func (FileControlClient) Receipt(ctx context.Context, ref ProjectRef, requestID string) (*steerline.ControlReceipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return steerline.ReadVerifiedControlReceipt(ref.Config, requestID)
}

func (FileControlClient) Request(ctx context.Context, ref ProjectRef, requestID string) (*steerline.ControlRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return steerline.ReadControlRequest(ref.Config, requestID)
}

func candidateFreshness(ref ProjectRef, snapshot ProjectSnapshot, now time.Time) (bool, string) {
	if snapshot.Tasks == nil {
		if snapshot.TaskError != "" {
			return false, snapshot.TaskError
		}
		return false, "任务候选快照不可用"
	}
	if snapshot.TaskError != "" {
		return false, snapshot.TaskError
	}
	controlRuntime := snapshot.Connection.Snapshot.Runtime
	if controlRuntime == nil || controlRuntime.Service.InstanceID == "" {
		return false, "缺少当前服务实例身份"
	}
	if snapshot.Tasks.InstanceID != controlRuntime.Service.InstanceID || viewCanonical(snapshot.Tasks.RepositoryIdentity) != viewCanonical(ref.Identity) || viewCanonical(controlRuntime.Repository.Identity) != viewCanonical(ref.Identity) {
		return false, "候选缓存属于其他服务实例或仓库"
	}
	if snapshot.Tasks.LastSuccessAt == nil || snapshot.Tasks.LastSuccessAt.IsZero() {
		return false, "候选查询尚无成功记录"
	}
	if snapshot.Tasks.QueriedAt.After(now.Add(time.Minute)) || snapshot.Tasks.LastSuccessAt.After(now.Add(time.Minute)) {
		return false, "候选快照时间位于未来"
	}
	poll := 0 * time.Second
	if d, err := time.ParseDuration(ref.Config.TaskPoll); err == nil && d > 0 {
		poll = d
	}
	limit := 30 * time.Second
	if poll*3 > limit {
		limit = poll * 3
	}
	if now.Sub(snapshot.Tasks.LastSuccessAt.UTC()) > limit {
		return false, fmt.Sprintf("候选缓存已过期（上次成功 %s）", snapshot.Tasks.LastSuccessAt.Local().Format("15:04:05"))
	}
	if snapshot.Connection.State != steerline.ConnectionConnected {
		return false, "服务未就绪：" + string(snapshot.Connection.State)
	}
	return true, ""
}

func absoluteViewKey(configPath, runDir string) (string, error) {
	configPath, err := filepath.Abs(configPath)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(runDir) == "" {
		return viewCanonical(configPath) + "\x00", nil
	}
	runDir, err = filepath.Abs(runDir)
	if err != nil {
		return "", err
	}
	return viewCanonical(configPath) + "\x00" + viewCanonical(runDir), nil
}

func viewCanonical(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}
