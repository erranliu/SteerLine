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
	"strconv"
	"strings"
	"time"
)

// Backend and protocol are deliberately independent; persisted refs carry both.
type BackendKind string
type ExecutionProtocol string

const (
	BackendLocal                BackendKind       = "local"
	BackendOrca                 BackendKind       = "orca"
	ProtocolLocalProcess        ExecutionProtocol = "local_process"
	ProtocolLocalCodexAppServer ExecutionProtocol = "local_codex_app_server_v1"
	ProtocolOrcaTerminalV2      ExecutionProtocol = "orca_terminal_v2"
	ProtocolOrcaSupervised      ExecutionProtocol = "orca_supervised"
	ExecutionRecordVersion                        = 1
)

type AgentSpec struct {
	Name string `json:"name"`
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
	Version              int               `json:"version"`
	Backend              BackendKind       `json:"backend"`
	Protocol             ExecutionProtocol `json:"protocol"`
	Agent                AgentSpec         `json:"agent"`
	Target               ExecutionTarget   `json:"target"`
	CheckedAt            time.Time         `json:"checked_at"`
	CenterVersion        string            `json:"center_version,omitempty"`
	PlatformVersion      string            `json:"platform_version,omitempty"`
	ExpectedAgentVersion string            `json:"expected_agent_version,omitempty"`
	AgentVersion         string            `json:"agent_version,omitempty"`
	Capabilities         []Capability      `json:"capabilities"`
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
	PlatformRequestID string       `json:"platform_request_id,omitempty"`
	Worker            *Worker      `json:"-"`
	Ref               ExecutionRef `json:"ref"`
	Run               Run          `json:"run"`
	ResidualResources []string     `json:"residual_resources,omitempty"`
}
type StopReceipt struct {
	Disposition string `json:"disposition"`
	Diagnostic  string `json:"diagnostic,omitempty"`
}
type SettlementReceipt struct {
	Disposition       string   `json:"disposition"`
	PlatformRequestID string   `json:"platform_request_id,omitempty"`
	ResidualResources []string `json:"residual_resources,omitempty"`
	Diagnostic        string   `json:"diagnostic,omitempty"`
}
type OutputBatch struct {
	Data          []byte    `json:"data"`
	Source        string    `json:"source"`
	RetainedFrom  string    `json:"retained_from,omitempty"`
	ObservedAt    time.Time `json:"observed_at"`
	NextCursor    string    `json:"next_cursor,omitempty"`
	Gap           bool      `json:"gap,omitempty"`
	SourceChanged bool      `json:"source_changed,omitempty"`
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
	Observe(context.Context, ExecutionRef) (Observation, error)
	InspectWorkspace(context.Context, ExecutionRef) (WorkspaceInspection, error)
	ReadOutput(context.Context, ExecutionRef, string, int) (OutputBatch, error)
	StopExecution(context.Context, ExecutionRef, Operation, *Worker) (StopReceipt, error)
	DetachObservation(context.Context, *Worker) error
	SettleResources(context.Context, ExecutionRef, string) (SettlementReceipt, error)
	ReconcileOperation(context.Context, Operation) (Operation, error)
}

