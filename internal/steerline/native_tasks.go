package steerline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const nativeTaskProtocol = "native-tasks/v1"

type NativeTaskCapabilities struct {
	DependencyReadiness string `json:"dependency_readiness"`
	DependencyBlockers  string `json:"dependency_blockers"`
	StartupBlockers     string `json:"startup_blockers"`
}

type NativeTaskDetail struct {
	TaskRecord
	ProjectID         string                 `json:"project_id"`
	Priority          int                    `json:"priority"`
	Contract          TaskContract           `json:"contract"`
	WorkflowRuns      []WorkflowRunRecord    `json:"workflow_runs"`
	Delivery          *TaskDeliveryRecord    `json:"delivery,omitempty"`
	Predecessors      []TaskDependencyStatus `json:"predecessors"`
	Successors        []string               `json:"successors"`
	DependencyReady   bool                   `json:"dependency_ready"`
	UnmetPredecessors []TaskDependencyStatus `json:"unmet_predecessors"`
	StartupBlockers   []string               `json:"startup_blockers"`
}

type TaskDependencyStatus struct {
	TaskID    string `json:"task_id"`
	Status    string `json:"status"`
	Delivered bool   `json:"delivered"`
	Blocker   string `json:"blocker,omitempty"`
}

type NativeTaskPage struct {
	Protocol         string                 `json:"protocol"`
	ProjectID        string                 `json:"project_id"`
	AuthorityVersion uint64                 `json:"authority_version"`
	GraphRevision    uint64                 `json:"graph_revision"`
	DatabaseRevision uint64                 `json:"database_revision"`
	DatabaseID       string                 `json:"database_id"`
	SampledAt        time.Time              `json:"sampled_at"`
	Capabilities     NativeTaskCapabilities `json:"capabilities"`
	Tasks            []NativeTaskDetail     `json:"tasks"`
	NextCursor       string                 `json:"next_cursor,omitempty"`
	HasMore          bool                   `json:"has_more"`
}

type nativeTaskCursor struct {
	Version          int    `json:"v"`
	ProjectID        string `json:"p"`
	AuthorityVersion uint64 `json:"a"`
	GraphRevision    uint64 `json:"g"`
	DatabaseRevision uint64 `json:"d"`
	DatabaseID       string `json:"i"`
	Offset           int    `json:"o"`
}

type NativeTaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *NativeTaskError) Error() string   { return e.Code + ": " + e.Message }
func nativeErr(code, message string) error { return &NativeTaskError{Code: code, Message: message} }

func ValidateNativeTaskID(id string) error {
	if !canonicalDecimalID(id) {
		return nativeErr("contract_mismatch", "task ID must be a canonical nonnegative decimal number")
	}
	return nil
}

func (q QueryService) ListNativeTasks(ctx context.Context, cursor string, limit int) (NativeTaskPage, error) {
	if q.Store == nil || q.Project == "" {
		return NativeTaskPage{}, errors.New("query service store and project identity are required")
	}
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	offset := 0
	expectedDB := uint64(0)
	expectedDatabaseID := ""
	expectedAuthority, expectedGraph := uint64(0), uint64(0)
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return NativeTaskPage{}, nativeErr("cursor_invalidated", "cursor encoding is invalid")
		}
		var c nativeTaskCursor
		if decode(raw, &c) != nil || c.Version != 1 || c.ProjectID != q.Project || c.Offset < 0 || c.DatabaseRevision == 0 || len(c.DatabaseID) != 32 {
			return NativeTaskPage{}, nativeErr("cursor_invalidated", "cursor identity or sampling revision is invalid")
		}
		offset, expectedDB = c.Offset, c.DatabaseRevision
		expectedDatabaseID = c.DatabaseID
		expectedAuthority, expectedGraph = c.AuthorityVersion, c.GraphRevision
	}
	page, err := q.Store.NativeTaskPage(ctx, q.Project, "", expectedDatabaseID, expectedDB, offset, limit)
	if err != nil {
		return NativeTaskPage{}, err
	}
	if expectedDB != 0 && (page.AuthorityVersion != expectedAuthority || page.GraphRevision != expectedGraph) {
		return NativeTaskPage{}, nativeErr("cursor_invalidated", "project authority changed between pages")
	}
	if page.HasMore {
		c := nativeTaskCursor{Version: 1, ProjectID: page.ProjectID, AuthorityVersion: page.AuthorityVersion, GraphRevision: page.GraphRevision, DatabaseID: page.DatabaseID, DatabaseRevision: page.DatabaseRevision, Offset: offset + len(page.Tasks)}
		b, _ := json.Marshal(c)
		page.NextCursor = base64.RawURLEncoding.EncodeToString(b)
	}
	return page, nil
}

