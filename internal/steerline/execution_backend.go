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

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/workspace"
)

func executionStopperFor(c Config) (executionapi.ExecutionStopper, error) {
	switch c.hostName() {
	case "local":
		return localExecutionBackend{config: c}, nil
	case "orca":
		return orcaTerminalBackend{config: c}, nil
	}
	return nil, unsupported("StopExecution")
}

func unsupported(name string) error {
	return &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: name + " is not supported by this backend"}
}
func digestValue(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type executionRecords struct{ root string }

func capabilityReportKey(report executionapi.CapabilityReport) string {
	return digestValue(struct {
		Backend              executionapi.BackendKind
		Protocol             executionapi.ExecutionProtocol
		Target               executionapi.ExecutionTarget
		Agent                executionapi.AgentSpec
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

func (r executionRecords) capabilityPath(report executionapi.CapabilityReport) string {
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

func (r executionRecords) saveCapabilityReport(report executionapi.CapabilityReport) error {
	if report.Version != 1 || report.Backend == "" || report.Protocol == "" || report.Target.ID == "" || report.Agent.Name == "" || report.PlatformVersion == "" || report.CheckedAt.IsZero() || report.Protocol == executionapi.ProtocolOrcaSupervised && report.CenterVersion == "" {
		return errors.New("incomplete capability report")
	}
	seen := map[string]bool{}
	for _, cap := range report.Capabilities {
		if cap.Name == "" || retiredExecutionCapabilityName(cap.Name) || seen[cap.Name] || cap.Status != executionapi.CapabilityVerified && cap.Status != executionapi.CapabilityUnsupported && cap.Status != executionapi.CapabilityNotTested || cap.Status == executionapi.CapabilityVerified && cap.Evidence == "" {
			return errors.New("capability report has invalid or unevidenced status")
		}
		if cap.Status == executionapi.CapabilityVerified {
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

func (r executionRecords) loadCapabilityReport(key executionapi.CapabilityReport) (executionapi.CapabilityReport, error) {
	b, err := os.ReadFile(r.capabilityPath(key))
	if err != nil {
		return executionapi.CapabilityReport{}, err
	}
	var report executionapi.CapabilityReport
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
		if cap.Name == "" || retiredExecutionCapabilityName(cap.Name) || seen[cap.Name] || cap.Status != executionapi.CapabilityVerified && cap.Status != executionapi.CapabilityUnsupported && cap.Status != executionapi.CapabilityNotTested || cap.Status == executionapi.CapabilityVerified && cap.Evidence == "" {
			return report, errors.New("capability report contains invalid or unevidenced status")
		}
		if cap.Status == executionapi.CapabilityVerified {
			if err := verifyCapabilityEvidence(cap.Evidence); err != nil {
				return report, err
			}
		}
		seen[cap.Name] = true
	}
	return report, nil
}

func capabilityStatus(report executionapi.CapabilityReport, name string) executionapi.CapabilityStatus {
	for _, cap := range report.Capabilities {
		if cap.Name == name {
			return cap.Status
		}
	}
	return executionapi.CapabilityNotTested
}

func (s *Service) requireBackendCapabilities(ctx context.Context, backend executionapi.ExecutionProbe, names ...string) (executionapi.CapabilityReport, error) {
	return s.requireBackendCapabilitiesFor(ctx, backend, targetFor(s.Config), agentFor(s.Config), names...)
}

func (s *Service) requireBackendCapabilitiesFor(ctx context.Context, backend executionapi.ExecutionProbe, target executionapi.ExecutionTarget, agent executionapi.AgentSpec, names ...string) (executionapi.CapabilityReport, error) {
	probe, err := backend.Probe(ctx, target, agent)
	if err != nil {
		return executionapi.CapabilityReport{}, err
	}
	records := executionRecords{root: s.Config.RunDir}
	report, err := records.loadCapabilityReport(probe)
	if os.IsNotExist(err) {
		if saveErr := records.saveCapabilityReport(probe); saveErr != nil {
			return executionapi.CapabilityReport{}, saveErr
		}
		report = probe
	} else if err != nil {
		return executionapi.CapabilityReport{}, err
	}
	for _, name := range names {
		if capabilityStatus(report, name) != executionapi.CapabilityVerified {
			return report, &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: fmt.Sprintf("%s capability is %s for %s/%s target %s version %s", name, capabilityStatus(report, name), report.Backend, report.Protocol, report.Target.ID, report.PlatformVersion)}
		}
	}
	return report, nil
}
func validateExecutionRef(ref executionapi.ExecutionRef) error {
	if ref.Version != executionapi.ExecutionRecordVersion || !stableExecutionID(ref.RunID) || !stableExecutionID(ref.AttemptID) || ref.Backend == "" || ref.Protocol == "" {
		return errors.New("incomplete execution binding")
	}
	if ref.Target.ID == "" || ref.Agent.Name == "" || !commitHash.MatchString(ref.BaseCommit) {
		return errors.New("execution binding lacks a verified target, agent or workspace")
	}
	switch {
	case ref.Backend == executionapi.BackendLocal && (ref.Protocol == executionapi.ProtocolLocalProcess || ref.Protocol == executionapi.ProtocolLocalCodexAppServer):
		if ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || ref.External.Kind != "os" || ref.External.PID <= 0 || ref.External.Created == "" || ref.External.Executable == "" {
			return errors.New("local execution binding lacks a verified process identity")
		}
	case ref.Backend == executionapi.BackendOrca && ref.Protocol == executionapi.ProtocolOrcaTerminalV2:
		if ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || ref.External.Kind != "orca" || ref.External.Terminal == "" || ref.External.RuntimeID == "" || ref.External.Incarnation == "" {
			return errors.New("Orca execution binding lacks runtime/terminal/incarnation identity")
		}
	case ref.Backend == executionapi.BackendOrca && ref.Protocol == executionapi.ProtocolOrcaSupervised:
		if ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || ref.External.Kind != "orca_supervised" || ref.External.RuntimeID == "" || ref.External.OrchestrationRun == "" || ref.External.Task == "" || ref.External.Dispatch == "" || ref.External.ExecutionHost == "" || ref.External.WorktreeID == "" {
			return errors.New("supervised Orca execution binding lacks exact runtime/Run/Task/Dispatch/host/worktree identity")
		}
	default:
		return errors.New("unsupported backend/protocol binding")
	}
	return nil
}

func (r executionRecords) loadBinding(run, attempt string) (executionapi.ExecutionRef, error) {
	if !stableExecutionID(run) || !stableExecutionID(attempt) {
		return executionapi.ExecutionRef{}, errors.New("invalid execution binding path identity")
	}
	b, err := r.readRecord("execution_bindings", run, attempt, "binding")
	if err != nil {
		return executionapi.ExecutionRef{}, err
	}
	var ref executionapi.ExecutionRef
	if err = decode(b, &ref); err != nil {
		return ref, fmt.Errorf("invalid execution binding; preserve it: %w", err)
	}
	if ref.Version != executionapi.ExecutionRecordVersion || ref.RunID != run || ref.AttemptID != attempt {
		return ref, errors.New("execution binding identity/version mismatch")
	}
	if err := validateExecutionRef(ref); err != nil {
		return ref, fmt.Errorf("invalid execution binding; preserve it: %w", err)
	}
	return ref, nil
}
func validateExecutionOperation(op executionapi.Operation) error {
	if op.Version != executionapi.ExecutionRecordVersion || !stableOperationID(op.ID) || !stableExecutionID(op.RunID) || !stableExecutionID(op.AttemptID) || len(op.PayloadDigest) != 64 || op.Action == "" || op.Backend == "" || op.Protocol == "" || op.Target.ID == "" {
		return errors.New("incomplete operation record")
	}
	if _, err := hex.DecodeString(op.PayloadDigest); err != nil {
		return errors.New("invalid operation payload digest")
	}
	if op.Action != "start" && op.Action != "stop" && op.Action != "reply" {
		return errors.New("unsupported execution operation action")
	}
	switch op.Status {
	case executionapi.OperationPending, executionapi.OperationInFlight, executionapi.OperationAccepted, executionapi.OperationConfirmed, executionapi.OperationRejected, executionapi.OperationUnknown:
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

func (r executionRecords) loadOperation(run, attempt, id string) (executionapi.Operation, error) {
	if !stableExecutionID(run) || !stableExecutionID(attempt) || !stableOperationID(id) {
		return executionapi.Operation{}, errors.New("invalid operation path identity")
	}
	b, err := r.readRecord("execution_operations", run, attempt, id)
	if err != nil {
		return executionapi.Operation{}, err
	}
	var op executionapi.Operation
	if err = decode(b, &op); err != nil {
		return op, fmt.Errorf("invalid operation record; preserve it: %w", err)
	}
	if op.Version != executionapi.ExecutionRecordVersion || op.RunID != run || op.AttemptID != attempt || op.ID != id {
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

func (b localExecutionBackend) Identity() (executionapi.BackendKind, executionapi.ExecutionProtocol) {
	return executionapi.BackendLocal, executionapi.ProtocolLocalProcess
}
func (b localExecutionBackend) Probe(ctx context.Context, target executionapi.ExecutionTarget, agent executionapi.AgentSpec) (executionapi.CapabilityReport, error) {
	if agent.Name != "codex" {
		return executionapi.CapabilityReport{}, unsupported("local_process agent " + agent.Name)
	}
	out, err := (Runner{Dir: b.config.Repository, Timeout: positiveDuration(b.config.CommandTimeout, 30*time.Second)}).Output(ctx, b.config.Codex, "--version")
	if err != nil {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendUnreachable, Diagnostic: "local Codex version probe failed", Cause: err}
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendProtocolInvalid, Diagnostic: "local Codex version probe returned no version"}
	}
	return capabilityReport(executionapi.BackendLocal, executionapi.ProtocolLocalProcess, target, agent, version), nil
}
func (b localExecutionBackend) Observe(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.Observation, error) {
	if ref.Backend != executionapi.BackendLocal || ref.Protocol != executionapi.ProtocolLocalProcess && ref.Protocol != executionapi.ProtocolLocalCodexAppServer || ref.Target != targetFor(b.config) {
		return executionapi.Observation{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "fixed local process target changed"}
	}
	if err := ctx.Err(); err != nil {
		return executionapi.Observation{}, err
	}
	label := inspectProcess(&ref.External)
	liveness := "unverifiable"
	if label == "matching process still exists" {
		liveness = "live"
	} else if label == "process absent" {
		liveness = "exited"
	} else if strings.Contains(label, "different process") {
		return executionapi.Observation{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: label}
	}
	return executionapi.Observation{Liveness: liveness, Connection: "connected", ObservedAt: now(), Source: "os-process-probe", Diagnostic: label}, nil
}
func (b localExecutionBackend) InspectWorkspace(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.WorkspaceInspection, error) {
	if ref.Backend != executionapi.BackendLocal || ref.Protocol != executionapi.ProtocolLocalProcess && ref.Protocol != executionapi.ProtocolLocalCodexAppServer || ref.Target != targetFor(b.config) {
		return executionapi.WorkspaceInspection{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "local workspace target differs from the fixed execution"}
	}
	return inspectBoundWorkspace(ctx, b.config, ref)
}

func inspectBoundWorkspace(ctx context.Context, c Config, ref executionapi.ExecutionRef) (executionapi.WorkspaceInspection, error) {
	if ref.Incomplete || ref.WorkspacePath == "" || ref.WorkspaceBranch == "" || !commitHash.MatchString(ref.BaseCommit) {
		return executionapi.WorkspaceInspection{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "workspace ref is incomplete"}
	}
	if c.identity == "" {
		if err := c.ResolveIdentity(ctx); err != nil {
			return executionapi.WorkspaceInspection{}, err
		}
	}
	intent := &executionapi.WorkspaceIntent{Path: ref.WorkspacePath, Branch: ref.WorkspaceBranch, BaseCommit: ref.BaseCommit}
	repository, err := workspaceRepository(c)
	if err != nil {
		return executionapi.WorkspaceInspection{}, err
	}
	inspection, err := workspace.Inspect(ctx, workspaceGit(c), repository, intent)
	if err != nil {
		return executionapi.WorkspaceInspection{}, err
	}
	return inspection, nil
}

type localOutputCursor struct {
	Version   int    `json:"v"`
	RunID     string `json:"r"`
	AttemptID string `json:"a"`
	Offset    int64  `json:"o"`
	Prefix    string `json:"p,omitempty"`
}

func (b localExecutionBackend) ReadOutput(ctx context.Context, ref executionapi.ExecutionRef, cursor string, limit int) (executionapi.OutputBatch, error) {
	if ref.Backend != executionapi.BackendLocal || ref.Protocol != executionapi.ProtocolLocalProcess || ref.Target != targetFor(b.config) || !stableExecutionID(ref.RunID) || !stableExecutionID(ref.AttemptID) {
		return executionapi.OutputBatch{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "output ref does not match the fixed local execution"}
	}
	if limit <= 0 || limit > 1<<20 {
		return executionapi.OutputBatch{}, &executionapi.BackendError{Code: executionapi.BackendProtocolInvalid, Diagnostic: "output limit must be between 1 and 1048576 bytes"}
	}
	path := filepath.Join(b.config.RunDir, "runs", ref.RunID)
	if ref.AttemptID != ref.RunID {
		path = filepath.Join(path, "attempts", ref.AttemptID)
	}
	path = filepath.Join(path, "events.jsonl")
	select {
	case <-ctx.Done():
		return executionapi.OutputBatch{}, ctx.Err()
	default:
	}
	offset := int64(0)
	oldPrefix := ""
	if cursor != "" {
		token, decodeErr := base64.RawURLEncoding.DecodeString(cursor)
		if decodeErr != nil {
			return executionapi.OutputBatch{}, &executionapi.BackendError{Code: executionapi.BackendProtocolInvalid, Diagnostic: "invalid output cursor", Cause: decodeErr}
		}
		var c localOutputCursor
		if decodeErr = json.Unmarshal(token, &c); decodeErr != nil || c.Version != 1 || c.RunID != ref.RunID || c.AttemptID != ref.AttemptID || c.Offset < 0 {
			return executionapi.OutputBatch{}, &executionapi.BackendError{Code: executionapi.BackendProtocolInvalid, Diagnostic: "output cursor is malformed or belongs to another execution"}
		}
		offset, oldPrefix = c.Offset, c.Prefix
	}
	f, err := os.Open(path)
	if err != nil {
		return executionapi.OutputBatch{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return executionapi.OutputBatch{}, err
	}
	batch := executionapi.OutputBatch{Source: "local-jsonl", RetainedFrom: "0", ObservedAt: now()}
	if offset > info.Size() {
		batch.Gap = true
		offset = info.Size()
	}
	if oldPrefix != "" {
		prefix, prefixErr := readOutputPrefix(f, offset)
		if prefixErr != nil {
			return executionapi.OutputBatch{}, prefixErr
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
			return executionapi.OutputBatch{}, readErr
		}
		chunk = chunk[:n]
	}
	batch.Data = bytes.Clone(chunk)
	end = offset + int64(len(chunk))
	prefix, prefixErr := readOutputPrefix(f, end)
	if prefixErr != nil {
		return executionapi.OutputBatch{}, prefixErr
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
func (b localExecutionBackend) StopExecution(ctx context.Context, ref executionapi.ExecutionRef, _ executionapi.Operation, handle executionapi.StopHandle) (executionapi.StopReceipt, error) {
	if ref.Backend != executionapi.BackendLocal || (ref.Protocol != executionapi.ProtocolLocalProcess && ref.Protocol != executionapi.ProtocolLocalCodexAppServer) || ref.Target != targetFor(b.config) {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "fixed local process target changed"}
	}
	if handle == nil || handle.Identity() != ref.External {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "local stop handle does not match the fixed process identity"}
	}
	if err := ctx.Err(); err != nil {
		return executionapi.StopReceipt{}, err
	}
	obs, err := b.Observe(ctx, ref)
	if err != nil {
		return executionapi.StopReceipt{}, err
	}
	if obs.Liveness == "exited" {
		return executionapi.StopReceipt{Disposition: "confirmed", Diagnostic: obs.Diagnostic}, nil
	}
	if obs.Liveness != "live" {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "local process liveness cannot be verified"}
	}
	handle.Kill()
	return executionapi.StopReceipt{Disposition: "accepted"}, nil
}

type orcaTerminalBackend struct{ config Config }

func (b orcaTerminalBackend) Identity() (executionapi.BackendKind, executionapi.ExecutionProtocol) {
	return executionapi.BackendOrca, executionapi.ProtocolOrcaTerminalV2
}
func (b orcaTerminalBackend) Probe(ctx context.Context, target executionapi.ExecutionTarget, agent executionapi.AgentSpec) (executionapi.CapabilityReport, error) {
	if agent.Name == "" {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendRejected, Diagnostic: "Orca agent is required"}
	}
	out, err := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "--version")
	if err != nil {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendUnreachable, Diagnostic: "Orca version probe failed", Cause: err}
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendProtocolInvalid, Diagnostic: "Orca version probe returned no version"}
	}
	report := capabilityReport(executionapi.BackendOrca, executionapi.ProtocolOrcaTerminalV2, target, agent, version)
	if b.config.needsCodexExecutable() {
		out, err := (Runner{Dir: b.config.Repository, Timeout: positiveDuration(b.config.CommandTimeout, 30*time.Second)}).Output(ctx, b.config.Codex, "--version")
		if err != nil || strings.TrimSpace(string(out)) == "" {
			return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendUnreachable, Diagnostic: "Orca Codex override version probe failed", Cause: err}
		}
		report.AgentVersion = strings.TrimSpace(string(out))
	}
	return report, nil
}
func (b orcaTerminalBackend) Observe(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.Observation, error) {
	old := ref.External
	if old.Kind != "orca" || old.Terminal == "" {
		return executionapi.Observation{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca terminal binding is incomplete"}
	}
	if targetFor(b.config) != ref.Target {
		return executionapi.Observation{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "configured Orca connection differs from the fixed execution target"}
	}
	if err := ctx.Err(); err != nil {
		return executionapi.Observation{}, err
	}
	term, e := inspectBoundOrcaTerminal(ctx, b.config, old)
	if e != nil {
		return executionapi.Observation{}, e
	}
	life := "live"
	if term.Exited() {
		life = "exited"
	} else if !term.Connected {
		life = "unverifiable"
	}
	return executionapi.Observation{Liveness: life, Connection: map[bool]string{true: "connected", false: "unreachable"}[term.Connected], ObservedAt: now(), Source: "orca-terminal-v2", Diagnostic: term.Status}, nil
}
func (b orcaTerminalBackend) InspectWorkspace(ctx context.Context, ref executionapi.ExecutionRef) (executionapi.WorkspaceInspection, error) {
	if ref.Backend != executionapi.BackendOrca || ref.Protocol != executionapi.ProtocolOrcaTerminalV2 || ref.Target != targetFor(b.config) {
		return executionapi.WorkspaceInspection{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca workspace target differs from the fixed execution"}
	}
	// terminal-v2 worktrees live on this configured host. Future supervised
	// targets must implement their own remote inspection instead of using Git here.
	return inspectBoundWorkspace(ctx, b.config, ref)
}

func (b orcaTerminalBackend) StopExecution(ctx context.Context, ref executionapi.ExecutionRef, _ executionapi.Operation, handle executionapi.StopHandle) (executionapi.StopReceipt, error) {
	obs, err := b.Observe(ctx, ref)
	if err != nil {
		return executionapi.StopReceipt{}, err
	}
	if obs.Liveness == "exited" {
		return executionapi.StopReceipt{Disposition: "confirmed", Diagnostic: obs.Diagnostic}, nil
	}
	if obs.Liveness != "live" {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca terminal is not verifiably live"}
	}
	if handle != nil && handle.Identity() != ref.External {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca observer handle does not match the fixed terminal identity"}
	}
	closeReceipt, err := (orcaClient{cfg: b.config}).closeTerminalChecked(ctx, ref.External.Terminal, ref.External.RuntimeID)
	if err != nil {
		return executionapi.StopReceipt{}, err
	}
	if closeReceipt.Disposition == "confirmed" {
		return closeReceipt, nil
	}
	obs, err = b.Observe(ctx, ref)
	if err != nil {
		return executionapi.StopReceipt{}, &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "terminal close was accepted but stop could not be confirmed", Cause: err}
	}
	if obs.Liveness == "exited" {
		return executionapi.StopReceipt{Disposition: "confirmed", Diagnostic: obs.Diagnostic}, nil
	}
	return executionapi.StopReceipt{Disposition: "accepted", Diagnostic: "close accepted; terminal exit is not yet confirmed"}, nil
}

func capabilityReport(backend executionapi.BackendKind, protocol executionapi.ExecutionProtocol, target executionapi.ExecutionTarget, agent executionapi.AgentSpec, version string) executionapi.CapabilityReport {
	caps := []executionapi.Capability{{Name: "start", Status: executionapi.CapabilityNotTested}, {Name: "observe", Status: executionapi.CapabilityNotTested}, {Name: "stop", Status: executionapi.CapabilityNotTested}, {Name: "detach_observation", Status: executionapi.CapabilityNotTested}, {Name: "operation_reconciliation", Status: executionapi.CapabilityNotTested}, {Name: "workspace_inspection", Status: executionapi.CapabilityNotTested}, {Name: "output_reading", Status: executionapi.CapabilityNotTested}, {Name: "output_turns", Status: executionapi.CapabilityNotTested}, {Name: "output_final_answer", Status: executionapi.CapabilityNotTested}, {Name: "resource_settlement", Status: executionapi.CapabilityUnsupported}}
	return executionapi.CapabilityReport{Version: 1, Backend: backend, Protocol: protocol, Target: target, Agent: agent, PlatformVersion: version, CheckedAt: now(), Capabilities: caps}
}

func retiredExecutionCapabilityName(name string) bool {
	return name == "reply_continue" || name == "retry"
}
func backendFor(c Config) executionapi.ExecutionBackend {
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
func agentFor(c Config) executionapi.AgentSpec { return executionapi.AgentSpec{Name: c.agentName()} }
func targetFor(c Config) executionapi.ExecutionTarget {
	if c.hostName() == "orca_supervised" {
		supervised := c.Orca.Supervised
		if supervised == nil {
			return executionapi.ExecutionTarget{}
		}
		return executionapi.ExecutionTarget{ID: "orca-supervised:" + supervised.ConnectionID, ConnectionID: supervised.ConnectionID, ExternalRuntime: supervised.EnvironmentID}
	}
	if c.hostName() == "orca" {
		return executionapi.ExecutionTarget{ID: "orca:" + digestValue(struct {
			Executable string
			Args       []string
		}{c.Orca.Executable, c.Orca.Args})[:24]}
	}
	name, _ := os.Hostname()
	return executionapi.ExecutionTarget{ID: "local:" + runtime.GOOS + ":" + name}
}
func (s *Service) readRunWorkspace(ctx context.Context, run *Run) (executionapi.WorkspaceInspection, error) {
	if run == nil || run.WorkflowVersion != WorkerFlowVersion {
		return executionapi.WorkspaceInspection{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "workspace inspection requires its fixed worker-flow-v1 identity"}
	}
	w, err := s.inspectWorkflowWorkspace(ctx, run)
	return executionapi.WorkspaceInspection{Repository: w.Repository, Worktree: w.Worktree, Branch: w.Branch, Base: w.Base, Head: w.Head, Dirty: w.Dirty, CheckedAt: w.VerifiedAt}, err
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
		if ref.Protocol == executionapi.ProtocolOrcaSupervised {
			execution := s.executionRecord(run)
			if execution == nil || execution.Process == nil || *execution.Process != *run.Process ||
				!sameSupervisedBaseTarget(ref.Target, execution.Target) || ref.External.Kind != "orca_supervised" ||
				ref.External.RuntimeID != run.Process.RuntimeID || ref.External.CoordinatorRuntimeID != run.Process.CoordinatorRuntimeID ||
				ref.External.Terminal != run.Process.Terminal || ref.External.Incarnation != run.Process.Incarnation ||
				ref.External.ExecutionHost != run.Process.ExecutionHost || ref.External.WorktreeID != run.Process.WorktreeID ||
				ref.WorkspacePath != execution.Workspace.Worktree || ref.WorkspaceBranch != execution.Workspace.Branch || ref.BaseCommit != execution.Workspace.Base {
				return "remote session conflicts with its original execution binding"
			}
			// The session binding stays immutable across turns. Inspect the current
			// Dispatch stored by the center's committed worker transition.
			ref = remoteExecutionRef(run, execution)
		}
		if ref.External != *run.Process || ref.Backend == "" || ref.Protocol == "" || ref.Target.ID == "" {
			return "execution binding conflicts with runtime identity"
		}
		if ref.Protocol == executionapi.ProtocolOrcaSupervised && !sameSupervisedBaseTarget(ref.Target, targetFor(c)) || ref.Protocol != executionapi.ProtocolOrcaSupervised && ref.Target != targetFor(c) {
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
			if ref.Backend == executionapi.BackendLocal {
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