type WorkspaceInspection struct {
	Repository string    `json:"repository"`
	Worktree   string    `json:"worktree"`
	Branch     string    `json:"branch"`
	Base       string    `json:"base"`
	Head       string    `json:"head"`
	Dirty      bool      `json:"dirty"`
	CheckedAt  time.Time `json:"checked_at"`
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

func capabilityReportKey(report CapabilityReport) string {
	return digestValue(struct {
		Backend              BackendKind
		Protocol             ExecutionProtocol
		Target               ExecutionTarget
		Agent                AgentSpec
		CenterVersion        string
		Version              string
		ExpectedAgentVersion string
		AgentVersion         string
	}{report.Backend, report.Protocol, report.Target, report.Agent, report.CenterVersion, report.PlatformVersion, report.ExpectedAgentVersion, report.AgentVersion})
}

func verifyCapabilityEvidence(value string) error {
	path, digest, found := strings.Cut(value, "#sha256=")
	if !found || !filepath.IsAbs(path) || len(digest) != 64 {
		return errors.New("verified capability lacks an absolute content-bound evidence reference")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return errors.New("verified capability evidence digest is invalid")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("capability evidence is unavailable: %w", err)
	}
	if len(data) == 0 || len(data) > 64<<10 {
		return errors.New("capability evidence size is invalid")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != digest {
		return errors.New("capability evidence content changed")
	}
	return nil
}

func (r executionRecords) capabilityPath(report CapabilityReport) string {
	return filepath.Join(r.root, "execution", "v1", "capabilities", capabilityReportKey(report)+".json")
}

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

func (r executionRecords) saveCapabilityReport(report CapabilityReport) error {
	if report.Version != 1 || report.Backend == "" || report.Protocol == "" || report.Target.ID == "" || report.Agent.Name == "" || report.PlatformVersion == "" || report.CheckedAt.IsZero() || report.Protocol == ProtocolOrcaSupervised && report.CenterVersion == "" {
		return errors.New("incomplete capability report")
	}
	seen := map[string]bool{}
	for _, cap := range report.Capabilities {
		if cap.Name == "" || retiredExecutionCapabilityName(cap.Name) || seen[cap.Name] || cap.Status != CapabilityVerified && cap.Status != CapabilityUnsupported && cap.Status != CapabilityNotTested || cap.Status == CapabilityVerified && cap.Evidence == "" {
			return errors.New("capability report has invalid or unevidenced status")
		}
		if cap.Status == CapabilityVerified {
			if err := verifyCapabilityEvidence(cap.Evidence); err != nil {
				return err
			}
		}
		seen[cap.Name] = true
	}
	p := r.capabilityPath(report)
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	return AtomicWrite(p, append(b, '\n'))
}

func (r executionRecords) loadCapabilityReport(key CapabilityReport) (CapabilityReport, error) {
	b, err := os.ReadFile(r.capabilityPath(key))
	if err != nil {
		return CapabilityReport{}, err
	}
	var report CapabilityReport
	if err = decode(b, &report); err != nil {
		return report, fmt.Errorf("invalid capability report; preserve it: %w", err)
	}
	if report.Version != 1 || report.Backend != key.Backend || report.Protocol != key.Protocol || report.Target != key.Target || report.Agent != key.Agent || report.CenterVersion != key.CenterVersion || report.PlatformVersion != key.PlatformVersion || report.ExpectedAgentVersion != key.ExpectedAgentVersion || report.AgentVersion != key.AgentVersion || report.CheckedAt.IsZero() {
		return report, errors.New("capability report identity/version mismatch")
	}
	if len(report.Capabilities) == 0 {
		return report, errors.New("capability report has no checked capabilities")
	}
	seen := map[string]bool{}
	for _, cap := range report.Capabilities {
		if cap.Name == "" || retiredExecutionCapabilityName(cap.Name) || seen[cap.Name] || cap.Status != CapabilityVerified && cap.Status != CapabilityUnsupported && cap.Status != CapabilityNotTested || cap.Status == CapabilityVerified && cap.Evidence == "" {
			return report, errors.New("capability report contains invalid or unevidenced status")
		}
		if cap.Status == CapabilityVerified {
			if err := verifyCapabilityEvidence(cap.Evidence); err != nil {
				return report, err
			}
		}
		seen[cap.Name] = true
	}
	return report, nil
}

func capabilityStatus(report CapabilityReport, name string) CapabilityStatus {
	for _, cap := range report.Capabilities {
		if cap.Name == name {
			return cap.Status
		}
	}
	return CapabilityNotTested
}

func (s *Service) requireBackendCapabilities(ctx context.Context, backend ExecutionBackend, names ...string) (CapabilityReport, error) {
	return s.requireBackendCapabilitiesFor(ctx, backend, targetFor(s.Config), agentFor(s.Config), names...)
}

func (s *Service) requireBackendCapabilitiesFor(ctx context.Context, backend ExecutionBackend, target ExecutionTarget, agent AgentSpec, names ...string) (CapabilityReport, error) {
	probe, err := backend.Probe(ctx, target, agent)
	if err != nil {
		return CapabilityReport{}, err
	}
	records := executionRecords{root: s.Config.RunDir}
	report, err := records.loadCapabilityReport(probe)
	if os.IsNotExist(err) {
		if saveErr := records.saveCapabilityReport(probe); saveErr != nil {
			return CapabilityReport{}, saveErr
		}
		report = probe
	} else if err != nil {
		return CapabilityReport{}, err
	}
	for _, name := range names {
		if capabilityStatus(report, name) != CapabilityVerified {
			return report, &BackendError{Code: BackendUnsupported, Diagnostic: fmt.Sprintf("%s capability is %s for %s/%s target %s version %s", name, capabilityStatus(report, name), report.Backend, report.Protocol, report.Target.ID, report.PlatformVersion)}
		}
	}
	return report, nil
}
func validateExecutionRef(ref ExecutionRef) error {
	if ref.Version != ExecutionRecordVersion || !stableExecutionID(ref.RunID) || !stableExecutionID(ref.AttemptID) || ref.Backend == "" || ref.Protocol == "" {
		return errors.New("incomplete execution binding")
	}
	if ref.Target.ID == "" || ref.Agent.Name == "" || !commitHash.MatchString(ref.BaseCommit) {
		return errors.New("execution binding lacks a verified target, agent or workspace")
	}
	switch {
	case ref.Backend == BackendLocal && (ref.Protocol == ProtocolLocalProcess || ref.Protocol == ProtocolLocalCodexAppServer):
		if ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || ref.External.Kind != "os" || ref.External.PID <= 0 || ref.External.Created == "" || ref.External.Executable == "" {
			return errors.New("local execution binding lacks a verified process identity")
		}
	case ref.Backend == BackendOrca && ref.Protocol == ProtocolOrcaTerminalV2:
		if ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || ref.External.Kind != "orca" || ref.External.Terminal == "" || ref.External.RuntimeID == "" || ref.External.Incarnation == "" {
			return errors.New("Orca execution binding lacks runtime/terminal/incarnation identity")
		}
	case ref.Backend == BackendOrca && ref.Protocol == ProtocolOrcaSupervised:
		if ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || ref.External.Kind != "orca_supervised" || ref.External.RuntimeID == "" || ref.External.OrchestrationRun == "" || ref.External.Task == "" || ref.External.Dispatch == "" || ref.External.ExecutionHost == "" || ref.External.WorktreeID == "" {
			return errors.New("supervised Orca execution binding lacks exact runtime/Run/Task/Dispatch/host/worktree identity")
		}
	default:
		return errors.New("unsupported backend/protocol binding")
	}
	return nil
}

func (r executionRecords) saveBinding(ref ExecutionRef) error {
	if err := validateExecutionRef(ref); err != nil {
		return err
	}
	ref.WorkspacePath = canonical(ref.WorkspacePath)
	if old, err := r.loadBinding(ref.RunID, ref.AttemptID); err == nil {
		old.WorkspacePath = canonical(old.WorkspacePath)
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
	return r.writeRecord("execution_bindings", ref.RunID, ref.AttemptID, "binding", b)
}
func (r executionRecords) loadBinding(run, attempt string) (ExecutionRef, error) {
	if !stableExecutionID(run) || !stableExecutionID(attempt) {
		return ExecutionRef{}, errors.New("invalid execution binding path identity")
	}
	b, err := r.readRecord("execution_bindings", run, attempt, "binding")
	if err != nil {
		return ExecutionRef{}, err
	}
	var ref ExecutionRef
	if err = decode(b, &ref); err != nil {
		return ref, fmt.Errorf("invalid execution binding; preserve it: %w", err)
	}
	if ref.Version != ExecutionRecordVersion || ref.RunID != run || ref.AttemptID != attempt {
		return ref, errors.New("execution binding identity/version mismatch")
	}
	if err := validateExecutionRef(ref); err != nil {
		return ref, fmt.Errorf("invalid execution binding; preserve it: %w", err)
	}
	return ref, nil
}
func validateExecutionOperation(op Operation) error {
	if op.Version != ExecutionRecordVersion || !stableOperationID(op.ID) || !stableExecutionID(op.RunID) || !stableExecutionID(op.AttemptID) || len(op.PayloadDigest) != 64 || op.Action == "" || op.Backend == "" || op.Protocol == "" || op.Target.ID == "" {
		return errors.New("incomplete operation record")
	}
	if _, err := hex.DecodeString(op.PayloadDigest); err != nil {
		return errors.New("invalid operation payload digest")
	}
	if op.Action != "start" && op.Action != "stop" && op.Action != "reply" {
		return errors.New("unsupported execution operation action")
	}
	switch op.Status {
	case OperationPending, OperationInFlight, OperationAccepted, OperationConfirmed, OperationRejected, OperationUnknown:
	default:
		return errors.New("invalid operation status")
	}
	if op.Ref != nil {
		if err := validateExecutionRef(*op.Ref); err != nil {
			return err
		}
		if op.Ref.RunID != op.RunID || op.Ref.AttemptID != op.AttemptID || op.Ref.Backend != op.Backend || op.Ref.Protocol != op.Protocol || op.Ref.Target != op.Target {
			return errors.New("operation ref conflicts with fixed identity")
		}
	}
	return nil
}

func (r executionRecords) saveOperation(op Operation) error {
	if err := validateExecutionOperation(op); err != nil {
		return err
	}
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
	return r.writeRecord("execution_operations", op.RunID, op.AttemptID, op.ID, b)
}
func (r executionRecords) loadOperation(run, attempt, id string) (Operation, error) {
	if !stableExecutionID(run) || !stableExecutionID(attempt) || !stableOperationID(id) {
		return Operation{}, errors.New("invalid operation path identity")
	}
	b, err := r.readRecord("execution_operations", run, attempt, id)
	if err != nil {
		return Operation{}, err
	}
	var op Operation
	if err = decode(b, &op); err != nil {
		return op, fmt.Errorf("invalid operation record; preserve it: %w", err)
	}
	if op.Version != ExecutionRecordVersion || op.RunID != run || op.AttemptID != attempt || op.ID != id {
		return op, errors.New("operation identity/version mismatch")
	}
	if err := validateExecutionOperation(op); err != nil {
		return op, fmt.Errorf("invalid operation record; preserve it: %w", err)
	}
	return op, nil
}

type localExecutionBackend struct {
	config Config
}

func (b localExecutionBackend) Identity() (BackendKind, ExecutionProtocol) {
	return BackendLocal, ProtocolLocalProcess
}
func (b localExecutionBackend) Probe(ctx context.Context, target ExecutionTarget, agent AgentSpec) (CapabilityReport, error) {
	if agent.Name != "codex" {
		return CapabilityReport{}, unsupported("local_process agent " + agent.Name)
	}
	out, err := (Runner{Dir: b.config.Repository, Timeout: positiveDuration(b.config.CommandTimeout, 30*time.Second)}).Output(ctx, b.config.Codex, "--version")
	if err != nil {
		return CapabilityReport{}, &BackendError{Code: BackendUnreachable, Diagnostic: "local Codex version probe failed", Cause: err}
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return CapabilityReport{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "local Codex version probe returned no version"}
	}
	return capabilityReport(BackendLocal, ProtocolLocalProcess, target, agent, version), nil
}
func (b localExecutionBackend) Observe(ctx context.Context, ref ExecutionRef) (Observation, error) {
	if ref.Backend != BackendLocal || ref.Protocol != ProtocolLocalProcess && ref.Protocol != ProtocolLocalCodexAppServer || ref.Target != targetFor(b.config) {
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
	if ref.Backend != BackendLocal || ref.Protocol != ProtocolLocalProcess && ref.Protocol != ProtocolLocalCodexAppServer || ref.Target != targetFor(b.config) {
		return WorkspaceInspection{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "local workspace target differs from the fixed execution"}
	}
	return inspectBoundWorkspace(ctx, b.config, ref)
}

func inspectBoundWorkspace(ctx context.Context, c Config, ref ExecutionRef) (WorkspaceInspection, error) {
	if ref.Incomplete || ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || !commitHash.MatchString(ref.BaseCommit) {
		return WorkspaceInspection{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "workspace ref is incomplete"}
	}
	if c.identity == "" {
		if err := c.ResolveIdentity(ctx); err != nil {
			return WorkspaceInspection{}, err
		}
	}
	intent := &WorkspaceIntent{Path: ref.WorkspacePath, Branch: ref.WorkspaceBranch, BaseCommit: ref.BaseCommit}
	if err := verifyWorkspace(ctx, c, intent, false); err != nil {
		return WorkspaceInspection{}, err
	}
	runner := gitRunner(c, ref.WorkspacePath)
	head, err := runner.Output(ctx, c.Git, "rev-parse", "HEAD")
	if err != nil {
		return WorkspaceInspection{}, err
	}
	dirty, err := inspectGitDirty(ctx, c, ref.WorkspacePath)
	if err != nil {
		return WorkspaceInspection{}, err
	}
	return WorkspaceInspection{Repository: c.Repository, Worktree: ref.WorkspacePath, Branch: ref.WorkspaceBranch, Base: ref.BaseCommit, Head: strings.TrimSpace(string(head)), Dirty: dirty, CheckedAt: now()}, nil
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
	batch := OutputBatch{Source: "local-jsonl", RetainedFrom: "0", ObservedAt: now()}
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
func (b localExecutionBackend) StopExecution(ctx context.Context, ref ExecutionRef, _ Operation, worker *Worker) (StopReceipt, error) {
	if ref.Backend != BackendLocal || (ref.Protocol != ProtocolLocalProcess && ref.Protocol != ProtocolLocalCodexAppServer) || ref.Target != targetFor(b.config) {
		return StopReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "fixed local process target changed"}
	}
	if worker == nil || worker.proc == nil || worker.proc.Identity() != ref.External {
		return StopReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "local stop handle does not match the fixed process identity"}
	}
	if err := ctx.Err(); err != nil {
		return StopReceipt{}, err
	}
	obs, err := b.Observe(ctx, ref)
	if err != nil {
		return StopReceipt{}, err
	}
	if obs.Liveness == "exited" {
		return StopReceipt{Disposition: "confirmed", Diagnostic: obs.Diagnostic}, nil
	}
	if obs.Liveness != "live" {
		return StopReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "local process liveness cannot be verified"}
	}
	worker.Kill()
	return StopReceipt{Disposition: "accepted"}, nil
}
func (b localExecutionBackend) DetachObservation(ctx context.Context, w *Worker) error {
	return w.detachLocalObservation(ctx)
}
func (b localExecutionBackend) SettleResources(context.Context, ExecutionRef, string) (SettlementReceipt, error) {
	return SettlementReceipt{}, unsupported("SettleResources")
}
func reconcileRecordedOperation(ctx context.Context, root string, expected Operation) (Operation, error) {
	if err := ctx.Err(); err != nil {
		return Operation{}, err
	}
	records := executionRecords{root: root}
	old, err := records.loadOperation(expected.RunID, expected.AttemptID, expected.ID)
	if err != nil {
		return Operation{}, err
	}
	if old.PayloadDigest != expected.PayloadDigest || old.Action != expected.Action || old.Backend != expected.Backend || old.Protocol != expected.Protocol || old.Target != expected.Target {
		return Operation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "operation identity or fixed payload changed"}
	}
	if old.Ref != nil {
		bound, bindErr := records.loadBinding(old.RunID, old.AttemptID)
		if bindErr != nil {
			return Operation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "operation ref has no valid execution binding", Cause: bindErr}
		}
		if digestValue(bound) != digestValue(*old.Ref) {
			return Operation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "operation ref conflicts with the persisted binding"}
		}
	}
	if old.Status == OperationInFlight || old.Status == OperationUnknown {
		return old, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "platform request has no verified query/replay capability; original outcome remains unknown"}
	}
	return old, nil
}