func (q QueryService) ShowNativeTask(ctx context.Context, id string) (NativeTaskPage, error) {
	if q.Store == nil || q.Project == "" {
		return NativeTaskPage{}, errors.New("query service store and project identity are required")
	}
	if err := ValidateNativeTaskID(id); err != nil {
		return NativeTaskPage{}, err
	}
	page, err := q.Store.NativeTaskPage(ctx, q.Project, id, "", 0, 0, 1)
	if err != nil {
		return NativeTaskPage{}, err
	}
	if len(page.Tasks) == 0 {
		return NativeTaskPage{}, nativeErr("task_not_found", "native task was not found")
	}
	return page, nil
}

func (s *SQLiteStore) NativeTaskPage(ctx context.Context, project, taskID, expectedDatabaseID string, expectedDB uint64, offset, limit int) (NativeTaskPage, error) {
	page := NativeTaskPage{Protocol: nativeTaskProtocol, ProjectID: project, Tasks: []NativeTaskDetail{}, Capabilities: NativeTaskCapabilities{DependencyReadiness: "provided", DependencyBlockers: "provided", StartupBlockers: "provided"}}
	if offset < 0 {
		return page, nativeErr("cursor_invalidated", "negative page offset")
	}
	if project == "" {
		return page, nativeErr("authority_mismatch", "project identity is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return page, err
	}
	defer tx.Rollback()
	var schemaVersion int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&schemaVersion); err != nil || schemaVersion != domainSchemaVersion {
		if expectedDB != 0 {
			return page, nativeErr("cursor_invalidated", "cursor database schema is unavailable")
		}
		return page, nativeErr("unsupported_schema", "native task query requires the current database schema")
	}
	var targetRepository, targetRemote, targetBranch, dispatchMode string
	var dispatchPaused int
	err = tx.QueryRowContext(ctx, `SELECT authority_version,graph_revision,repository,remote,branch,dispatch_mode,dispatch_paused FROM project_task_authorities WHERE project_identity=? AND authority='native'`, project).Scan(&page.AuthorityVersion, &page.GraphRevision, &targetRepository, &targetRemote, &targetBranch, &dispatchMode, &dispatchPaused)
	if errors.Is(err, sql.ErrNoRows) {
		if expectedDB != 0 {
			return page, nativeErr("cursor_invalidated", "project authority could not be read for this cursor")
		}
		return page, nativeErr("authority_mismatch", "project has no native task authority")
	}
	if err != nil {
		if expectedDB != 0 {
			return page, nativeErr("cursor_invalidated", "project authority could not be read for this cursor")
		}
		return page, nativeErr("unsupported_schema", "native project authority schema is unavailable")
	}
	var dbRevision uint64
	if err = tx.QueryRowContext(ctx, `SELECT database_id,revision FROM database_state WHERE singleton=1`).Scan(&page.DatabaseID, &dbRevision); err != nil {
		if expectedDB != 0 {
			return page, nativeErr("cursor_invalidated", "database sampling revision is unavailable")
		}
		return page, nativeErr("unsupported_schema", "database sampling revision is unavailable")
	}
	if expectedDB != 0 && (expectedDB != dbRevision || expectedDatabaseID != page.DatabaseID) {
		return page, nativeErr("cursor_invalidated", "database revision changed between pages")
	}
	page.DatabaseRevision = dbRevision
	if binding := s.nativeBinding; binding != nil && (binding.ProjectIdentity != project || binding.Repository != targetRepository || binding.Remote != targetRemote || binding.Branch != targetBranch) {
		return page, nativeErr("authority_mismatch", "project binding changed after opening the query reader")
	}
	page.SampledAt = time.Now().UTC()
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	query := `SELECT task_id,status,version,payload FROM tasks WHERE project_identity=?`
	args := []any{project}
	if taskID != "" {
		query += ` AND task_id=?`
		args = append(args, taskID)
	}
	// Canonical decimal IDs sort numerically by length then text, without casts
	// or loading and sorting the entire project in memory.
	query += ` ORDER BY length(task_id),task_id LIMIT ? OFFSET ?`
	args = append(args, limit+1, offset)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return page, err
	}
	var all []TaskRecord
	for rows.Next() {
		var id string
		var status string
		var version uint64
		var payload []byte
		if err = rows.Scan(&id, &status, &version, &payload); err != nil {
			rows.Close()
			return page, err
		}
		var record TaskRecord
		if err = json.Unmarshal(payload, &record); err != nil {
			rows.Close()
			return page, err
		}
		if record.Source != "native" {
			rows.Close()
			return page, nativeErr("authority_mismatch", "native authority contains a non-native task record")
		}
		if record.ProjectIdentity != project || record.TaskID != id || !canonicalDecimalID(record.TaskID) {
			rows.Close()
			return page, nativeErr("contract_mismatch", "native task ID is not canonical")
		}
		if record.Status != status || record.Version != version || !validTaskStatus(record.Status) || record.Version == 0 || record.Type == "" || record.Priority < 0 || record.Priority > 4 {
			rows.Close()
			return page, nativeErr("contract_mismatch", "native task record has invalid status, type, priority or version")
		}
		if record.Status == "discarded" && (record.DiscardedAt == nil || strings.TrimSpace(record.DiscardReason) == "" || strings.TrimSpace(record.DiscardedBy) == "" || !controlIDPattern.MatchString(record.DiscardRequestID)) || record.Status != "discarded" && (record.DiscardedAt != nil || record.DiscardReason != "" || record.DiscardedBy != "" || record.DiscardRequestID != "" || len(record.CleanupPending) > 0) {
			rows.Close()
			return page, nativeErr("contract_mismatch", "native discard identity and cleanup fields do not match task status")
		}
		if record.Status == "open" && record.Claimant != "" || (record.Status == "in_progress" || record.Status == "closed") && record.Claimant == "" {
			rows.Close()
			return page, nativeErr("contract_mismatch", "native task claim does not match task status")
		}
		all = append(all, record)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return page, err
	}
	rows.Close()
	// Read every task, edge and delivery in this same snapshot so a detail page
	// never treats a missing page-local predecessor as delivered or ready.
	statuses := map[string]string{}
	rows, err = tx.QueryContext(ctx, `SELECT task_id,status FROM tasks WHERE project_identity=?`, project)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		var id, status string
		if err = rows.Scan(&id, &status); err != nil {
			rows.Close()
			return page, err
		}
		statuses[id] = status
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return page, err
	}
	rows.Close()
	edges := []TaskDependencyRecord{}
	rows, err = tx.QueryContext(ctx, `SELECT predecessor_id,successor_id FROM task_dependencies WHERE project_identity=? ORDER BY length(predecessor_id),predecessor_id,length(successor_id),successor_id`, project)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		var edge TaskDependencyRecord
		edge.ProjectIdentity = project
		if err = rows.Scan(&edge.PredecessorID, &edge.SuccessorID); err != nil {
			rows.Close()
			return page, err
		}
		edges = append(edges, edge)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return page, err
	}
	rows.Close()
	delivered := map[string]bool{}
	rows, err = tx.QueryContext(ctx, `SELECT task_id FROM task_deliveries WHERE project_identity=?`, project)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return page, err
		}
		delivered[id] = true
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return page, err
	}
	rows.Close()
	var executionSlotBusy int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM executions WHERE project_identity=? AND status IN ('pending','active','waiting_decision','blocked')`, project).Scan(&executionSlotBusy); err != nil {
		return page, err
	}
	if offset > 0 && len(all) == 0 {
		return page, nativeErr("cursor_invalidated", "cursor offset is outside the current snapshot")
	}
	page.HasMore = len(all) > limit
	if page.HasMore {
		all = all[:limit]
	}
	for _, record := range all {
		detail := NativeTaskDetail{TaskRecord: record, ProjectID: project, Priority: record.Priority, WorkflowRuns: []WorkflowRunRecord{}, Predecessors: []TaskDependencyStatus{}, Successors: []string{}, UnmetPredecessors: []TaskDependencyStatus{}, StartupBlockers: []string{}}
		dependencyReady := record.Status == "open"
		for _, edge := range edges {
			if edge.PredecessorID == record.TaskID {
				detail.Successors = append(detail.Successors, edge.SuccessorID)
			}
			if edge.SuccessorID != record.TaskID {
				continue
			}
			predecessor := TaskDependencyStatus{TaskID: edge.PredecessorID, Status: statuses[edge.PredecessorID], Delivered: delivered[edge.PredecessorID]}
			if predecessor.Status == "" {
				predecessor.Blocker = "inconsistent_delivery"
			} else if predecessor.Status == "discarded" {
				predecessor.Blocker = "discarded_predecessor"
			} else if predecessor.Status == "closed" && !predecessor.Delivered {
				predecessor.Blocker = "inconsistent_delivery"
			} else if predecessor.Delivered && predecessor.Status != "closed" {
				predecessor.Blocker = "inconsistent_delivery"
			} else if !predecessor.Delivered {
				predecessor.Blocker = "predecessor_not_delivered"
			}
			detail.Predecessors = append(detail.Predecessors, predecessor)
			if predecessor.Blocker != "" {
				detail.UnmetPredecessors = append(detail.UnmetPredecessors, predecessor)
				dependencyReady = false
			}
		}
		detail.DependencyReady = dependencyReady
		if record.Status != "open" {
			detail.StartupBlockers = append(detail.StartupBlockers, "task_not_open")
		}
		if dispatchPaused != 0 {
			detail.StartupBlockers = append(detail.StartupBlockers, "dispatch_paused")
		}
		if executionSlotBusy > 0 {
			detail.StartupBlockers = append(detail.StartupBlockers, "execution_slot_busy")
		}
		if record.Status == "open" {
			detail.StartupBlockers = append(detail.StartupBlockers, "native_execution_unavailable")
		}
		var contractPayload []byte
		var contractDigest string
		err = tx.QueryRowContext(ctx, `SELECT digest,payload FROM task_contracts WHERE project_identity=? AND contract_id=? AND task_id=?`, project, record.ContractID, record.TaskID).Scan(&contractDigest, &contractPayload)
		if errors.Is(err, sql.ErrNoRows) {
			return page, nativeErr("contract_mismatch", "native task has no matching fixed contract")
		}
		if err != nil {
			return page, err
		}
		if err = json.Unmarshal(contractPayload, &detail.Contract); err != nil {
			return page, err
		}
		if detail.Contract.ContractID != record.ContractID || detail.Contract.Digest != contractDigest {
			return page, nativeErr("contract_mismatch", "task contract identity differs from task record")
		}
		if err = ValidateTaskContract(detail.Contract); err != nil {
			return page, err
		}
		if !sameRepositoryTarget(detail.Contract.Repository, targetRepository) || detail.Contract.Remote != targetRemote || detail.Contract.Branch != targetBranch {
			return page, nativeErr("contract_mismatch", "task contract target differs from persisted project authority")
		}
		runRows, e := tx.QueryContext(ctx, `SELECT workflow_run_id,phase,payload FROM workflow_runs WHERE project_identity=? AND task_id=? ORDER BY workflow_run_id`, project, record.TaskID)
		if e != nil {
			return page, e
		}
		for runRows.Next() {
			var b []byte
			var runID, phase string
			var run WorkflowRunRecord
			if e = runRows.Scan(&runID, &phase, &b); e != nil {
				runRows.Close()
				return page, e
			}
			if e = json.Unmarshal(b, &run); e != nil {
				runRows.Close()
				return page, e
			}
			if run.ProjectIdentity != project || run.TaskID != record.TaskID || run.WorkflowRunID != runID || run.Phase != phase {
				runRows.Close()
				return page, nativeErr("contract_mismatch", "workflow payload differs from its stored task binding")
			}
			detail.WorkflowRuns = append(detail.WorkflowRuns, run)
		}
		e = runRows.Err()
		runRows.Close()
		if e != nil {
			return page, e
		}
		var delivery TaskDeliveryRecord
		var merged, recorded string
		e = tx.QueryRowContext(ctx, `SELECT task_id,selected_workflow_run_id,pr_instance,pr_repository,pr_number,pr_url,merge_commit,merged_at,recorded_at,evidence FROM task_deliveries WHERE project_identity=? AND task_id=?`, project, record.TaskID).Scan(&delivery.TaskID, &delivery.SelectedWorkflowRunID, &delivery.PRInstance, &delivery.PRRepository, &delivery.PRNumber, &delivery.PRURL, &delivery.MergeCommit, &merged, &recorded, &delivery.Evidence)
		if e == nil {
			delivery.ProjectIdentity = project
			delivery.MergedAt, err = time.Parse(time.RFC3339Nano, merged)
			if err != nil {
				return page, nativeErr("contract_mismatch", "invalid delivery merge timestamp")
			}
			delivery.RecordedAt, err = time.Parse(time.RFC3339Nano, recorded)
			if err != nil {
				return page, nativeErr("contract_mismatch", "invalid delivery recording timestamp")
			}
			detail.Delivery = &delivery
		} else if !errors.Is(e, sql.ErrNoRows) {
			return page, e
		}
		if (record.Status == "closed") != (detail.Delivery != nil) {
			detail.DependencyReady = false
			detail.StartupBlockers = append(detail.StartupBlockers, "inconsistent_delivery")
		}
		page.Tasks = append(page.Tasks, detail)
	}
	if err = tx.Commit(); err != nil {
		return page, err
	}
	return page, nil
}

// OpenNativeTaskProject initializes one native project authority in a clean
// state directory. Once initialized, each read verifies the persisted binding.
func OpenNativeTaskProject(ctx context.Context, c *Config) (*SQLiteStore, error) {
	if c == nil {
		return nil, nativeErr("authority_mismatch", "native project configuration is required")
	}
	if c.taskAuthority() != "native" {
		return nil, nativeErr("authority_mismatch", "configuration must select task_authority= native")
	}
	if err := validateNativeStateLocation(*c); err != nil {
		return nil, err
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return nil, err
	}
	if err := preflightNativeStatePath(*c); err != nil {
		return nil, err
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	if _, err := os.Stat(path); err == nil {
		// Existing readers must coexist with the repository's single writer.
		return openBoundNativeStore(ctx, *c, path)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	lock, err := LockRepository(c.Identity())
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err := preflightNativeStatePath(*c); err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(path); statErr == nil {
		return openBoundNativeStore(ctx, *c, path)
	} else if !os.IsNotExist(statErr) {
		return nil, statErr
	}
	if err := ensureNativeStateClean(c.RunDir, c.Repository); err != nil {
		return nil, err
	}
	targetRepository, err := configuredRepositoryTarget(ctx, *c)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(c.RunDir, 0700); err != nil {
		return nil, err
	}
	db, err := OpenSQLiteStore(path)
	if err != nil {
		cleanupNewNativeDB(path)
		return nil, err
	}
	authority := ProjectTaskAuthority{ProjectIdentity: c.Identity(), Authority: "native", AuthorityVersion: 1, GraphRevision: 0, Repository: targetRepository, Remote: c.Remote, Branch: c.Branch, DispatchMode: "manual", DispatchPaused: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	err = db.Transact(ctx, func(tx DomainTx) error {
		var count int
		if err := tx.(*sqliteTx).tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return nativeErr("authority_mismatch", "clean native initialization found existing task records")
		}
		return tx.PutProjectTaskAuthority(authority)
	})
	if err != nil {
		_ = db.Close()
		cleanupNewNativeDB(path)
		return nil, err
	}
	if err = db.Close(); err != nil {
		return nil, err
	}
	return openBoundNativeStore(ctx, *c, path)
}

// OpenNativeTaskCommandStore opens the repository's existing native database
// for a bounded command. The caller must hold the repository identity lock,
// and the project must already have been initialized and bound.
func OpenNativeTaskCommandStore(ctx context.Context, c *Config) (*SQLiteStore, error) {
	if c == nil || c.taskAuthority() != "native" {
		return nil, nativeErr("authority_mismatch", "native task command requires task_authority=native")
	}
	if err := validateNativeStateLocation(*c); err != nil {
		return nil, err
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return nil, err
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	reader, err := openBoundNativeStore(ctx, *c, path)
	if err != nil {
		return nil, err
	}
	_ = reader.Close()
	writer, err := OpenSQLiteStore(path)
	if err != nil {
		return nil, err
	}
	authority, err := writer.nativeAuthority(ctx, c.Identity())
	if err != nil || authority.Authority != "native" || authority.Remote != c.Remote || authority.Branch != c.Branch {
		_ = writer.Close()
		return nil, nativeErr("authority_mismatch", "native project binding changed before command admission")
	}
	target, err := configuredRepositoryTarget(ctx, *c)
	if err != nil || !sameRepositoryTarget(authority.Repository, target) {
		_ = writer.Close()
		return nil, nativeErr("authority_mismatch", "repository target differs from the persisted native project")
	}
	writer.nativeBinding = &authority
	return writer, nil
}

func validateNativeStateLocation(c Config) error {
	key, err := repoStateKey(c)
	if err != nil {
		return nativeErr("authority_mismatch", "cannot resolve the repository's stable native state location")
	}
	expected := filepath.Join(SteerlineStateRoot(), key)
	if canonical(c.RunDir) != canonical(expected) {
		return nativeErr("authority_mismatch", "native tasks require the repository's stable shared state directory; remove the custom run_dir setting")
	}
	return nil
}

func cleanupNewNativeDB(path string) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(path + suffix)
	}
}

func openBoundNativeStore(ctx context.Context, c Config, path string) (_ *SQLiteStore, retErr error) {
	db, err := OpenSQLiteReader(path)
	if err != nil {
		return nil, nativeErr("unsupported_schema", "existing database is not a readable current SQLite database")
	}
	defer func() {
		if retErr != nil {
			_ = db.Close()
		}
	}()
	if err = validateDatabaseSchema(ctx, db.db); err != nil {
		return nil, err
	}
	authority, err := db.nativeAuthority(ctx, c.Identity())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nativeErr("authority_mismatch", "database has no native authority for this project; preserve existing state")
	}
	if err != nil {
		return nil, nativeErr("unsupported_schema", "database does not contain the native task schema")
	}
	target, err := configuredRepositoryTarget(ctx, c)
	if err != nil {
		return nil, err
	}
	if authority.Authority != "native" || authority.Repository != target || authority.Remote != c.Remote || authority.Branch != c.Branch {
		return nil, nativeErr("authority_mismatch", "configuration repository, remote or branch differs from persisted native authority")
	}
	db.nativeBinding = &authority
	return db, nil
}

func preflightNativeStatePath(c Config) error {
	if err := ensureNativeRepositoryHasNoBeads(c.Repository); err != nil {
		return err
	}
	if err := ensureRunDirOutsideRepository(c); err != nil {
		return err
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	if _, err := os.Stat(path); err == nil {
		// Full authority/binding checks are performed after Git identity resolution.
		if err := ensureNativeStateFilesAllowed(c.RunDir); err != nil {
			return err
		}
		reader, e := OpenSQLiteReader(path)
		if e != nil {
			return nativeErr("unsupported_schema", "existing native state database is unreadable")
		}
		defer reader.Close()
		return validateDatabaseSchema(context.Background(), reader.db)
	} else if !os.IsNotExist(err) {
		return err
	}
	return ensureNativeStateClean(c.RunDir, c.Repository)
}

func ensureRunDirOutsideRepository(c Config) error {
	if c.RunDir == "" || c.Repository == "" {
		return nativeErr("authority_mismatch", "repository and run_dir are required")
	}
	resolvedRun, err := resolveProspectivePath(c.RunDir)
	if err != nil {
		return err
	}
	resolvedRepo, err := filepath.EvalSymlinks(c.Repository)
	if err != nil {
		return err
	}
	if inside(resolvedRepo, resolvedRun) {
		return nativeErr("authority_mismatch", "native state directory must be outside the repository")
	}
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	worktrees, err := r.Output(context.Background(), c.Git, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nativeErr("authority_mismatch", "cannot verify native state against linked Git worktrees")
	}
	for _, record := range strings.Split(string(worktrees), "\x00") {
		if strings.HasPrefix(record, "worktree ") && inside(strings.TrimPrefix(record, "worktree "), resolvedRun) {
			return nativeErr("authority_mismatch", "native state directory must be outside every linked repository worktree")
		}
	}
	return nil
}

func ensureNativeRepositoryHasNoBeads(repository string) error {
	for _, name := range []string{".beads", ".beads.db", "beads.db"} {
		if _, err := os.Lstat(filepath.Join(repository, name)); err == nil {
			return nativeErr("authority_mismatch", "native project cannot initialize while Beads state exists; preserve it for manual recovery")
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func ensureNativeStateClean(runDir, repository string) error {
	if err := ensureNativeRepositoryHasNoBeads(repository); err != nil {
		return err
	}
	entries, err := os.ReadDir(runDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return nativeErr("authority_mismatch", "native initialization requires a clean state directory")
	}
	return nil
}

func ensureNativeStateFilesAllowed(runDir string) error {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		switch entry.Name() {
		case "steerline.db", "steerline.db-wal", "steerline.db-shm":
		default:
			return nativeErr("authority_mismatch", "native state directory contains unsupported legacy or runtime data: "+entry.Name())
		}
	}
	return nil
}

func configuredRepositoryTarget(ctx context.Context, c Config) (string, error) {
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	if c.Remote == "" || c.Branch == "" {
		return "", nativeErr("authority_mismatch", "native project requires a fixed remote and branch")
	}
	remoteURL, err := r.Output(ctx, c.Git, "remote", "get-url", c.Remote)
	if err != nil {
		return "", nativeErr("authority_mismatch", "configured Git remote cannot be resolved")
	}
	target := strings.TrimSpace(string(remoteURL))
	if target == "" {
		return "", nativeErr("authority_mismatch", "configured Git remote resolved to an empty repository")
	}
	if strings.Contains(target, "://") {
		parsed, parseErr := url.Parse(target)
		if parseErr != nil {
			return "", nativeErr("authority_mismatch", "configured Git remote URL is invalid")
		}
		if (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.User != nil {
			return "", nativeErr("authority_mismatch", "Git remote URL must not embed credentials")
		}
	}
	if c.PR.Repository != "" {
		if err := c.PR.ValidateRemote(target); err != nil {
			return "", nativeErr("authority_mismatch", "PR repository does not match the configured Git remote")
		}
	}
	return target, nil
}
func sameRepositoryTarget(a, b string) bool {
	// A remote URL is not a Windows filesystem path: its repository component
	// may be case sensitive and must never be folded with canonical().
	return a == b
}

func validSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func validCommit(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func TaskContractDigest(v TaskContract) (string, error) {
	v.Digest = ""
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	canonical, err := CanonicalJSON(b)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func ValidateTaskContract(v TaskContract) error {
	if v.ContractID == "" || v.Repository == "" || v.Remote == "" || v.Branch == "" {
		return nativeErr("contract_mismatch", "contract ID and repository target are required")
	}
	if v.SpecRef.Path == v.PlanRef.Path {
		return nativeErr("contract_mismatch", "contract must pin exactly one independent Spec and one distinct Plan")
	}
	if len(v.AcceptanceRefs) == 0 {
		return nativeErr("contract_mismatch", "contract requires at least one acceptance reference")
	}
	refs := []FileReference{v.SpecRef, v.PlanRef}
	refs = append(refs, v.CommonRefs...)
	refs = append(refs, v.AcceptanceRefs...)
	for _, ref := range refs {
		if err := validateFileReferenceShape(ref); err != nil {
			return err
		}
	}
	for _, ref := range v.CommonRefs {
		if ref.Path == v.SpecRef.Path || ref.Path == v.PlanRef.Path || !sharedContractPath(ref.Path) {
			return nativeErr("contract_mismatch", "common_refs may contain only shared constraints or protocols")
		}
	}
	for _, ref := range v.AcceptanceRefs {
		if !strings.HasSuffix(strings.ToLower(ref.Path), ".md") {
			return nativeErr("contract_mismatch", "acceptance reference must point to a Markdown document")
		}
		if ref == v.SpecRef || ref == v.PlanRef {
			continue
		}
		if ref.Path == v.SpecRef.Path || ref.Path == v.PlanRef.Path || (taskDocumentPath(ref.Path) && !sharedContractPath(ref.Path)) {
			return nativeErr("contract_mismatch", "acceptance_refs cannot add another task's Spec/Plan or another version of the fixed documents")
		}
	}
	digest, err := TaskContractDigest(v)
	if err != nil {
		return err
	}
	if !strings.EqualFold(digest, v.Digest) {
		return nativeErr("contract_mismatch", "contract canonical digest does not match its content")
	}
	return nil
}

func taskDocumentPath(name string) bool {
	name = strings.ToLower(name)
	return strings.HasSuffix(name, ".spec.md") || strings.HasSuffix(name, ".plan.md")
}

// Shared documentation uses constraint/protocol paths; the execution
// architecture is the repository's established shared .spec.md document.
// This structural convention does not prove semantic specification quality.
func sharedContractPath(name string) bool {
	name = strings.ToLower(name)
	if path.Base(name) == "execution-architecture.spec.md" {
		return true
	}
	if taskDocumentPath(name) {
		return false
	}
	for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '-' || r == '_' || r == '.' }) {
		if part == "constraint" || part == "constraints" || part == "protocol" || part == "protocols" {
			return true
		}
	}
	return false
}

func validateFileReferenceShape(ref FileReference) error {
	if ref.Path == "" || strings.Contains(ref.Path, "\\") || path.IsAbs(ref.Path) || filepath.IsAbs(ref.Path) || regexp.MustCompile(`^[A-Za-z]:`).MatchString(ref.Path) || path.Clean(ref.Path) != ref.Path || ref.Path == "." || strings.HasPrefix(ref.Path, "../") || ref.Path == ".." {
		return nativeErr("contract_mismatch", "specification path must be a normalized repository-relative path")
	}
	if !validCommit(ref.Commit) || !validSHA256(ref.SHA256) {
		return nativeErr("contract_mismatch", "file reference requires a full commit and SHA-256 blob digest")
	}
	return nil
}

// VerifyTaskContractFiles validates pinned committed blobs. TASK-DAG must also
// check the actual execution workspace before claim/implementation; this check
// alone is never a startup permission.
func VerifyTaskContractFiles(ctx context.Context, c Config, v TaskContract) error {
	if err := ValidateTaskContract(v); err != nil {
		return err
	}
	target, err := configuredRepositoryTarget(ctx, c)
	if err != nil {
		return err
	}
	if !sameRepositoryTarget(v.Repository, target) || v.Remote != c.Remote || v.Branch != c.Branch {
		return nativeErr("contract_mismatch", "contract repository, remote or branch differs from the configured project")
	}
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	refs := []FileReference{v.SpecRef, v.PlanRef}
	refs = append(refs, v.CommonRefs...)
	refs = append(refs, v.AcceptanceRefs...)
	for _, ref := range refs {
		b, err := readCommittedRegularFile(ctx, r, c.Git, ref)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), ref.SHA256) {
			return nativeErr("contract_mismatch", "committed file SHA-256 does not match pinned reference: "+ref.Path)
		}
		if containsReference(v.AcceptanceRefs, ref) {
			if !hasAcceptanceHeading(b) {
				return nativeErr("contract_mismatch", "acceptance reference has no acceptance heading: "+ref.Path)
			}
		}
	}
	return nil
}

func containsReference(refs []FileReference, target FileReference) bool {
	for _, ref := range refs {
		if ref == target {
			return true
		}
	}
	return false
}
func hasAcceptanceHeading(b []byte) bool {
	re := regexp.MustCompile(`(?im)^#{1,6}[ \t]+[^\r\n]*(验收|acceptance)[^\r\n]*$`)
	return re.Match(b)
}

