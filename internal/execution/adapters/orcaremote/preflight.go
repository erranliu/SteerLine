package orca_remote_worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func Preflight(ctx context.Context, c Config) (PreflightResult, error) {
	if err := c.Validate(); err != nil {
		return PreflightResult{}, err
	}
	sc := &c.Settings
	if strings.TrimSpace(c.Agent) == "" || (c.ReasoningEffort != "" && c.Model == "") {
		return PreflightResult{}, errors.New("remote Orca agent/launch selection is incomplete")
	}
	client := Client{config: c}
	timeout := positiveDuration(c.CommandTimeout, 30*time.Second)
	centerVersionBytes, err := client.config.Call(ctx, timeout, "--version")
	if err != nil {
		return PreflightResult{}, fmt.Errorf("center Orca version probe: %w", err)
	}
	centerVersion := strings.TrimSpace(string(centerVersionBytes))
	if _, ok := orcaVersionFamily(centerVersion); !ok {
		return PreflightResult{}, fmt.Errorf("center Orca version %q has no major.minor family", centerVersion)
	}
	centerStatus, err := supervisedRuntimeStatus(ctx, client, "")
	if err != nil {
		return PreflightResult{}, fmt.Errorf("center Orca status: %w", err)
	}
	if centerStatus.State != "ready" || !centerStatus.Reachable || centerStatus.RuntimeID == "" {
		return PreflightResult{}, errors.New("center Orca runtime is not ready")
	}
	if c.local() && centerStatus.TargetKind != "local" {
		return PreflightResult{}, errors.New("local Orca supervised target was redirected or omitted its target identity; clear ambient Orca remote routing")
	}
	if centerStatus.AppVersion != "" && !SameVersionFamily(centerVersion, centerStatus.AppVersion) {
		return PreflightResult{}, errors.New("center Orca CLI and runtime versions are from different major.minor families")
	}

	remoteStatus := centerStatus
	repoSelector := sc.RepositorySelector
	if c.local() {
		repoID, findErr := orcaworker.FindRepo(ctx, client.centerPayload(centerStatus.RuntimeID), c.Repository)
		if findErr != nil {
			return PreflightResult{}, fmt.Errorf("local Orca repository lookup: %w", findErr)
		}
		repoSelector = "id:" + repoID
	} else {
		remoteStatus, err = supervisedRuntimeStatus(ctx, client, sc.EnvironmentID)
		if err != nil {
			return PreflightResult{}, fmt.Errorf("remote Orca status: %w", err)
		}
	}
	if remoteStatus.State != "ready" || !remoteStatus.Reachable || remoteStatus.RuntimeID == "" || remoteStatus.AppVersion == "" {
		return PreflightResult{}, errors.New("remote Orca runtime is not ready or omitted its version/identity")
	}
	if c.ExpectedRemoteRuntime != "" && remoteStatus.RuntimeID != c.ExpectedRemoteRuntime {
		return PreflightResult{}, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "original remote runtime epoch changed"}
	}
	if !SameVersionFamily(centerVersion, remoteStatus.AppVersion) {
		return PreflightResult{}, fmt.Errorf("Orca version-family mismatch: center=%s remote=%s", centerVersion, remoteStatus.AppVersion)
	}
	if !c.local() {
		if err = requireOrcaCapabilities(remoteStatus.Capabilities); err != nil {
			return PreflightResult{}, fmt.Errorf("remote Orca lacks required supervised capabilities: %w", err)
		}
		if err = verifySavedEnvironment(ctx, client, sc.EnvironmentID, sc.ConnectionID, remoteStatus.RuntimeID); err != nil {
			return PreflightResult{}, err
		}
	}

	repoArgs := []string{"repo", "show", "--repo", repoSelector, "--json"}
	if !c.local() {
		repoArgs = append(repoArgs, "--environment", sc.EnvironmentID)
	}
	repoBytes, err := client.config.Call(ctx, timeout, repoArgs...)
	if err != nil {
		return PreflightResult{}, fmt.Errorf("remote repository lookup: %w", err)
	}
	repoEnvelope, err := parseEnvelope(repoBytes)
	if err != nil {
		return PreflightResult{}, fmt.Errorf("remote repository response: %w", err)
	}
	var repoResult struct {
		Repo struct {
			ID                string `json:"id"`
			Kind              string `json:"kind"`
			GitRemoteIdentity struct {
				CanonicalKey string `json:"canonicalKey"`
				RemoteURL    string `json:"remoteUrl"`
			} `json:"gitRemoteIdentity"`
		} `json:"repo"`
	}
	if err = json.Unmarshal(repoEnvelope.Result, &repoResult); err != nil {
		return PreflightResult{}, fmt.Errorf("decode remote repository response: %w", err)
	}
	if repoResult.Repo.ID == "" || repoResult.Repo.Kind != "git" || repoResult.Repo.GitRemoteIdentity.RemoteURL == "" {
		return PreflightResult{}, errors.New("remote repository response lacks a Git identity")
	}
	if c.local() && (repoEnvelope.Meta.RuntimeID != remoteStatus.RuntimeID || "id:"+repoResult.Repo.ID != repoSelector) {
		return PreflightResult{}, errors.New("Orca repository response came from a different execution runtime")
	}
	localRemote, err := c.Origin(ctx)
	if err != nil {
		return PreflightResult{}, fmt.Errorf("local repository remote lookup: %w", err)
	}
	if normalizeRemoteURL(strings.TrimSpace(string(localRemote))) != normalizeRemoteURL(repoResult.Repo.GitRemoteIdentity.RemoteURL) {
		return PreflightResult{}, errors.New("remote repository origin differs from the configured local repository")
	}

	// A new Execution creates its own coordinator. An optional configured
	// bootstrap handle is checked only when explicitly supplied.
	if sc.CoordinatorTerminal == "" {
		return PreflightResult{CenterVersion: centerVersion, RemoteVersion: remoteStatus.AppVersion, CenterRuntime: centerStatus.RuntimeID, RemoteRuntime: remoteStatus.RuntimeID, EnvironmentID: c.executionHost(), RepositoryID: repoResult.Repo.ID, RepositoryURL: repoResult.Repo.GitRemoteIdentity.RemoteURL}, nil
	}
	terminalBytes, err := client.config.Call(ctx, timeout, "terminal", "show", "--terminal", sc.CoordinatorTerminal, "--json")
	if err != nil {
		return PreflightResult{}, fmt.Errorf("coordinator terminal lookup: %w", err)
	}
	terminalEnvelope, err := parseEnvelope(terminalBytes)
	if err != nil {
		return PreflightResult{}, fmt.Errorf("coordinator terminal response: %w", err)
	}
	var terminalResult struct {
		Terminal struct {
			Handle    string `json:"handle"`
			Connected bool   `json:"connected"`
			Orphaned  bool   `json:"orphaned"`
		} `json:"terminal"`
	}
	if err = json.Unmarshal(terminalEnvelope.Result, &terminalResult); err != nil {
		return PreflightResult{}, fmt.Errorf("decode coordinator terminal response: %w", err)
	}
	if terminalResult.Terminal.Handle != sc.CoordinatorTerminal || !terminalResult.Terminal.Connected || terminalResult.Terminal.Orphaned || terminalEnvelope.Meta.RuntimeID != centerStatus.RuntimeID {
		return PreflightResult{}, errors.New("coordinator terminal is not connected to the current center runtime")
	}
	return PreflightResult{CenterVersion: centerVersion, RemoteVersion: remoteStatus.AppVersion, CenterRuntime: centerStatus.RuntimeID, RemoteRuntime: remoteStatus.RuntimeID, EnvironmentID: c.executionHost(), RepositoryID: repoResult.Repo.ID, RepositoryURL: repoResult.Repo.GitRemoteIdentity.RemoteURL, Coordinator: sc.CoordinatorTerminal}, nil
}

