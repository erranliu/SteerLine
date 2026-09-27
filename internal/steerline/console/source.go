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
	Legacy     *steerline.LegacyBeadsProject
	Identity   string
	RunDir     string
	Error      string
	Conflict   string
}

type ProjectSnapshot struct {
	Connection      steerline.ServiceConnection
	Observation     steerline.ObservationSnapshot
	Tasks           *steerline.TaskSnapshot
	TaskError       string
	NativeTasks     *steerline.NativeTaskPage
	SelectedNative  *steerline.NativeTaskDetail
	NativeTaskError string
	Domain          *steerline.DomainSnapshot
	DomainError     string
	Legacy          *steerline.LegacyBeadsSnapshot
	LegacyError     string
}

type QuerySource interface {
	ReadProject(context.Context, ProjectRef, string) ProjectSnapshot
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

type DecisionCommandClient interface {
	DecisionEligibility(context.Context, ProjectRef, *steerline.Run) error
	SubmitDecision(context.Context, ProjectRef, steerline.DecisionAnswer, string, string) (steerline.CommandRecord, bool, error)
	DecisionCommand(context.Context, ProjectRef, string) (steerline.CommandRecord, error)
	SubmitServiceAction(context.Context, ProjectRef, string, string, string, uint64, string, any) (steerline.CommandRecord, bool, error)
}

type FileQuerySource struct{}

func (FileQuerySource) ReadProject(ctx context.Context, ref ProjectRef, selectedTaskID string) ProjectSnapshot {
	result := ProjectSnapshot{}
	if ref.Legacy != nil {
		legacy, err := ref.Legacy.Read(ctx)
		result.Legacy = &legacy
		if err != nil {
			result.LegacyError = err.Error()
		}
		return result
	}
	if ref.Error != "" {
		result.TaskError, result.DomainError = ref.Error, ref.Error
		return result
	}
	c := ref.Config
	if c.TaskAuthority != "native" {
		result.TaskError = "task_authority=native is required"
		result.DomainError = result.TaskError
		return result
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		result.TaskError = "resolve repository identity: " + err.Error()
		result.Connection = steerline.ServiceConnection{State: steerline.ConnectionIdentityError, Diagnostic: result.TaskError}
		return result
	}
	connectionCh := make(chan steerline.ServiceConnection, 1)
	observationCh := make(chan steerline.ObservationSnapshot, 1)
	type readerResult struct {
		store *steerline.SQLiteStore
		err   error
	}
	readerCh := make(chan readerResult, 1)
	connectionConfig, observationConfig, readerConfig := c, c, c
	go func() { connectionCh <- steerline.ConnectService(ctx, connectionConfig) }()
	go func() { observationCh <- viewer.NewFileSource(observationConfig).Snapshot() }()
	go func() {
		store, err := steerline.OpenNativeTaskReader(ctx, &readerConfig)
		readerCh <- readerResult{store: store, err: err}
	}()
	result.Connection = <-connectionCh
	result.Observation = result.Connection.Snapshot
	databaseObservation := <-observationCh
	if databaseObservation.Runtime != nil && (result.Observation.Runtime == nil || databaseObservation.Runtime.Revision > result.Observation.Runtime.Revision || (databaseObservation.Runtime.Revision == result.Observation.Runtime.Revision && databaseObservation.Runtime.HeartbeatAt.After(result.Observation.Runtime.HeartbeatAt))) {
		result.Observation = databaseObservation
	} else if result.Observation.Runtime == nil && result.Observation.State == steerline.SnapshotNone {
		result.Observation = steerline.ReadObservationSnapshot(filepath.Join(c.RunDir, c.RuntimeFile), c.Heartbeat, nil)
	}
	if _, statErr := os.Stat(filepath.Join(c.RunDir, "steerline.db")); statErr != nil {
		if reader := <-readerCh; reader.store != nil {
			_ = reader.store.Close()
		}
		if os.IsNotExist(statErr) {
			result.NativeTaskError = "native task database does not exist; the console remains read-only"
		} else {
			result.NativeTaskError = statErr.Error()
		}
		result.TaskError, result.DomainError = result.NativeTaskError, result.NativeTaskError
		return result
	}
	reader := <-readerCh
	store, err := reader.store, reader.err
	if err != nil {
		result.NativeTaskError, result.TaskError, result.DomainError = err.Error(), err.Error(), err.Error()
		return result
	}
	defer store.Close()
	query := steerline.QueryService{Store: store, Project: c.Identity()}
	nativeTasks, err := query.ListNativeTasks(ctx, "", 500)
	if err != nil {
		result.NativeTaskError, result.TaskError = err.Error(), err.Error()
	} else {
		result.NativeTasks = &nativeTasks
		ready, readyErr := query.ReadyNativeTaskSnapshot(ctx)
		if readyErr != nil {
			result.TaskError = readyErr.Error()
		} else if ready.ProjectID != nativeTasks.ProjectID || ready.AuthorityVersion != nativeTasks.AuthorityVersion || ready.GraphRevision != nativeTasks.GraphRevision || ready.DatabaseID != nativeTasks.DatabaseID || ready.DatabaseRevision != nativeTasks.DatabaseRevision {
			result.TaskError = "cursor_invalidated: selected task and Ready candidates use different sampled versions"
		} else {
			result.Tasks = nativeCandidateSnapshot(ref, result.Connection, ready)
		}
		if selectedTaskID != "" && steerline.ValidateNativeTaskID(selectedTaskID) == nil {
			found := false
			for i := range nativeTasks.Tasks {
				if nativeTasks.Tasks[i].TaskID == selectedTaskID {
					found = true
					break
				}
			}
			if !found {
				selected, showErr := query.ShowNativeTask(ctx, selectedTaskID)
				if showErr == nil && len(selected.Tasks) == 1 && selected.AuthorityVersion == nativeTasks.AuthorityVersion && selected.DatabaseID == nativeTasks.DatabaseID && selected.DatabaseRevision == nativeTasks.DatabaseRevision && selected.GraphRevision == nativeTasks.GraphRevision {
					result.SelectedNative = &selected.Tasks[0]
				} else if showErr == nil {
					result.NativeTaskError = "cursor_invalidated: selected task uses a different sampled version"
					result.TaskError = result.NativeTaskError
				} else {
					result.NativeTaskError = showErr.Error()
					result.TaskError = showErr.Error()
				}
			}
		}
	}
	domain, queryErr := store.Query(ctx, c.Identity())
	if queryErr != nil {
		result.DomainError = queryErr.Error()
	} else {
		result.Domain = &domain
	}
	return result
}

func nativeCandidateSnapshot(ref ProjectRef, connection steerline.ServiceConnection, page steerline.NativeReadyTaskSnapshot) *steerline.TaskSnapshot {
	queried := page.SampledAt
	snapshot := &steerline.TaskSnapshot{Version: 1, RepositoryIdentity: ref.Identity, QueriedAt: queried, LastSuccessAt: &queried, Tasks: []steerline.TaskCandidate{}}
	if connection.State == steerline.ConnectionConnected && connection.CanControl && connection.Snapshot.Runtime != nil {
		snapshot.InstanceID = connection.Snapshot.Runtime.Service.InstanceID
	}
	for _, task := range page.Tasks {
		if task.Status != "open" || !nativeTaskTypeExecutable(ref.Config.ExecutableTypes, task.Type) {
			continue
		}
		snapshot.Tasks = append(snapshot.Tasks, steerline.TaskCandidate{TaskID: task.TaskID, Title: task.Title, Type: task.Type})
	}
	return snapshot
}

func nativeTaskTypeExecutable(types []string, taskType string) bool {
	for _, candidate := range types {
		if candidate == taskType {
			return true
		}
	}
	return false
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

func (FileControlClient) DecisionEligibility(ctx context.Context, ref ProjectRef, run *steerline.Run) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ref.Error != "" {
		return errors.New(ref.Error)
	}
	return steerline.DecisionReplyEligibility(ctx, ref.Config, run)
}

func (FileControlClient) Submit(ctx context.Context, ref ProjectRef, action string, payload any, requestID, instance string, revision *uint64) (steerline.ControlRequest, error) {
	if ref.Legacy != nil {
		return steerline.ControlRequest{}, errors.New("old Beads project is read-only in this console")
	}
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

func (FileControlClient) SubmitDecision(ctx context.Context, ref ProjectRef, answer steerline.DecisionAnswer, requestedBy, commandID string) (steerline.CommandRecord, bool, error) {
	if ref.Legacy != nil {
		return steerline.CommandRecord{}, false, errors.New("old Beads project is read-only in this console")
	}
	if err := ctx.Err(); err != nil {
		return steerline.CommandRecord{}, false, err
	}
	if ref.Error != "" {
		return steerline.CommandRecord{}, false, errors.New(ref.Error)
	}
	return steerline.SubmitAnswerDecision(ctx, ref.Config, answer.RunID, answer.DecisionID, answer.OptionID, answer.Note, requestedBy, answer.ExpectedDecisionVersion, answer.ExpectedRunRevision, answer.ServiceInstanceID, commandID)
}

func (FileControlClient) DecisionCommand(ctx context.Context, ref ProjectRef, commandID string) (steerline.CommandRecord, error) {
	if err := ctx.Err(); err != nil {
		return steerline.CommandRecord{}, err
	}
	db, err := steerline.OpenSQLiteReader(filepath.Join(ref.RunDir, "steerline.db"))
	if err != nil {
		return steerline.CommandRecord{}, err
	}
	defer db.Close()
	return db.Command(ctx, ref.Identity, commandID)
}

func (FileControlClient) SubmitServiceAction(ctx context.Context, ref ProjectRef, kind, targetID, requestedBy string, expectedRevision uint64, commandID string, payload any) (steerline.CommandRecord, bool, error) {
	if ref.Legacy != nil {
		return steerline.CommandRecord{}, false, errors.New("old Beads project is read-only in this console")
	}
	if err := ctx.Err(); err != nil {
		return steerline.CommandRecord{}, false, err
	}
	if ref.Error != "" {
		return steerline.CommandRecord{}, false, errors.New(ref.Error)
	}
	return steerline.SubmitServiceCommand(ctx, ref.Config, kind, targetID, requestedBy, commandID, expectedRevision, payload)
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
	if snapshot.Connection.State != steerline.ConnectionConnected || !snapshot.Connection.CanControl {
		return false, firstText(snapshot.Connection.Diagnostic, "服务未连接；缓存任务不能作为启动许可")
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