func readCommittedRegularFile(ctx context.Context, r Runner, git Command, ref FileReference) ([]byte, error) {
	if err := validateFileReferenceShape(ref); err != nil {
		return nil, err
	}
	resolved, err := r.Output(ctx, git, "rev-parse", "--verify", ref.Commit+"^{commit}")
	if err != nil || strings.TrimSpace(string(resolved)) != ref.Commit {
		return nil, nativeErr("contract_mismatch", "pinned Git commit is missing or is not a full canonical commit: "+ref.Commit)
	}
	tree, err := r.Output(ctx, git, "ls-tree", "-z", ref.Commit, "--", ref.Path)
	if err != nil {
		return nil, nativeErr("contract_mismatch", "cannot resolve committed contract file: "+ref.Path)
	}
	entries := bytes.Split(bytes.TrimSuffix(tree, []byte{0}), []byte{0})
	if len(entries) != 1 || len(entries[0]) == 0 {
		return nil, nativeErr("contract_mismatch", "pinned contract path is missing or ambiguous: "+ref.Path)
	}
	parts := bytes.SplitN(entries[0], []byte{'\t'}, 2)
	if len(parts) != 2 || string(parts[1]) != ref.Path {
		return nil, nativeErr("contract_mismatch", "pinned contract path is not an exact repository file: "+ref.Path)
	}
	meta := strings.Fields(string(parts[0]))
	if len(meta) != 3 || meta[1] != "blob" || (meta[0] != "100644" && meta[0] != "100755") {
		return nil, nativeErr("contract_mismatch", "contract references must resolve to regular committed files, not links or directories")
	}
	b, err := r.Output(ctx, git, "cat-file", "blob", meta[2])
	if err != nil {
		return nil, nativeErr("contract_mismatch", "cannot read committed blob: "+ref.Path)
	}
	return b, nil
}

