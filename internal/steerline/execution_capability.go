package steerline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type nativeCapabilityEvidence struct {
	Version               int               `json:"version"`
	CapturedAt            time.Time         `json:"captured_at"`
	Platform              string            `json:"platform"`
	Backend               BackendKind       `json:"backend"`
	Protocol              ExecutionProtocol `json:"protocol"`
	TargetID              string            `json:"target_id"`
	Agent                 string            `json:"agent"`
	CodexVersion          string            `json:"codex_version"`
	OrcaVersion           string            `json:"orca_version"`
	OutputSHA256          string            `json:"output_sha256"`
	OutputBytes           int               `json:"output_bytes"`
	OutputSource          string            `json:"output_source"`
	StopStatus            OperationStatus   `json:"stop_status"`
	ProcessAbsent         bool              `json:"process_absent"`
	ReconnectMatched      bool              `json:"reconnect_matched"`
	WorktreeRetained      bool              `json:"worktree_retained"`
	WorktreeCreated       bool              `json:"worktree_created"`
	AgentTerminalBound    bool              `json:"agent_terminal_bound"`
	InitialOutputObserved bool              `json:"initial_output_observed"`
	GitClean              bool              `json:"git_clean"`
	WorkspaceInspected    bool              `json:"workspace_inspected"`
	BaseAncestor          bool              `json:"base_ancestor"`
	FirstInputAccepted    bool              `json:"first_input_accepted"`
	FirstTurnStarted      bool              `json:"first_turn_started"`
	SecondInputAccepted   bool              `json:"second_input_accepted"`
	SecondTurnStarted     bool              `json:"second_turn_started"`
	SameTerminal          bool              `json:"same_terminal"`
	SameIncarnation       bool              `json:"same_incarnation"`
	SameRuntime           bool              `json:"same_runtime"`
	FirstOutputObserved   bool              `json:"first_output_observed"`
	SecondOutputObserved  bool              `json:"second_output_observed"`
	StopConfirmed         bool              `json:"stop_confirmed"`
	RetrySameRun          bool              `json:"retry_same_run"`
	RetryNewAttempt       bool              `json:"retry_new_attempt"`
	ClaimCount            int               `json:"claim_count"`
	WorkspaceReused       bool              `json:"workspace_reused"`
	PreviousRunPreserved  bool              `json:"previous_run_preserved"`
}

func evidenceDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func markVerified(report *CapabilityReport, name, evidence string) {
	for i := range report.Capabilities {
		if report.Capabilities[i].Name == name {
			report.Capabilities[i].Status = CapabilityVerified
			report.Capabilities[i].Evidence = evidence
			return
		}
	}
}

