package steerlineapp

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

	"github.com/erranliu/SteerLine/internal/steerline"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
)

type capabilityEvidence struct {
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

func CertifyExecutionBackend(ctx context.Context, config steerline.Config, paths []string) (executionapi.CapabilityReport, error) {
	if config.TaskAuthority != "native" {
		return executionapi.CapabilityReport{}, errors.New("task_authority=native is required")
	}
	if config.WorkflowProtocol != steerline.WorkerFlowVersion {
		return executionapi.CapabilityReport{}, errors.New("only the current worker flow can be certified")
	}
	if len(paths) == 0 {
		return executionapi.CapabilityReport{}, errors.New("at least one capability evidence file is required")
	}
	if config.Identity() == "" {
		if err := config.ResolveIdentity(ctx); err != nil {
			return executionapi.CapabilityReport{}, err
		}
	}
	if config.ExecutionDirectory == nil {
		return executionapi.CapabilityReport{}, errors.New("execution directory is required")
	}
	entry, err := config.ExecutionDirectory.Resolve(config)
	if err != nil {
		return executionapi.CapabilityReport{}, err
	}
	probe, err := entry.ProbeRequirements(ctx, config)
	if err != nil {
		return executionapi.CapabilityReport{}, err
	}
	if runtime.GOOS != "windows" {
		return executionapi.CapabilityReport{}, errors.New("native capability certification requires Windows evidence")
	}
	proofs := map[string]string{}
	copies := map[string][]byte{}
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return executionapi.CapabilityReport{}, readErr
		}
		if len(data) == 0 || len(data) > 64<<10 {
			return executionapi.CapabilityReport{}, fmt.Errorf("capability evidence size is invalid: %s", path)
		}
		var evidence capabilityEvidence
		if readErr = json.Unmarshal(data, &evidence); readErr != nil {
			return executionapi.CapabilityReport{}, fmt.Errorf("invalid capability evidence %s: %w", path, readErr)
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
			return executionapi.CapabilityReport{}, fmt.Errorf("capability evidence identity, target, platform or version differs from the selected implementation: %s", path)
		}
		if probe.Backend == executionapi.BackendOrca && evidence.CodexVersion != probe.AgentVersion {
			return executionapi.CapabilityReport{}, fmt.Errorf("capability evidence does not match the selected agent launch mode and version: %s", path)
		}
		sum := sha256.Sum256(data)
		stablePath := filepath.Join(config.RunDir, "execution", "v1", "evidence", hex.EncodeToString(sum[:])+".json")
		copies[stablePath] = data
		reference := stablePath + "#sha256=" + hex.EncodeToString(sum[:])
		switch {
		case probe.Backend == executionapi.BackendLocal && evidence.ProcessAbsent && evidence.StopStatus == executionapi.OperationConfirmed && evidence.OutputBytes > 0 && evidenceDigest(evidence.OutputSHA256):
			for _, name := range []string{"start", "observe", "stop", "workspace_inspection", "output_reading"} {
				proofs[name] = reference
			}
		case probe.Backend == executionapi.BackendOrca && evidence.WorktreeCreated && evidence.AgentTerminalBound && evidence.InitialOutputObserved && evidence.StopStatus == executionapi.OperationConfirmed && evidence.GitClean:
			proofs["start"] = reference
		case probe.Backend == executionapi.BackendOrca && evidence.ReconnectMatched && evidence.StopStatus == executionapi.OperationConfirmed && evidence.WorktreeRetained && evidence.OutputBytes > 0 && evidenceDigest(evidence.OutputSHA256) && evidence.OutputSource != "":
			for _, name := range []string{"observe", "stop", "output_reading"} {
				proofs[name] = reference
			}
		case probe.Backend == executionapi.BackendOrca && evidence.WorkspaceInspected && evidence.BaseAncestor && evidence.GitClean:
			proofs["workspace_inspection"] = reference
		default:
			return executionapi.CapabilityReport{}, fmt.Errorf("capability evidence does not prove a supported capability: %s", path)
		}
	}
	for _, name := range []string{"start", "observe", "stop", "workspace_inspection", "output_reading"} {
		if proofs[name] == "" {
			return executionapi.CapabilityReport{}, fmt.Errorf("capability evidence does not prove required %s capability", name)
		}
	}
	for name, reference := range proofs {
		markCapabilityVerified(&probe, name, reference)
	}
	probe.CheckedAt = time.Now().UTC()
	if err := os.MkdirAll(config.RunDir, 0700); err != nil {
		return executionapi.CapabilityReport{}, err
	}
	repoLock, err := steerline.LockRepository(config.Identity())
	if err != nil {
		return executionapi.CapabilityReport{}, err
	}
	defer repoLock.Close()
	runLock, err := steerline.LockRepository(config.RunDir)
	if err != nil {
		return executionapi.CapabilityReport{}, err
	}
	defer runLock.Close()
	for path, data := range copies {
		if current, readErr := os.ReadFile(path); readErr == nil {
			if string(current) != string(data) {
				return executionapi.CapabilityReport{}, fmt.Errorf("content-addressed evidence conflicts with existing file: %s", path)
			}
		} else if !os.IsNotExist(readErr) {
			return executionapi.CapabilityReport{}, readErr
		} else {
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return executionapi.CapabilityReport{}, err
			}
			if err := steerline.AtomicWrite(path, data); err != nil {
				return executionapi.CapabilityReport{}, err
			}
		}
	}
	if err := steerline.SaveExecutionCapabilityReport(config, probe); err != nil {
		return executionapi.CapabilityReport{}, err
	}
	return probe, nil
}

func evidenceDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func markCapabilityVerified(report *executionapi.CapabilityReport, name, reference string) {
	for index := range report.Capabilities {
		if report.Capabilities[index].Name == name {
			report.Capabilities[index].Status = executionapi.CapabilityVerified
			report.Capabilities[index].Evidence = reference
			return
		}
	}
}
