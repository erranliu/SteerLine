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

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
)

type nativeCapabilityEvidence struct {
	Version               int                            `json:"version"`
	CapturedAt            time.Time                      `json:"captured_at"`
	Platform              string                         `json:"platform"`
	Backend               executionapi.BackendKind       `json:"backend"`
	Protocol              executionapi.ExecutionProtocol `json:"protocol"`
	TargetID              string                         `json:"target_id"`
	Agent                 string                         `json:"agent"`
	CodexVersion          string                         `json:"codex_version"`
	OrcaVersion           string                         `json:"orca_version"`
	OutputSHA256          string                         `json:"output_sha256"`
	OutputBytes           int                            `json:"output_bytes"`
	OutputSource          string                         `json:"output_source"`
	StopStatus            executionapi.OperationStatus   `json:"stop_status"`
	ProcessAbsent         bool                           `json:"process_absent"`
	ReconnectMatched      bool                           `json:"reconnect_matched"`
	WorktreeRetained      bool                           `json:"worktree_retained"`
	WorktreeCreated       bool                           `json:"worktree_created"`
	AgentTerminalBound    bool                           `json:"agent_terminal_bound"`
	InitialOutputObserved bool                           `json:"initial_output_observed"`
	GitClean              bool                           `json:"git_clean"`
	WorkspaceInspected    bool                           `json:"workspace_inspected"`
	BaseAncestor          bool                           `json:"base_ancestor"`
	StopConfirmed         bool                           `json:"stop_confirmed"`
}

func evidenceDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func markVerified(report *executionapi.CapabilityReport, name, evidence string) {
	for i := range report.Capabilities {
		if report.Capabilities[i].Name == name {
			report.Capabilities[i].Status = executionapi.CapabilityVerified
			report.Capabilities[i].Evidence = evidence
			return
		}
	}
}