// CertifyExecutionBackend imports evidence captured by the native acceptance
// harness for this exact backend, agent, target, platform and CLI version.
// Repository and run-dir locks exclude the live Service while the report is saved.
func CertifyExecutionBackend(ctx context.Context, c Config, paths []string) (CapabilityReport, error) {
	if len(paths) == 0 {
		return CapabilityReport{}, errors.New("at least one native evidence file is required")
	}
	if c.identity == "" {
		if err := c.ResolveIdentity(ctx); err != nil {
			return CapabilityReport{}, err
		}
	}
	backend := backendFor(c, BeadsContext{})
	probe, err := backend.Probe(ctx, targetFor(c), agentFor(c))
	if err != nil {
		return CapabilityReport{}, err
	}
	if runtime.GOOS != "windows" {
		return CapabilityReport{}, errors.New("PR 4A native capability certification currently requires Windows evidence")
	}
	proofs := map[string]string{}
	copies := map[string][]byte{}
	for _, path := range paths {
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return CapabilityReport{}, readErr
		}
		if len(b) == 0 || len(b) > 64<<10 {
			return CapabilityReport{}, fmt.Errorf("native evidence size is invalid: %s", path)
		}
		var evidence nativeCapabilityEvidence
		if readErr = json.Unmarshal(b, &evidence); readErr != nil {
			return CapabilityReport{}, fmt.Errorf("invalid native evidence %s: %w", path, readErr)
		}
		version := evidence.CodexVersion
		if probe.Backend == BackendOrca {
			version = evidence.OrcaVersion
		}
		if evidence.Version != 1 || evidence.CapturedAt.IsZero() || evidence.Platform != runtime.GOOS+"/"+runtime.GOARCH || evidence.Backend != probe.Backend || evidence.Protocol != probe.Protocol || evidence.TargetID != probe.Target.ID || evidence.Agent != probe.Agent.Name || version != probe.PlatformVersion {
			return CapabilityReport{}, fmt.Errorf("native evidence identity, target, platform or version differs from current backend: %s", path)
		}
		sum := sha256.Sum256(b)
		stablePath := filepath.Join(c.RunDir, "execution", "v1", "evidence", hex.EncodeToString(sum[:])+".json")
		copies[stablePath] = b
		ref := stablePath + "#sha256=" + hex.EncodeToString(sum[:])
		switch {
		case probe.Backend == BackendLocal && evidence.RetrySameRun && evidence.RetryNewAttempt && evidence.ClaimCount == 1 && evidence.WorkspaceReused && evidence.PreviousRunPreserved && evidence.StopStatus == OperationConfirmed && evidence.OutputBytes > 0 && evidenceDigest(evidence.OutputSHA256):
			proofs["retry"] = ref
		case probe.Backend == BackendLocal && evidence.ProcessAbsent && evidence.StopStatus == OperationConfirmed && evidence.OutputBytes > 0 && evidenceDigest(evidence.OutputSHA256):
			for _, name := range []string{"start", "observe", "stop", "workspace_inspection", "output_reading"} {
				proofs[name] = ref
			}
		case probe.Backend == BackendOrca && evidence.WorktreeCreated && evidence.AgentTerminalBound && evidence.InitialOutputObserved && evidence.StopStatus == OperationConfirmed && evidence.GitClean:
			proofs["start"] = ref
		case probe.Backend == BackendOrca && evidence.ReconnectMatched && evidence.StopStatus == OperationConfirmed && evidence.WorktreeRetained && evidence.OutputBytes > 0 && evidenceDigest(evidence.OutputSHA256) && evidence.OutputSource != "":
			for _, name := range []string{"observe", "stop", "output_reading"} {
				proofs[name] = ref
			}
		case probe.Backend == BackendOrca && evidence.WorkspaceInspected && evidence.BaseAncestor && evidence.GitClean:
			proofs["workspace_inspection"] = ref
		case probe.Backend == BackendOrca && evidence.FirstInputAccepted && evidence.FirstTurnStarted && evidence.SecondInputAccepted && evidence.SecondTurnStarted && evidence.SameTerminal && evidence.SameIncarnation && evidence.SameRuntime && evidence.FirstOutputObserved && evidence.SecondOutputObserved && evidence.StopConfirmed && evidence.GitClean:
			proofs["reply_continue"] = ref
		default:
			return CapabilityReport{}, fmt.Errorf("native evidence does not prove a supported capability: %s", path)
		}
	}
	for _, name := range []string{"start", "observe", "stop", "workspace_inspection", "output_reading"} {
		if proofs[name] == "" {
			return CapabilityReport{}, fmt.Errorf("native evidence does not prove required %s capability", name)
		}
	}
	for name, ref := range proofs {
		markVerified(&probe, name, ref)
	}
	probe.CheckedAt = now()
	if err = os.MkdirAll(c.RunDir, 0700); err != nil {
		return CapabilityReport{}, err
	}
	repoLock, err := LockRepository(c.identity)
	if err != nil {
		return CapabilityReport{}, err
	}
	defer repoLock.Close()
	runLock, err := LockRepository(c.RunDir)
	if err != nil {
		return CapabilityReport{}, err
	}
	defer runLock.Close()
	records := executionRecords{root: c.RunDir}
	for path, data := range copies {
		if existing, readErr := os.ReadFile(path); readErr == nil {
			if string(existing) != string(data) {
				return CapabilityReport{}, fmt.Errorf("content-addressed native evidence conflicts with existing file: %s", path)
			}
		} else if !os.IsNotExist(readErr) {
			return CapabilityReport{}, readErr
		} else {
			if mkdirErr := os.MkdirAll(filepath.Dir(path), 0700); mkdirErr != nil {
				return CapabilityReport{}, mkdirErr
			}
			if writeErr := AtomicWrite(path, data); writeErr != nil {
				return CapabilityReport{}, writeErr
			}
		}
	}
	if _, loadErr := records.loadCapabilityReport(probe); loadErr != nil && !os.IsNotExist(loadErr) {
		return CapabilityReport{}, loadErr
	}
	if err = records.saveCapabilityReport(probe); err != nil {
		return CapabilityReport{}, err
	}
	return probe, nil
}