func ValidateTaskContractJSON(data []byte) (TaskContract, error) {
	if _, err := CanonicalJSON(data); err != nil {
		return TaskContract{}, nativeErr("contract_mismatch", err.Error())
	}
	var v TaskContract
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return v, nativeErr("contract_mismatch", err.Error())
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return v, nativeErr("contract_mismatch", "expected one JSON value")
	}
	return v, ValidateTaskContract(v)
}

// CanonicalJSON rejects duplicate object keys and emits sorted-key, UTF-8 JSON
// with no insignificant whitespace. Array order is preserved.
func CanonicalJSON(data []byte) ([]byte, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("canonical JSON input must be valid UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	v, err := readJSONValue(d)
	if err != nil {
		return nil, err
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	var out bytes.Buffer
	e := json.NewEncoder(&out)
	e.SetEscapeHTML(false)
	if err = e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(out.Bytes(), []byte{'\n'}), nil
}

func readJSONValue(d *json.Decoder) (any, error) {
	t, err := d.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return t, nil
	}
	switch delim {
	case '{':
		m := map[string]any{}
		for d.More() {
			kt, err := d.Token()
			if err != nil {
				return nil, err
			}
			key, ok := kt.(string)
			if !ok {
				return nil, errors.New("object key is not a string")
			}
			if _, exists := m[key]; exists {
				return nil, fmt.Errorf("duplicate JSON object key %q", key)
			}
			value, err := readJSONValue(d)
			if err != nil {
				return nil, err
			}
			m[key] = value
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return nil, errors.New("invalid JSON object")
		}
		return m, nil
	case '[':
		a := []any{}
		for d.More() {
			value, err := readJSONValue(d)
			if err != nil {
				return nil, err
			}
			a = append(a, value)
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return nil, errors.New("invalid JSON array")
		}
		return a, nil
	default:
		return nil, errors.New("unexpected JSON delimiter")
	}
}

func (s *SQLiteStore) nativeAuthority(ctx context.Context, project string) (ProjectTaskAuthority, error) {
	var v ProjectTaskAuthority
	var paused int
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT authority,authority_version,graph_revision,repository,remote,branch,dispatch_mode,dispatch_paused,created_at,updated_at FROM project_task_authorities WHERE project_identity=?`, project).Scan(&v.Authority, &v.AuthorityVersion, &v.GraphRevision, &v.Repository, &v.Remote, &v.Branch, &v.DispatchMode, &paused, &created, &updated)
	if err != nil {
		return v, err
	}
	v.ProjectIdentity = project
	v.DispatchPaused = paused != 0
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return v, nil
}

var _ DomainStore = (*SQLiteStore)(nil)
var _ DomainTx = (*sqliteTx)(nil)