// CertifyExecutionBackend imports evidence captured by the native acceptance
// harness for this backend, agent, target, platform and CLI version family.
// Repository and run-dir locks exclude the live Service while the report is saved.
func CertifyExecutionBackend(ctx context.Context, c Config, paths []string) (executionapi.CapabilityReport, error) {
	if err := requireNativeTaskAuthority(c); err != nil {
		return executionapi.CapabilityReport{}, err
	}
	if len(paths) == 0 {
		return executionapi.CapabilityReport{}, errors.New("at least one native evidence file is required")
	}
	if c.identity == "" {
		if err := c.ResolveIdentity(ctx); err != nil {
			return executionapi.CapabilityReport{}, err
		}
	}
	backend := backendFor(c)
	probe, err := backend.Probe(ctx, targetFor(c), agentFor(c))
	if err != nil {
		return executionapi.CapabilityReport{}, err
	}
	if runtime.GOOS != "windows" {
		return executionapi.CapabilityReport{}, errors.New("PR 4A native capability certification currently requires Windows evidence")
	}
	proofs := map[string]string{}
	copies := map[string][]byte{}
	for _, path := range paths {
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return executionapi.CapabilityReport{}, readErr
		}
		if len(b) == 0 || len(b) > 64<<10 {
			return executionapi.CapabilityReport{}, fmt.Errorf("native evidence size is invalid: %s", path)
		}
		var evidence nativeCapabilityEvidence
		if readErr = json.Unmarshal(b, &evidence); readErr != nil {
			return executionapi.CapabilityReport{}, fmt.Errorf("invalid native evidence %s: %w", path, readErr)
		}
		version := evidence.CodexVersion
		if probe.Backend == executionapi.BackendOrca {
			version = evidence.OrcaVersion
		}
		versionMatches := version == probe.PlatformVersion
		if probe.Backend == executionapi.BackendOrca && !versionMatches {
			versionMatches = remote.SameVersionFamily(version, probe.PlatformVersion)
		}
		if evidence.Version != 1 || evidence.CapturedAt.IsZero() || evidence.Platform != runtime.GOOS+"/"+runtime.GOARCH || evidence.Backend != probe.Backend || evidence.Protocol != probe.Protocol || evidence.TargetID != probe.Target.ID || evidence.Agent != probe.Agent.Name || !versionMatches {
			return executionapi.CapabilityReport{}, fmt.Errorf("native evidence identity, target, platform or version differs from current backend: %s", path)
		}
		if probe.Backend == executionapi.BackendOrca && evidence.CodexVersion != probe.AgentVersion {
			return executionapi.CapabilityReport{}, fmt.Errorf("native evidence does not match Orca's native/override agent launch mode and version: %s", path)
		}
		sum := sha256.Sum256(b)
		stablePath := filepath.Join(c.RunDir, "execution", "v1", "evidence", hex.EncodeToString(sum[:])+".json")
		copies[stablePath] = b
		ref := stablePath + "#sha256=" + hex.EncodeToString(sum[:])
		switch {
		case probe.Backend == executionapi.BackendLocal && evidence.ProcessAbsent && evidence.StopStatus == executionapi.OperationConfirmed && evidence.OutputBytes > 0 && evidenceDigest(evidence.OutputSHA256):
			for _, name := range []string{"start", "observe", "stop", "workspace_inspection", "output_reading"} {
				proofs[name] = ref
			}
		case probe.Backend == executionapi.BackendOrca && evidence.WorktreeCreated && evidence.AgentTerminalBound && evidence.InitialOutputObserved && evidence.StopStatus == executionapi.OperationConfirmed && evidence.GitClean:
			proofs["start"] = ref
		case probe.Backend == executionapi.BackendOrca && evidence.ReconnectMatched && evidence.StopStatus == executionapi.OperationConfirmed && evidence.WorktreeRetained && evidence.OutputBytes > 0 && evidenceDigest(evidence.OutputSHA256) && evidence.OutputSource != "":
			for _, name := range []string{"observe", "stop", "output_reading"} {
				proofs[name] = ref
			}
		case probe.Backend == executionapi.BackendOrca && evidence.WorkspaceInspected && evidence.BaseAncestor && evidence.GitClean:
			proofs["workspace_inspection"] = ref
		default:
			return executionapi.CapabilityReport{}, fmt.Errorf("native evidence does not prove a supported capability: %s", path)
		}
	}
	for _, name := range []string{"start", "observe", "stop", "workspace_inspection", "output_reading"} {
		if proofs[name] == "" {
			return executionapi.CapabilityReport{}, fmt.Errorf("native evidence does not prove required %s capability", name)
		}
	}
	for name, ref := range proofs {
		markVerified(&probe, name, ref)
	}
	probe.CheckedAt = now()
	if err = os.MkdirAll(c.RunDir, 0700); err != nil {
		return executionapi.CapabilityReport{}, err
	}
	repoLock, err := LockRepository(c.identity)
	if err != nil {
		return executionapi.CapabilityReport{}, err
	}
	defer repoLock.Close()
	runLock, err := LockRepository(c.RunDir)
	if err != nil {
		return executionapi.CapabilityReport{}, err
	}
	defer runLock.Close()
	records := executionRecords{root: c.RunDir}
	for path, data := range copies {
		if existing, readErr := os.ReadFile(path); readErr == nil {
			if string(existing) != string(data) {
				return executionapi.CapabilityReport{}, fmt.Errorf("content-addressed native evidence conflicts with existing file: %s", path)
			}
		} else if !os.IsNotExist(readErr) {
			return executionapi.CapabilityReport{}, readErr
		} else {
			if mkdirErr := os.MkdirAll(filepath.Dir(path), 0700); mkdirErr != nil {
				return executionapi.CapabilityReport{}, mkdirErr
			}
			if writeErr := AtomicWrite(path, data); writeErr != nil {
				return executionapi.CapabilityReport{}, writeErr
			}
		}
	}
	if _, loadErr := records.loadCapabilityReport(probe); loadErr != nil && !os.IsNotExist(loadErr) {
		return executionapi.CapabilityReport{}, loadErr
	}
	if err = records.saveCapabilityReport(probe); err != nil {
		return executionapi.CapabilityReport{}, err
	}
	return probe, nil
}
