package steerline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Backend and protocol are deliberately independent. A host setting is only a
// compatibility input; persisted refs always carry both values.
type BackendKind string
type ExecutionProtocol string

const (
	BackendLocal           BackendKind       = "local"
	BackendOrca            BackendKind       = "orca"
	ProtocolLocalProcess   ExecutionProtocol = "local_process"
	ProtocolOrcaTerminalV2 ExecutionProtocol = "orca_terminal_v2"
	ExecutionRecordVersion                   = 1
)

type AgentSpec struct {
	Name string `json:"name"`
}
type ExecutionTarget struct {
	ID string `json:"id"`
}

// AttemptSpec is a value snapshot passed across the platform boundary. Legacy
// Run and Task values are copies used only to render today's dispatch format;
// the adapter cannot mutate the service's authoritative Run.
type AttemptSpec struct {
	ProjectID    string            `json:"project_id"`
	TaskID       string            `json:"task_id"`
	RunID        string            `json:"run_id"`
	AttemptID    string            `json:"attempt_id"`
	OperationID  string            `json:"operation_id"`
	ConfigDigest string            `json:"config_digest"`
	Repository   string            `json:"repository"`
	BaseCommit   string            `json:"base_commit"`
	Workspace    WorkspaceIntent   `json:"workspace"`
	Agent        AgentSpec         `json:"agent"`
	Protocol     ExecutionProtocol `json:"protocol"`
	Target       ExecutionTarget   `json:"target"`
	Task         Task              `json:"task"`
	Run          Run               `json:"run"`
	Beads        BeadsContext      `json:"beads"`
}

type ExecutionRef struct {
	Version         int               `json:"version"`
	Backend         BackendKind       `json:"backend"`
	Protocol        ExecutionProtocol `json:"protocol"`
	Target          ExecutionTarget   `json:"target"`
	Agent           AgentSpec         `json:"agent"`
	RunID           string            `json:"run_id"`
	AttemptID       string            `json:"attempt_id"`
	WorkspacePath   string            `json:"workspace_path,omitempty"`
	WorkspaceBranch string            `json:"workspace_branch,omitempty"`
	BaseCommit      string            `json:"base_commit,omitempty"`
	External        ProcessIdentity   `json:"external"`
	Generation      string            `json:"generation,omitempty"`
	Legacy          bool              `json:"legacy,omitempty"`
	Incomplete      bool              `json:"incomplete,omitempty"`
}

type OperationStatus string

const (
	OperationPending   OperationStatus = "pending"
	OperationInFlight  OperationStatus = "in_flight"
	OperationAccepted  OperationStatus = "accepted"
	OperationConfirmed OperationStatus = "confirmed"
	OperationRejected  OperationStatus = "rejected"
	OperationUnknown   OperationStatus = "outcome_unknown"
)