func supervisedRuntimeStatus(ctx context.Context, client Client, environment string) (orcaRuntimeStatus, error) {
	args := []string{"status", "--json"}
	if environment != "" {
		args = []string{"status", "--environment", environment, "--json"}
	}
	out, err := client.config.Call(ctx, positiveDuration(client.config.CommandTimeout, 30*time.Second), args...)
	if err != nil {
		return orcaRuntimeStatus{}, err
	}
	envelope, err := parseEnvelope(out)
	if err != nil {
		return orcaRuntimeStatus{}, err
	}
	var result struct {
		Runtime orcaRuntimeStatus     `json:"runtime"`
		Target  struct{ Kind string } `json:"target"`
	}
	if err = json.Unmarshal(envelope.Result, &result); err != nil {
		return orcaRuntimeStatus{}, err
	}
	status := result.Runtime
	status.TargetKind = result.Target.Kind
	if envelope.Meta.RuntimeID == "" || status.RuntimeID != "" && status.RuntimeID != envelope.Meta.RuntimeID {
		return orcaRuntimeStatus{}, errors.New("Orca status envelope/runtime identity mismatch")
	}
	status.RuntimeID = envelope.Meta.RuntimeID
	return status, nil
}

func verifySavedEnvironment(ctx context.Context, client Client, environmentID, connectionID, runtimeID string) error {
	out, err := client.config.Call(ctx, positiveDuration(client.config.CommandTimeout, 30*time.Second), "environment", "list", "--json")
	if err != nil {
		return fmt.Errorf("saved environment list: %w", err)
	}
	env, err := parseEnvelope(out)
	if err != nil {
		return fmt.Errorf("saved environment list response: %w", err)
	}
	var result struct {
		Environments []struct {
			ID             string `json:"id"`
			RuntimeID      string `json:"runtimeId"`
			PairedDeviceID string `json:"pairedDeviceId"`
		} `json:"environments"`
	}
	if err = json.Unmarshal(env.Result, &result); err != nil {
		return fmt.Errorf("decode saved environments: %w", err)
	}
	for _, saved := range result.Environments {
		if saved.ID == environmentID {
			if saved.RuntimeID != runtimeID {
				return errors.New("saved environment runtime identity differs from the connected remote status")
			}
			if connectionID == "" || saved.PairedDeviceID != connectionID {
				return errors.New("saved environment paired-device identity differs from the fixed connection_id")
			}
			return nil
		}
	}
	return errors.New("configured environment ID is not a saved Orca connection")
}

func requireOrcaCapabilities(capabilities []string) error {
	got := make(map[string]bool, len(capabilities))
	for _, capability := range capabilities {
		got[capability] = true
	}
	required := []string{
		"orchestration.federation.v1",
		"orchestration.federation-control-mail.v1",
		"orchestration.federation-lifecycle-settlement.v1",
		"orchestration.worker-launch-preferences.v1",
		"orchestration.federation-structured-read.v1",
		"orchestration.federation-fleet-snapshot.v1",
		"orchestration.federation-release-archive.v1",
		"agent.launch.v2",
	}
	missing := []string{}
	for _, capability := range required {
		if !got[capability] {
			missing = append(missing, capability)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing %s", strings.Join(missing, ", "))
	}
	return nil
}

func normalizeRemoteURL(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ".git")
	return strings.TrimSuffix(value, "/")
}