func (b localExecutionBackend) ReconcileOperation(ctx context.Context, op Operation) (Operation, error) {
	return reconcileRecordedOperation(ctx, b.config.RunDir, op)
}

type orcaTerminalBackend struct{ config Config }

func (b orcaTerminalBackend) Identity() (BackendKind, ExecutionProtocol) {
	return BackendOrca, ProtocolOrcaTerminalV2
}
func (b orcaTerminalBackend) Probe(ctx context.Context, target ExecutionTarget, agent AgentSpec) (CapabilityReport, error) {
	if agent.Name == "" {
		return CapabilityReport{}, &BackendError{Code: BackendRejected, Diagnostic: "Orca agent is required"}
	}
	out, err := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "--version")
	if err != nil {
		return CapabilityReport{}, &BackendError{Code: BackendUnreachable, Diagnostic: "Orca version probe failed", Cause: err}
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return CapabilityReport{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "Orca version probe returned no version"}
	}
	report := capabilityReport(BackendOrca, ProtocolOrcaTerminalV2, target, agent, version)
	if b.config.needsCodexExecutable() {
		out, err := (Runner{Dir: b.config.Repository, Timeout: positiveDuration(b.config.CommandTimeout, 30*time.Second)}).Output(ctx, b.config.Codex, "--version")
		if err != nil || strings.TrimSpace(string(out)) == "" {
			return CapabilityReport{}, &BackendError{Code: BackendUnreachable, Diagnostic: "Orca Codex override version probe failed", Cause: err}
		}
		report.AgentVersion = strings.TrimSpace(string(out))
	}
	return report, nil
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
	term, e := inspectBoundOrcaTerminal(ctx, b.config, old)
	if e != nil {
		return Observation{}, e
	}
	life := "live"
	if term.Exited() {
		life = "exited"
	} else if !term.Connected {
		life = "unverifiable"
	}
	return Observation{Liveness: life, Connection: map[bool]string{true: "connected", false: "unreachable"}[term.Connected], ObservedAt: now(), Source: "orca-terminal-v2", Diagnostic: term.Status}, nil
}
func (b orcaTerminalBackend) InspectWorkspace(ctx context.Context, ref ExecutionRef) (WorkspaceInspection, error) {
	if ref.Backend != BackendOrca || ref.Protocol != ProtocolOrcaTerminalV2 || ref.Target != targetFor(b.config) {
		return WorkspaceInspection{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca workspace target differs from the fixed execution"}
	}
	// terminal-v2 worktrees live on this configured host. Future supervised
	// targets must implement their own remote inspection instead of using Git here.
	return inspectBoundWorkspace(ctx, b.config, ref)
}

type orcaOutputCursor struct {
	Version     int    `json:"v"`
	RunID       string `json:"r"`
	AttemptID   string `json:"a"`
	Target      string `json:"t"`
	Runtime     string `json:"u"`
	Terminal    string `json:"h"`
	Incarnation string `json:"i"`
	Source      string `json:"s"`
	Position    string `json:"p"`
}

func platformCursor(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value, nil
	}
	var number int64
	if err := json.Unmarshal(raw, &number); err == nil && number >= 0 {
		return strconv.FormatInt(number, 10), nil
	}
	return "", &BackendError{Code: BackendProtocolInvalid, Diagnostic: "Orca returned an invalid output cursor"}
}