type Operation struct {
	Version           int               `json:"version"`
	ID                string            `json:"id"`
	RunID             string            `json:"run_id"`
	AttemptID         string            `json:"attempt_id"`
	Backend           BackendKind       `json:"backend"`
	Protocol          ExecutionProtocol `json:"protocol"`
	Target            ExecutionTarget   `json:"target"`
	Action            string            `json:"action"`
	PayloadDigest     string            `json:"payload_digest"`
	Status            OperationStatus   `json:"status"`
	StartedAt         time.Time         `json:"started_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	Diagnostic        string            `json:"diagnostic,omitempty"`
	ResidualResources []string          `json:"residual_resources,omitempty"`
	Ref               *ExecutionRef     `json:"ref,omitempty"`
}

type CapabilityStatus string

const (
	CapabilityVerified    CapabilityStatus = "verified"
	CapabilityUnsupported CapabilityStatus = "unsupported"
	CapabilityNotTested   CapabilityStatus = "not_tested"
)

type Capability struct {
	Name     string           `json:"name"`
	Status   CapabilityStatus `json:"status"`
	Evidence string           `json:"evidence,omitempty"`
}
type CapabilityReport struct {
	Version         int               `json:"version"`
	Backend         BackendKind       `json:"backend"`
	Protocol        ExecutionProtocol `json:"protocol"`
	Agent           AgentSpec         `json:"agent"`
	Target          ExecutionTarget   `json:"target"`
	CheckedAt       time.Time         `json:"checked_at"`
	PlatformVersion string            `json:"platform_version,omitempty"`
	Capabilities    []Capability      `json:"capabilities"`
}

type Observation struct {
	Liveness   string    `json:"liveness"`
	Connection string    `json:"connection"`
	ObservedAt time.Time `json:"observed_at"`
	Source     string    `json:"source"`
	Diagnostic string    `json:"diagnostic,omitempty"`
}
type StartReceipt struct {
	Disposition       string       `json:"disposition"` // accepted, rejected, or outcome_unknown
	Worker            *Worker      `json:"-"`
	Ref               ExecutionRef `json:"ref"`
	Run               Run          `json:"run"`
	ResidualResources []string     `json:"residual_resources,omitempty"`
}
type StopReceipt struct {
	Disposition string `json:"disposition"`
	Diagnostic  string `json:"diagnostic,omitempty"`
}
type OutputBatch struct {
	Data          []byte `json:"data"`
	NextCursor    string `json:"next_cursor,omitempty"`
	Gap           bool   `json:"gap,omitempty"`
	SourceChanged bool   `json:"source_changed,omitempty"`
}
type BackendErrorCode string

const (
	BackendUnsupported      BackendErrorCode = "unsupported"
	BackendRejected         BackendErrorCode = "rejected"
	BackendUnreachable      BackendErrorCode = "unreachable"
	BackendIdentityMismatch BackendErrorCode = "identity_mismatch"
	BackendProtocolInvalid  BackendErrorCode = "protocol_invalid"
	BackendOutcomeUnknown   BackendErrorCode = "outcome_unknown"
)

type BackendError struct {
	Code       BackendErrorCode
	Diagnostic string
	Cause      error
}

func (e *BackendError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Diagnostic, e.Cause)
	}
	return string(e.Code) + ": " + e.Diagnostic
}
func (e *BackendError) Unwrap() error { return e.Cause }

// ExecutionBackend is the shared boundary for platform operations. Optional
// operations return a typed unsupported error; they never succeed as no-ops.
type ExecutionBackend interface {
	Identity() (BackendKind, ExecutionProtocol)
	Probe(context.Context, ExecutionTarget, AgentSpec) (CapabilityReport, error)
	Start(context.Context, AttemptSpec) (StartReceipt, error)
	Observe(context.Context, ExecutionRef) (Observation, error)
	InspectWorkspace(context.Context, ExecutionRef) (WorkspaceInspection, error)
	ReadOutput(context.Context, ExecutionRef, string, int) (OutputBatch, error)
	Reply(context.Context, ExecutionRef, string, Operation) (ReplyReceipt, error)
	StopExecution(context.Context, ExecutionRef, Operation) (StopReceipt, error)
	DetachObservation(context.Context, *Worker) error
	SettleResources(context.Context, ExecutionRef, string) ([]string, error)
	ReconcileOperation(context.Context, Operation) (Operation, error)
}

type WorkspaceInspection struct {
	Repository string    `json:"repository"`
	Worktree   string    `json:"worktree"`
	Base       string    `json:"base"`
	Head       string    `json:"head"`
	Dirty      bool      `json:"dirty"`
	CheckedAt  time.Time `json:"checked_at"`
}
type ReplyReceipt struct {
	InputAccepted bool   `json:"input_accepted"`
	TurnStarted   bool   `json:"turn_started"`
	Applied       bool   `json:"applied"`
	Diagnostic    string `json:"diagnostic,omitempty"`
}

func unsupported(name string) error {
	return &BackendError{Code: BackendUnsupported, Diagnostic: name + " is not supported by this backend"}
}
func digestValue(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type executionRecords struct{ root string }

func stableExecutionID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func stableOperationID(value string) bool {
	if value == "" || len(value) > 160 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (r executionRecords) bindingPath(run, attempt string) string {
	return filepath.Join(r.root, "execution", "v1", "bindings", run, attempt+".json")
}
func (r executionRecords) operationPath(run, attempt, id string) string {
	return filepath.Join(r.root, "execution", "v1", "operations", run, attempt, id+".json")
}
func (r executionRecords) saveCapabilityReport(report CapabilityReport) error {
	if report.Version != 1 || report.Backend == "" || report.Protocol == "" || report.Target.ID == "" || report.Agent.Name == "" || report.CheckedAt.IsZero() {
		return errors.New("incomplete capability report")
	}
	key := digestValue(struct {
		Backend  BackendKind
		Protocol ExecutionProtocol
		Target   ExecutionTarget
		Agent    AgentSpec
		Version  string
	}{report.Backend, report.Protocol, report.Target, report.Agent, report.PlatformVersion})
	p := filepath.Join(r.root, "execution", "v1", "capabilities", key+".json")
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	return AtomicWrite(p, append(b, '\n'))
}
func (r executionRecords) saveBinding(ref ExecutionRef) error {
	if ref.Version != ExecutionRecordVersion || !stableExecutionID(ref.RunID) || !stableExecutionID(ref.AttemptID) || ref.Backend == "" || ref.Protocol == "" {
		return errors.New("incomplete execution binding")
	}
	if ref.Target.ID == "" || ref.Agent.Name == "" || ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || !commitHash.MatchString(ref.BaseCommit) {
		return errors.New("execution binding lacks a verified target, agent or workspace")
	}
	switch {
	case ref.Backend == BackendLocal && ref.Protocol == ProtocolLocalProcess:
		if ref.External.Kind != "os" || ref.External.PID <= 0 || ref.External.Created == "" || ref.External.Executable == "" {
			return errors.New("local execution binding lacks a verified process identity")
		}
	case ref.Backend == BackendOrca && ref.Protocol == ProtocolOrcaTerminalV2:
		if ref.External.Kind != "orca" || ref.External.Terminal == "" || ref.External.RuntimeID == "" || ref.External.Incarnation == "" {
			return errors.New("Orca execution binding lacks runtime/terminal/incarnation identity")
		}
	default:
		return errors.New("unsupported backend/protocol binding")
	}
	p := r.bindingPath(ref.RunID, ref.AttemptID)
	if old, err := r.loadBinding(ref.RunID, ref.AttemptID); err == nil {
		if digestValue(old) != digestValue(ref) {
			return errors.New("execution binding is immutable and conflicts with existing record")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	b, err := json.MarshalIndent(ref, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	return AtomicWrite(p, append(b, '\n'))
}
func (r executionRecords) loadBinding(run, attempt string) (ExecutionRef, error) {
	if !stableExecutionID(run) || !stableExecutionID(attempt) {
		return ExecutionRef{}, errors.New("invalid execution binding path identity")
	}
	b, err := os.ReadFile(r.bindingPath(run, attempt))
	if err != nil {
		return ExecutionRef{}, err
	}
	var ref ExecutionRef
	if err = json.Unmarshal(b, &ref); err != nil {
		return ref, fmt.Errorf("invalid execution binding; preserve it: %w", err)
	}
	if ref.Version != ExecutionRecordVersion || ref.RunID != run || ref.AttemptID != attempt {
		return ref, errors.New("execution binding identity/version mismatch")
	}
	return ref, nil
}
func (r executionRecords) saveOperation(op Operation) error {
	if op.Version != ExecutionRecordVersion || !stableOperationID(op.ID) || !stableExecutionID(op.RunID) || !stableExecutionID(op.AttemptID) || len(op.PayloadDigest) != 64 {
		return errors.New("incomplete operation record")
	}
	p := r.operationPath(op.RunID, op.AttemptID, op.ID)
	if old, err := r.loadOperation(op.RunID, op.AttemptID, op.ID); err == nil {
		if old.PayloadDigest != op.PayloadDigest || old.Action != op.Action || old.Backend != op.Backend || old.Protocol != op.Protocol || old.Target != op.Target {
			return errors.New("operation id reused with different fixed payload")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	b, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	return AtomicWrite(p, append(b, '\n'))
}
func (r executionRecords) loadOperation(run, attempt, id string) (Operation, error) {
	if !stableExecutionID(run) || !stableExecutionID(attempt) || !stableOperationID(id) {
		return Operation{}, errors.New("invalid operation path identity")
	}
	b, err := os.ReadFile(r.operationPath(run, attempt, id))
	if err != nil {
		return Operation{}, err
	}
	var op Operation
	if err = json.Unmarshal(b, &op); err != nil {
		return op, fmt.Errorf("invalid operation record; preserve it: %w", err)
	}
	if op.Version != ExecutionRecordVersion || op.RunID != run || op.AttemptID != attempt || op.ID != id {
		return op, errors.New("operation identity/version mismatch")
	}
	return op, nil
}

type localExecutionBackend struct {
	config       Config
	source       BeadsContext
	afterPrepare func()
}

func (b localExecutionBackend) Identity() (BackendKind, ExecutionProtocol) {
	return BackendLocal, ProtocolLocalProcess
}
func (b localExecutionBackend) Probe(_ context.Context, target ExecutionTarget, agent AgentSpec) (CapabilityReport, error) {
	return capabilityReport(BackendLocal, ProtocolLocalProcess, target, agent, ""), nil
}
func (b localExecutionBackend) Start(ctx context.Context, s AttemptSpec) (StartReceipt, error) {
	r := cloneRun(s.Run)
	intent := s.Workspace
	c := b.config
	var err error
	if r.WorktreePath != nil {
		if err = verifyRunWorkspace(ctx, c, &r); err == nil {
			c, err = workerForWorkspace(ctx, c, &r, s.Beads)
		}
	} else {
		if _, statErr := os.Lstat(intent.Path); statErr == nil {
			return StartReceipt{}, &BackendError{Code: BackendRejected, Diagnostic: "local workspace path already exists; preserved without reuse"}
		} else if !os.IsNotExist(statErr) {
			return StartReceipt{}, &BackendError{Code: BackendRejected, Diagnostic: "local workspace path is inaccessible", Cause: statErr}
		}
		c, err = prepareWorkspace(ctx, c, &r, s.Beads)
	}
	if err != nil {
		residuals := []string(nil)
		if r.WorktreePath != nil {
			residuals = append(residuals, *r.WorktreePath)
		} else if _, statErr := os.Lstat(intent.Path); statErr == nil {
			residuals = append(residuals, intent.Path)
		}
		return StartReceipt{Disposition: "rejected", Run: r, ResidualResources: residuals}, &BackendError{Code: BackendRejected, Diagnostic: "local workspace preparation failed", Cause: err}
	}
	if b.afterPrepare != nil {
		b.afterPrepare()
	}
	path, branch, base := intent.Path, intent.Branch, intent.BaseCommit
	r.WorktreePath = &path
	r.WorktreeBranch = &branch
	r.BaseCommit = &base
	w, err := StartWorker(c, s.Task, &r)
	if err != nil {
		return StartReceipt{}, &BackendError{Code: BackendRejected, Diagnostic: "local process launch failed", Cause: err}
	}
	id := w.proc.Identity()
	ref := ExecutionRef{Version: 1, Backend: BackendLocal, Protocol: ProtocolLocalProcess, Target: s.Target, Agent: s.Agent, RunID: s.RunID, AttemptID: s.AttemptID, WorkspacePath: path, WorkspaceBranch: branch, BaseCommit: base, External: id, Generation: id.Created}
	return StartReceipt{Disposition: "accepted", Worker: w, Ref: ref, Run: r}, nil
}
func (b localExecutionBackend) Observe(ctx context.Context, ref ExecutionRef) (Observation, error) {
	if ref.Backend != BackendLocal || ref.Protocol != ProtocolLocalProcess || ref.Target != targetFor(b.config) {
		return Observation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "fixed local process target changed"}
	}
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	label := inspectProcess(&ref.External)
	liveness := "unverifiable"
	if label == "matching process still exists" {
		liveness = "live"
	} else if label == "process absent" {
		liveness = "exited"
	} else if strings.Contains(label, "different process") {
		return Observation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: label}
	}
	return Observation{Liveness: liveness, Connection: "connected", ObservedAt: now(), Source: "os-process-probe", Diagnostic: label}, nil
}
func (b localExecutionBackend) InspectWorkspace(ctx context.Context, ref ExecutionRef) (WorkspaceInspection, error) {
	if ref.Incomplete || ref.WorkspacePath == "" {
		return WorkspaceInspection{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "workspace identity is incomplete"}
	}
	head, e := gitRunner(b.config, ref.WorkspacePath).Output(ctx, b.config.Git, "rev-parse", "HEAD")
	if e != nil {
		return WorkspaceInspection{}, e
	}
	dirty, e := gitRunner(b.config, ref.WorkspacePath).Output(ctx, b.config.Git, "status", "--porcelain")
	if e != nil {
		return WorkspaceInspection{}, e
	}
	return WorkspaceInspection{Repository: b.config.Repository, Worktree: ref.WorkspacePath, Base: ref.BaseCommit, Head: strings.TrimSpace(string(head)), Dirty: strings.TrimSpace(string(dirty)) != "", CheckedAt: now()}, nil
}

type localOutputCursor struct {
	Version   int    `json:"v"`
	RunID     string `json:"r"`
	AttemptID string `json:"a"`
	Offset    int64  `json:"o"`
	Prefix    string `json:"p,omitempty"`
}

func (b localExecutionBackend) ReadOutput(ctx context.Context, ref ExecutionRef, cursor string, limit int) (OutputBatch, error) {
	if ref.Backend != BackendLocal || ref.Protocol != ProtocolLocalProcess || ref.Target != targetFor(b.config) || !stableExecutionID(ref.RunID) || !stableExecutionID(ref.AttemptID) {
		return OutputBatch{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "output ref does not match the fixed local execution"}
	}
	if limit <= 0 || limit > 1<<20 {
		return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "output limit must be between 1 and 1048576 bytes"}
	}
	path := filepath.Join(b.config.RunDir, "runs", ref.RunID)
	if ref.AttemptID != ref.RunID {
		path = filepath.Join(path, "attempts", ref.AttemptID)
	}
	path = filepath.Join(path, "events.jsonl")
	select {
	case <-ctx.Done():
		return OutputBatch{}, ctx.Err()
	default:
	}
	offset := int64(0)
	oldPrefix := ""
	if cursor != "" {
		token, decodeErr := base64.RawURLEncoding.DecodeString(cursor)
		if decodeErr != nil {
			return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "invalid output cursor", Cause: decodeErr}
		}
		var c localOutputCursor
		if decodeErr = json.Unmarshal(token, &c); decodeErr != nil || c.Version != 1 || c.RunID != ref.RunID || c.AttemptID != ref.AttemptID || c.Offset < 0 {
			return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "output cursor is malformed or belongs to another execution"}
		}
		offset, oldPrefix = c.Offset, c.Prefix
	}
	f, err := os.Open(path)
	if err != nil {
		return OutputBatch{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return OutputBatch{}, err
	}
	batch := OutputBatch{}
	if offset > info.Size() {
		batch.Gap = true
		offset = info.Size()
	}
	if oldPrefix != "" {
		prefix, prefixErr := readOutputPrefix(f, offset)
		if prefixErr != nil {
			return OutputBatch{}, prefixErr
		}
		if prefix != oldPrefix {
			batch.SourceChanged = true
			offset = 0
		}
	}
	end := offset + int64(limit)
	if end > info.Size() {
		end = info.Size()
	}
	chunk := make([]byte, end-offset)
	if len(chunk) > 0 {
		n, readErr := f.ReadAt(chunk, offset)
		if readErr != nil && readErr != io.EOF {
			return OutputBatch{}, readErr
		}
		chunk = chunk[:n]
	}
	batch.Data = bytes.Clone(chunk)
	end = offset + int64(len(chunk))
	prefix, prefixErr := readOutputPrefix(f, end)
	if prefixErr != nil {
		return OutputBatch{}, prefixErr
	}
	next := localOutputCursor{Version: 1, RunID: ref.RunID, AttemptID: ref.AttemptID, Offset: end, Prefix: prefix}
	encoded, _ := json.Marshal(next)
	batch.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	return batch, nil
}

func readOutputPrefix(f *os.File, offset int64) (string, error) {
	n := offset
	if n > 64<<10 {
		n = 64 << 10
	}
	if n < 0 {
		n = 0
	}
	first := make([]byte, n)
	last := make([]byte, n)
	if n > 0 {
		read, err := f.ReadAt(first, 0)
		if err != nil && err != io.EOF {
			return "", err
		}
		first = first[:read]
		read, err = f.ReadAt(last, offset-n)
		if err != nil && err != io.EOF {
			return "", err
		}
		last = last[:read]
	}
	hash := sha256.New()
	_, _ = hash.Write(first)
	_, _ = hash.Write(last)
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func (b localExecutionBackend) Reply(context.Context, ExecutionRef, string, Operation) (ReplyReceipt, error) {
	return ReplyReceipt{}, unsupported("Reply")
}
func (b localExecutionBackend) StopExecution(context.Context, ExecutionRef, Operation) (StopReceipt, error) {
	return StopReceipt{}, unsupported("StopExecution requires service shutdown policy")
}
func (b localExecutionBackend) DetachObservation(context.Context, *Worker) error {
	// Local stdout supervision owns the worker pipe. Closing it can signal the
	// child and is not equivalent to detaching observation.
	return unsupported("DetachObservation while a local process is running")
}
func (b localExecutionBackend) SettleResources(context.Context, ExecutionRef, string) ([]string, error) {
	return nil, unsupported("SettleResources")
}
func (b localExecutionBackend) ReconcileOperation(context.Context, Operation) (Operation, error) {
	return Operation{}, unsupported("local operation reconciliation")
}

type orcaTerminalBackend struct{ config Config }

func (b orcaTerminalBackend) Identity() (BackendKind, ExecutionProtocol) {
	return BackendOrca, ProtocolOrcaTerminalV2
}
func (b orcaTerminalBackend) Probe(_ context.Context, target ExecutionTarget, agent AgentSpec) (CapabilityReport, error) {
	return capabilityReport(BackendOrca, ProtocolOrcaTerminalV2, target, agent, ""), nil
}
func (b orcaTerminalBackend) Start(ctx context.Context, s AttemptSpec) (StartReceipt, error) {
	r := cloneRun(s.Run)
	intent := s.Workspace
	w, err := startOrcaRun(ctx, b.config, s.Task, &r, s.Beads, &intent, filepath.Dir(deref(r.ResultPath)))
	if err != nil {
		return StartReceipt{Disposition: "outcome_unknown", Run: r, ResidualResources: []string{"Orca worktree or terminal may have been created"}}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca start may have created external resources", Cause: err}
	}
	r.WorkspaceIntent = &intent
	id := w.proc.Identity()
	ref := ExecutionRef{Version: 1, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: s.Target, Agent: s.Agent, RunID: s.RunID, AttemptID: s.AttemptID, WorkspacePath: deref(r.WorktreePath), WorkspaceBranch: deref(r.WorktreeBranch), BaseCommit: deref(r.BaseCommit), External: id, Generation: id.Incarnation}
	return StartReceipt{Disposition: "accepted", Worker: w, Ref: ref, Run: r}, nil
}
func (b orcaTerminalBackend) Observe(ctx context.Context, ref ExecutionRef) (Observation, error) {
	old := ref.External
	if old.Kind != "orca" || old.Terminal == "" {
		return Observation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca terminal binding is incomplete"}
	}
	if targetFor(b.config) != ref.Target {
		return Observation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "configured Orca connection differs from the fixed execution target"}
	}
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	term, e := orcaClient{cfg: b.config}.showTerminal(ctx, old.Terminal)
	if e != nil {
		return Observation{}, &BackendError{Code: BackendUnreachable, Diagnostic: "could not inspect fixed Orca terminal", Cause: e}
	}
	if old.RuntimeID == "" || old.Incarnation == "" || term.RuntimeID == "" || term.IncarnationID == "" {
		return Observation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca runtime/incarnation evidence is incomplete"}
	}
	if old.RuntimeID != term.RuntimeID || old.Incarnation != term.IncarnationID {
		return Observation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca terminal runtime/incarnation changed"}
	}
	life := "live"
	if term.Status == "exited" {
		life = "exited"
	} else if term.Orphaned || !term.Connected {
		life = "unverifiable"
	}
	return Observation{Liveness: life, Connection: map[bool]string{true: "connected", false: "unreachable"}[term.Connected], ObservedAt: now(), Source: "orca-terminal-v2", Diagnostic: term.Status}, nil
}
func (b orcaTerminalBackend) InspectWorkspace(context.Context, ExecutionRef) (WorkspaceInspection, error) {
	return WorkspaceInspection{}, unsupported("Orca remote workspace inspection has not been natively verified")
}
func (b orcaTerminalBackend) ReadOutput(context.Context, ExecutionRef, string, int) (OutputBatch, error) {
	return OutputBatch{}, unsupported("ReadOutput (4B source cursor is not implemented)")
}
func (b orcaTerminalBackend) Reply(ctx context.Context, ref ExecutionRef, text string, _ Operation) (ReplyReceipt, error) {
	if ref.Backend != BackendOrca || ref.Protocol != ProtocolOrcaTerminalV2 || ref.External.Terminal == "" {
		return ReplyReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca terminal binding is incomplete"}
	}
	if targetFor(b.config) != ref.Target {
		return ReplyReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "configured Orca connection differs from the fixed execution target"}
	}
	if _, err := (orcaTerminalBackend{config: b.config}).Observe(ctx, ref); err != nil {
		return ReplyReceipt{}, err
	}
	if err := (orcaClient{cfg: b.config}).terminalSend(ctx, ref.External.Terminal, text); err != nil {
		return ReplyReceipt{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca may have accepted the continuation before the response failed", Cause: err}
	}
	return ReplyReceipt{InputAccepted: true}, nil
}
func (b orcaTerminalBackend) StopExecution(context.Context, ExecutionRef, Operation) (StopReceipt, error) {
	return StopReceipt{Disposition: "rejected", Diagnostic: "Orca terminal v2 stop behavior is not verified; use explicit service shutdown/manual recovery"}, unsupported("StopExecution")
}
func (b orcaTerminalBackend) DetachObservation(ctx context.Context, w *Worker) error {
	if w == nil {
		return nil
	}
	proc, ok := w.proc.(*orcaProcess)
	if !ok {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "worker does not belong to Orca terminal v2"}
	}
	proc.Close() // cancels only the local observer; it does not close the Orca terminal.
	select {
	case <-w.Done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (b orcaTerminalBackend) SettleResources(context.Context, ExecutionRef, string) ([]string, error) {
	return nil, unsupported("SettleResources")
}
func (b orcaTerminalBackend) ReconcileOperation(context.Context, Operation) (Operation, error) {
	return Operation{}, unsupported("Orca terminal v2 operation reconciliation")
}

func capabilityReport(backend BackendKind, protocol ExecutionProtocol, target ExecutionTarget, agent AgentSpec, version string) CapabilityReport {
	return CapabilityReport{Version: 1, Backend: backend, Protocol: protocol, Target: target, Agent: agent, PlatformVersion: version, CheckedAt: now(), Capabilities: []Capability{{Name: "start", Status: CapabilityNotTested}, {Name: "observe", Status: CapabilityNotTested}, {Name: "stop", Status: CapabilityNotTested}, {Name: "reply_continue", Status: CapabilityNotTested}, {Name: "retry", Status: CapabilityNotTested}, {Name: "operation_reconciliation", Status: CapabilityNotTested}, {Name: "workspace_inspection", Status: CapabilityNotTested}, {Name: "output_reading", Status: CapabilityNotTested}}}
}
func backendFor(c Config, source BeadsContext) ExecutionBackend {
	if c.hostName() == "orca" {
		return orcaTerminalBackend{config: c}
	}
	return localExecutionBackend{config: c, source: source}
}
func cloneRun(r Run) Run {
	b, _ := json.Marshal(r)
	var copy Run
	_ = json.Unmarshal(b, &copy)
	return copy
}
func agentFor(c Config) AgentSpec { return AgentSpec{Name: c.agentName()} }
func targetFor(c Config) ExecutionTarget {
	if c.hostName() == "orca" {
		return ExecutionTarget{ID: "orca:" + digestValue(struct {
			Executable string
			Args       []string
		}{c.Orca.Executable, c.Orca.Args})[:24]}
	}
	name, _ := os.Hostname()
	return ExecutionTarget{ID: "local:" + runtime.GOOS + ":" + name}
}
func attemptOperationID(run, attempt string) string { return "start-" + run + "-" + attempt }
func executionRefFor(c Config, run *Run) ExecutionRef {
	if run == nil {
		return ExecutionRef{Version: 1, Legacy: true, Incomplete: true}
	}
	if run.Process == nil {
		return ExecutionRef{Version: 1, RunID: run.RunID, AttemptID: run.AttemptID, Legacy: true, Incomplete: true}
	}
	b, p := BackendLocal, ProtocolLocalProcess
	if run.Process.Kind == "orca" {
		b, p = BackendOrca, ProtocolOrcaTerminalV2
	}
	return ExecutionRef{Version: 1, Backend: b, Protocol: p, Target: targetFor(c), Agent: agentFor(c), RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: deref(run.WorktreePath), WorkspaceBranch: deref(run.WorktreeBranch), BaseCommit: deref(run.BaseCommit), External: *run.Process, Generation: run.Process.Created, Legacy: true}
}

func (s *Service) inspectRunProcess(run *Run) string {
	if run == nil || run.Process == nil {
		return "execution identity unavailable"
	}
	records := executionRecords{root: s.Config.RunDir}
	ref, err := records.loadBinding(run.RunID, run.AttemptID)
	if err == nil {
		if ref.External != *run.Process || ref.Backend == "" || ref.Protocol == "" || ref.Target.ID == "" {
			return "execution binding conflicts with runtime identity"
		}
		if ref.Target != targetFor(s.Config) {
			return "execution target changed; refusing to inspect through a replacement backend connection"
		}
		if ref.Backend == BackendLocal {
			return inspectProcess(&ref.External)
		}
		obs, observeErr := (orcaTerminalBackend{config: s.Config}).Observe(context.Background(), ref)
		if observeErr != nil {
			return "orca execution identity unverifiable: " + observeErr.Error()
		}
		if obs.Liveness == "live" {
			return "matching process still exists"
		}
		if obs.Liveness == "exited" {
			return "orca terminal exited"
		}
		return "orca terminal unverifiable: " + obs.Diagnostic
	}
	if !os.IsNotExist(err) {
		return "execution binding invalid: " + err.Error()
	}
	// Legacy records are inspected only through their recorded identity. The
	// current Orca executable must match the executable recorded with the ref.
	if run.Process.Kind == "orca" && run.Process.Executable != "" && run.Process.Executable != s.Config.Orca.Executable {
		return "legacy Orca connection changed; refusing to route an old terminal through it"
	}
	return inspectProcessWithConfig(s.Config, run.Process)
}
