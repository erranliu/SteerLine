package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func executionStopperFor(c Config) (executionbackend.ExecutionStopper, error) {
	entry, err := registrationFor(c)
	if err != nil {
		return nil, err
	}
	stopper, ok := entry.Backend(c).(executionbackend.ExecutionStopper)
	if !ok {
		return nil, unsupported("StopExecution")
	}
	return stopper, nil
}

func unsupported(name string) error {
	return &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: name + " is not supported by this backend"}
}

func evidenceDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func digestValue(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type executionRecords struct {
	root      string
	reader    ExecutionRecordReader
	resources ResourcePort
}

// VerifyStoredCapabilityReport checks existing evidence without acquiring a
// writer or publishing a replacement report.
func VerifyStoredCapabilityReport(root string, report executionbackend.CapabilityReport) error {
	_, err := (executionRecords{root: root}).loadCapabilityReport(report)
	return err
}

// SaveExecutionCapabilityReport persists one already verified, generic report
// using the center's capability record format.
func SaveExecutionCapabilityReport(c Config, report executionbackend.CapabilityReport) error {
	if c.RunDir == "" {
		return errors.New("run directory is required for a capability report")
	}
	records := executionRecords{root: c.RunDir, reader: c.ExecutionRecords, resources: c.Resources}
	if _, err := records.loadCapabilityReport(report); err != nil && !os.IsNotExist(err) {
		return err
	}
	return records.saveCapabilityReport(report)
}

func CapabilityReportKey(report executionbackend.CapabilityReport) string {
	return digestValue(struct {
		Backend              executionbackend.BackendKind
		Protocol             executionbackend.ExecutionProtocol
		Target               executionbackend.ExecutionTarget
		Agent                executionbackend.AgentSpec
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

func (r executionRecords) capabilityPath(report executionbackend.CapabilityReport) string {
	return filepath.Join(r.root, "execution", "v1", "capabilities", CapabilityReportKey(report)+".json")
}

func (r executionRecords) saveCapabilityReport(report executionbackend.CapabilityReport) error {
	if report.Version != 1 || report.Backend == "" || report.Protocol == "" || report.Target.ID == "" || report.Agent.Name == "" || report.PlatformVersion == "" || report.CheckedAt.IsZero() {
		return errors.New("incomplete capability report")
	}
	seen := map[string]bool{}
	for _, cap := range report.Capabilities {
		if cap.Name == "" || retiredExecutionCapabilityName(cap.Name) || seen[cap.Name] || cap.Status != executionbackend.CapabilityVerified && cap.Status != executionbackend.CapabilityUnsupported && cap.Status != executionbackend.CapabilityNotTested || cap.Status == executionbackend.CapabilityVerified && cap.Evidence == "" {
			return errors.New("capability report has invalid or unevidenced status")
		}
		if cap.Status == executionbackend.CapabilityVerified {
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
	if r.resources == nil {
		return errors.New("file resource port is required")
	}
	return r.resources.AtomicWrite(p, append(b, '\n'))
}

func (r executionRecords) loadCapabilityReport(key executionbackend.CapabilityReport) (executionbackend.CapabilityReport, error) {
	b, err := os.ReadFile(r.capabilityPath(key))
	if err != nil {
		return executionbackend.CapabilityReport{}, err
	}
	var report executionbackend.CapabilityReport
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
		if cap.Name == "" || retiredExecutionCapabilityName(cap.Name) || seen[cap.Name] || cap.Status != executionbackend.CapabilityVerified && cap.Status != executionbackend.CapabilityUnsupported && cap.Status != executionbackend.CapabilityNotTested || cap.Status == executionbackend.CapabilityVerified && cap.Evidence == "" {
			return report, errors.New("capability report contains invalid or unevidenced status")
		}
		if cap.Status == executionbackend.CapabilityVerified {
			if err := verifyCapabilityEvidence(cap.Evidence); err != nil {
				return report, err
			}
		}
		seen[cap.Name] = true
	}
	return report, nil
}

func capabilityStatus(report executionbackend.CapabilityReport, name string) executionbackend.CapabilityStatus {
	for _, cap := range report.Capabilities {
		if cap.Name == name {
			return cap.Status
		}
	}
	return executionbackend.CapabilityNotTested
}

func (s *Service) requireBackendCapabilitiesFor(ctx context.Context, backend executionbackend.ExecutionProbe, target executionbackend.ExecutionTarget, agent executionbackend.AgentSpec, names ...string) (executionbackend.CapabilityReport, error) {
	probe, err := backend.Probe(ctx, target, agent)
	if err != nil {
		return executionbackend.CapabilityReport{}, err
	}
	records := executionRecords{root: s.Config.RunDir, reader: s.Config.ExecutionRecords, resources: s.Config.Resources}
	report, err := records.loadCapabilityReport(probe)
	if os.IsNotExist(err) {
		if saveErr := records.saveCapabilityReport(probe); saveErr != nil {
			return executionbackend.CapabilityReport{}, saveErr
		}
		report = probe
	} else if err != nil {
		return executionbackend.CapabilityReport{}, err
	}
	for _, name := range names {
		if capabilityStatus(report, name) != executionbackend.CapabilityVerified {
			return report, &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: fmt.Sprintf("%s capability is %s for %s/%s target %s version %s", name, capabilityStatus(report, name), report.Backend, report.Protocol, report.Target.ID, report.PlatformVersion)}
		}
	}
	return report, nil
}

func retiredExecutionCapabilityName(name string) bool {
	return name == "reply_continue" || name == "retry"
}

func (s *Service) readRunWorkspace(ctx context.Context, run *contracts.Run) (executionbackend.WorkspaceInspection, error) {
	if run == nil || run.WorkflowVersion != executionmodel.Version {
		return executionbackend.WorkspaceInspection{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "workspace inspection requires its fixed worker-flow-v1 identity"}
	}
	w, err := s.inspectWorkflowWorkspace(ctx, run)
	return executionbackend.WorkspaceInspection{Repository: w.Repository, Worktree: w.Worktree, Branch: w.Branch, Base: w.Base, Head: w.Head, Dirty: w.Dirty, CheckedAt: w.VerifiedAt}, err
}

func (s *Service) inspectRunProcess(run *contracts.Run) string {
	if run == nil || run.Process == nil {
		return "execution identity unavailable"
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return "fixed execution identity unavailable: " + err.Error()
	}
	if fixed.execution.Process == nil || *fixed.execution.Process != *run.Process {
		return "execution binding conflicts with runtime identity"
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return "execution binding invalid: " + err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), positiveDuration(fixed.config.CommandTimeout, 30*time.Second))
	defer cancel()
	evidence, err := lifecycle.VerifyManualRecovery(ctx, snapshot)
	if err != nil {
		return "execution identity unverifiable: " + err.Error()
	}
	if evidence.ProcessAbsent {
		return "process absent"
	}
	if evidence.ManagedProcess && evidence.SessionAvailable {
		return "matching process still exists"
	}
	return "execution identity unverifiable"
}