func (b orcaTerminalBackend) ReadOutput(ctx context.Context, ref ExecutionRef, cursor string, limit int) (OutputBatch, error) {
	if ref.Backend != BackendOrca || ref.Protocol != ProtocolOrcaTerminalV2 || ref.Target != targetFor(b.config) || ref.External.Kind != "orca" || ref.External.Terminal == "" || ref.External.RuntimeID == "" || ref.External.Incarnation == "" {
		return OutputBatch{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "output ref lacks the fixed Orca terminal identity"}
	}
	if limit <= 0 || limit > 1000 {
		return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "Orca output limit must be 1–1000 rows"}
	}
	position := ""
	previousSource := ""
	if cursor != "" {
		token, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "invalid Orca output cursor", Cause: err}
		}
		var saved orcaOutputCursor
		if err = json.Unmarshal(token, &saved); err != nil || saved.Version != 1 || saved.RunID != ref.RunID || saved.AttemptID != ref.AttemptID || saved.Target != ref.Target.ID || saved.Runtime != ref.External.RuntimeID || saved.Terminal != ref.External.Terminal || saved.Incarnation != ref.External.Incarnation || saved.Source == "" {
			return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "Orca output cursor is malformed or belongs to another source"}
		}
		position, previousSource = saved.Position, saved.Source
	}
	if _, err := b.Observe(ctx, ref); err != nil {
		return OutputBatch{}, err
	}
	changed := false
	for attempt := 0; attempt < 2; attempt++ {
		args := []string{"terminal", "read", "--terminal", ref.External.Terminal, "--limit", strconv.Itoa(limit), "--json"}
		// Orca's omitted-cursor read is a tail preview. An initial collection
		// must request cursor zero to page every row still retained upstream.
		readPosition := position
		if readPosition == "" {
			readPosition = "0"
		}
		args = append(args, "--cursor", readPosition)
		out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), args...)
		env, err := parseEnvelope(out)
		if callErr != nil {
			return OutputBatch{}, &BackendError{Code: BackendUnreachable, Diagnostic: "Orca terminal output could not be read", Cause: errors.Join(callErr, err)}
		}
		if err != nil {
			return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "Orca terminal output response is invalid", Cause: err}
		}
		if env.Meta.RuntimeID == "" || env.Meta.RuntimeID != ref.External.RuntimeID {
			return OutputBatch{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca output came from a different runtime"}
		}
		var result struct {
			Source       string          `json:"source"`
			Tail         []string        `json:"tail"`
			NextCursor   json.RawMessage `json:"nextCursor"`
			OldestCursor json.RawMessage `json:"oldestCursor"`
		}
		var wrapped struct {
			Terminal json.RawMessage `json:"terminal"`
		}
		if err = json.Unmarshal(env.Result, &wrapped); err != nil {
			return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "Orca terminal output envelope is invalid", Cause: err}
		}
		payload := env.Result
		if len(wrapped.Terminal) != 0 {
			payload = wrapped.Terminal
		}
		if err = json.Unmarshal(payload, &result); err != nil {
			return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "Orca terminal output payload is invalid", Cause: err}
		}
		if result.Source == "" {
			return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "Orca output source is unavailable; source-bound paging cannot be verified"}
		}
		if previousSource != "" && previousSource != result.Source {
			changed = true
			position = ""
			previousSource = result.Source
			continue
		}
		if len(result.Tail) > limit {
			return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "Orca output exceeded the requested row bound"}
		}
		data := []byte(strings.Join(result.Tail, "\n"))
		if len(data) > 1<<20 {
			return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "Orca output exceeded the 1 MiB batch bound"}
		}
		next, err := platformCursor(result.NextCursor)
		if err != nil {
			return OutputBatch{}, err
		}
		oldest, err := platformCursor(result.OldestCursor)
		if err != nil {
			return OutputBatch{}, err
		}
		if next == "" {
			return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "Orca output omitted nextCursor"}
		}
		gap := false
		if position == "" && oldest != "" {
			if have, parseErr := strconv.ParseInt(oldest, 10, 64); parseErr == nil {
				gap = have > 0
			} else {
				gap = true
			}
		} else if position != "" && oldest != "" {
			want, werr := strconv.ParseInt(position, 10, 64)
			have, herr := strconv.ParseInt(oldest, 10, 64)
			if werr == nil && herr == nil && want < have {
				gap = true
			}
		}
		if _, err = b.Observe(ctx, ref); err != nil {
			return OutputBatch{}, err
		}
		token, _ := json.Marshal(orcaOutputCursor{Version: 1, RunID: ref.RunID, AttemptID: ref.AttemptID, Target: ref.Target.ID, Runtime: ref.External.RuntimeID, Terminal: ref.External.Terminal, Incarnation: ref.External.Incarnation, Source: result.Source, Position: next})
		return OutputBatch{Data: data, Source: result.Source, RetainedFrom: oldest, ObservedAt: now(), NextCursor: base64.RawURLEncoding.EncodeToString(token), Gap: gap, SourceChanged: changed}, nil
	}
	return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "Orca output source changed repeatedly during one read"}
}
func (b orcaTerminalBackend) StopExecution(ctx context.Context, ref ExecutionRef, _ Operation, worker *Worker) (StopReceipt, error) {
	obs, err := b.Observe(ctx, ref)
	if err != nil {
		return StopReceipt{}, err
	}
	if obs.Liveness == "exited" {
		return StopReceipt{Disposition: "confirmed", Diagnostic: obs.Diagnostic}, nil
	}
	if obs.Liveness != "live" {
		return StopReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca terminal is not verifiably live"}
	}
	if worker != nil && (worker.proc == nil || worker.proc.Identity() != ref.External) {
		return StopReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca observer handle does not match the fixed terminal identity"}
	}
	closeReceipt, err := (orcaClient{cfg: b.config}).closeTerminalChecked(ctx, ref.External.Terminal, ref.External.RuntimeID)
	if err != nil {
		return StopReceipt{}, err
	}
	if worker != nil {
		if proc, ok := worker.proc.(*orcaProcess); ok {
			proc.Close()
		}
	}
	if closeReceipt.Disposition == "confirmed" {
		return closeReceipt, nil
	}
	obs, err = b.Observe(ctx, ref)
	if err != nil {
		return StopReceipt{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "terminal close was accepted but stop could not be confirmed", Cause: err}
	}
	if obs.Liveness == "exited" {
		return StopReceipt{Disposition: "confirmed", Diagnostic: obs.Diagnostic}, nil
	}
	return StopReceipt{Disposition: "accepted", Diagnostic: "close accepted; terminal exit is not yet confirmed"}, nil
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
	if w.observationDone == nil {
		return unsupported("Orca observation completion signal")
	}
	select {
	case <-w.observationDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (b orcaTerminalBackend) SettleResources(context.Context, ExecutionRef, string) (SettlementReceipt, error) {
	return SettlementReceipt{}, unsupported("SettleResources")
}
func (b orcaTerminalBackend) ReconcileOperation(ctx context.Context, op Operation) (Operation, error) {
	return reconcileRecordedOperation(ctx, b.config.RunDir, op)
}

func capabilityReport(backend BackendKind, protocol ExecutionProtocol, target ExecutionTarget, agent AgentSpec, version string) CapabilityReport {
	caps := []Capability{{Name: "start", Status: CapabilityNotTested}, {Name: "observe", Status: CapabilityNotTested}, {Name: "stop", Status: CapabilityNotTested}, {Name: "detach_observation", Status: CapabilityNotTested}, {Name: "operation_reconciliation", Status: CapabilityNotTested}, {Name: "workspace_inspection", Status: CapabilityNotTested}, {Name: "output_reading", Status: CapabilityNotTested}, {Name: "output_turns", Status: CapabilityNotTested}, {Name: "output_final_answer", Status: CapabilityNotTested}, {Name: "resource_settlement", Status: CapabilityUnsupported}}
	return CapabilityReport{Version: 1, Backend: backend, Protocol: protocol, Target: target, Agent: agent, PlatformVersion: version, CheckedAt: now(), Capabilities: caps}
}

func retiredExecutionCapabilityName(name string) bool {
	return name == "reply_continue" || name == "retry"
}
func backendFor(c Config) ExecutionBackend {
	if c.hostName() == "orca_supervised" {
		return orcaSupervisedBackend{config: c}
	}
	if c.hostName() == "orca" {
		return orcaTerminalBackend{config: c}
	}
	return localExecutionBackend{config: c}
}
func cloneRun(r Run) Run {
	b, _ := json.Marshal(r)
	var copy Run
	_ = json.Unmarshal(b, &copy)
	return copy
}
func agentFor(c Config) AgentSpec { return AgentSpec{Name: c.agentName()} }
func targetFor(c Config) ExecutionTarget {
	if c.hostName() == "orca_supervised" {
		supervised := c.Orca.Supervised
		if supervised == nil {
			return ExecutionTarget{}
		}
		return ExecutionTarget{ID: "orca-supervised:" + supervised.ConnectionID, ConnectionID: supervised.ConnectionID, ExternalRuntime: supervised.EnvironmentID}
	}
	if c.hostName() == "orca" {
		return ExecutionTarget{ID: "orca:" + digestValue(struct {
			Executable string
			Args       []string
		}{c.Orca.Executable, c.Orca.Args})[:24]}
	}
	name, _ := os.Hostname()
	return ExecutionTarget{ID: "local:" + runtime.GOOS + ":" + name}
}
func (s *Service) readRunWorkspace(ctx context.Context, run *Run) (WorkspaceInspection, error) {
	if run == nil || run.WorkflowVersion != WorkerFlowVersion {
		return WorkspaceInspection{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "workspace inspection requires its fixed worker-flow-v1 identity"}
	}
	w, err := s.inspectWorkflowWorkspace(ctx, run)
	return WorkspaceInspection{Repository: w.Repository, Worktree: w.Worktree, Branch: w.Branch, Base: w.Base, Head: w.Head, Dirty: w.Dirty, CheckedAt: w.VerifiedAt}, err
}

func (s *Service) inspectRunWorkspace(ctx context.Context, run *Run) error {
	_, err := s.readRunWorkspace(ctx, run)
	return err
}

func (s *Service) inspectRunProcess(run *Run) string {
	if run == nil || run.Process == nil {
		return "execution identity unavailable"
	}
	c, configErr := s.executionConfigForRun(run)
	if configErr != nil {
		return "fixed execution profile unavailable: " + configErr.Error()
	}
	records := executionRecords{root: c.RunDir}
	ref, err := records.loadBinding(run.RunID, run.AttemptID)
	if err == nil {
		if ref.External != *run.Process || ref.Backend == "" || ref.Protocol == "" || ref.Target.ID == "" {
			return "execution binding conflicts with runtime identity"
		}
		if ref.Protocol == ProtocolOrcaSupervised && !sameSupervisedBaseTarget(ref.Target, targetFor(c)) || ref.Protocol != ProtocolOrcaSupervised && ref.Target != targetFor(c) {
			return "execution target changed; refusing to inspect through a replacement backend connection"
		}
		backend := backendFor(c)
		kind, protocol := backend.Identity()
		if kind != ref.Backend || protocol != ref.Protocol {
			return "execution backend or protocol differs from the fixed binding"
		}
		obs, observeErr := backend.Observe(context.Background(), ref)
		if observeErr != nil {
			return "execution identity unverifiable: " + observeErr.Error()
		}
		if obs.Liveness == "live" {
			return "matching process still exists"
		}
		if obs.Liveness == "exited" {
			if ref.Backend == BackendLocal {
				return "process absent"
			}
			return "orca terminal exited"
		}
		return "execution identity unverifiable: " + obs.Diagnostic
	}
	if !os.IsNotExist(err) {
		return "execution binding invalid: " + err.Error()
	}
	return "execution binding unavailable; refusing to inspect through current configuration"
}
