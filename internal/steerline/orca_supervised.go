package steerline

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxSupervisedMessageBytes = 8 << 10

// supervisedPreflight is deliberately read-only. It proves the configured
// center/remote pair, repository selector, coordinator terminal, and required
// federation surface before a workflow can claim a task. It does not claim
// that worker start, message application, or result settlement is implemented.
type supervisedPreflight struct {
	CenterVersion string
	RemoteVersion string
	CenterRuntime string
	RemoteRuntime string
	EnvironmentID string
	RepositoryID  string
	RepositoryURL string
	Coordinator   string
}

type supervisedStartReceipt struct {
	RunID             string
	TaskID            string
	DispatchID        string
	WorktreeID        string
	CenterRuntimeID   string
	PlatformRequestID string
	State             string
	Stage             string
}

type supervisedWorkerObservation struct {
	Identity       ProcessIdentity
	Liveness       string
	DispatchState  string
	AgentWaitState string
	WorktreePath   string
	Branch         string
	ObservedAt     time.Time
}

type supervisedDelivery struct {
	ID       string
	Messages []OrcaInboxMessage
	TimedOut bool
}

type orcaRuntimeStatus struct {
	State        string   `json:"state"`
	Reachable    bool     `json:"reachable"`
	RuntimeID    string   `json:"runtimeId"`
	AppVersion   string   `json:"appVersion"`
	Capabilities []string `json:"capabilities"`
}

func probeSupervisedOrcaTarget(ctx context.Context, c Config) (supervisedPreflight, error) {
	if err := validateSupervisedOrcaConfig(&c); err != nil {
		return supervisedPreflight{}, err
	}
	sc := c.Orca.Supervised
	if c.Model == "" || c.ReasoningEffort == "" || c.agentName() != "codex" {
		return supervisedPreflight{}, errors.New("supervised Orca requires an explicit Codex model and reasoning effort")
	}
	client := orcaClient{cfg: c}
	timeout := positiveDuration(c.CommandTimeout, 30*time.Second)
	centerVersionBytes, err := client.call(ctx, timeout, "--version")
	if err != nil {
		return supervisedPreflight{}, fmt.Errorf("center Orca version probe: %w", err)
	}
	centerVersion := strings.TrimSpace(string(centerVersionBytes))
	if _, ok := orcaVersionFamily(centerVersion); !ok {
		return supervisedPreflight{}, fmt.Errorf("center Orca version %q has no major.minor family", centerVersion)
	}
	centerStatus, err := supervisedRuntimeStatus(ctx, client, "")
	if err != nil {
		return supervisedPreflight{}, fmt.Errorf("center Orca status: %w", err)
	}
	if centerStatus.State != "ready" || !centerStatus.Reachable || centerStatus.RuntimeID == "" {
		return supervisedPreflight{}, errors.New("center Orca runtime is not ready")
	}
	if centerStatus.AppVersion != "" && !sameOrcaVersionFamily(centerVersion, centerStatus.AppVersion) {
		return supervisedPreflight{}, errors.New("center Orca CLI and runtime versions are from different major.minor families")
	}

	remoteStatus, err := supervisedRuntimeStatus(ctx, client, sc.EnvironmentID)
	if err != nil {
		return supervisedPreflight{}, fmt.Errorf("remote Orca status: %w", err)
	}
	if remoteStatus.State != "ready" || !remoteStatus.Reachable || remoteStatus.RuntimeID == "" || remoteStatus.AppVersion == "" {
		return supervisedPreflight{}, errors.New("remote Orca runtime is not ready or omitted its version/identity")
	}
	if !sameOrcaVersionFamily(centerVersion, remoteStatus.AppVersion) {
		return supervisedPreflight{}, fmt.Errorf("Orca version-family mismatch: center=%s remote=%s", centerVersion, remoteStatus.AppVersion)
	}
	if err = requireOrcaCapabilities(remoteStatus.Capabilities); err != nil {
		return supervisedPreflight{}, fmt.Errorf("remote Orca lacks required supervised capabilities: %w", err)
	}
	if err = verifySavedEnvironment(ctx, client, sc.EnvironmentID, sc.ConnectionID, remoteStatus.RuntimeID); err != nil {
		return supervisedPreflight{}, err
	}

	repoBytes, err := client.call(ctx, timeout, "repo", "show", "--repo", sc.RepositorySelector, "--environment", sc.EnvironmentID, "--json")
	if err != nil {
		return supervisedPreflight{}, fmt.Errorf("remote repository lookup: %w", err)
	}
	repoEnvelope, err := parseEnvelope(repoBytes)
	if err != nil {
		return supervisedPreflight{}, fmt.Errorf("remote repository response: %w", err)
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
		return supervisedPreflight{}, fmt.Errorf("decode remote repository response: %w", err)
	}
	if repoResult.Repo.ID == "" || repoResult.Repo.Kind != "git" || repoResult.Repo.GitRemoteIdentity.RemoteURL == "" {
		return supervisedPreflight{}, errors.New("remote repository response lacks a Git identity")
	}
	localRemote, err := (Runner{Dir: c.Repository, Timeout: timeout}).Output(ctx, c.Git, "remote", "get-url", c.Remote)
	if err != nil {
		return supervisedPreflight{}, fmt.Errorf("local repository remote lookup: %w", err)
	}
	if normalizeRemoteURL(strings.TrimSpace(string(localRemote))) != normalizeRemoteURL(repoResult.Repo.GitRemoteIdentity.RemoteURL) {
		return supervisedPreflight{}, errors.New("remote repository origin differs from the configured local repository")
	}

	terminalBytes, err := client.call(ctx, timeout, "terminal", "show", "--terminal", sc.CoordinatorTerminal, "--json")
	if err != nil {
		return supervisedPreflight{}, fmt.Errorf("coordinator terminal lookup: %w", err)
	}
	terminalEnvelope, err := parseEnvelope(terminalBytes)
	if err != nil {
		return supervisedPreflight{}, fmt.Errorf("coordinator terminal response: %w", err)
	}
	var terminalResult struct {
		Terminal struct {
			Handle    string `json:"handle"`
			Connected bool   `json:"connected"`
			Orphaned  bool   `json:"orphaned"`
		} `json:"terminal"`
	}
	if err = json.Unmarshal(terminalEnvelope.Result, &terminalResult); err != nil {
		return supervisedPreflight{}, fmt.Errorf("decode coordinator terminal response: %w", err)
	}
	if terminalResult.Terminal.Handle != sc.CoordinatorTerminal || !terminalResult.Terminal.Connected || terminalResult.Terminal.Orphaned || terminalEnvelope.Meta.RuntimeID != centerStatus.RuntimeID {
		return supervisedPreflight{}, errors.New("coordinator terminal is not connected to the current center runtime")
	}
	return supervisedPreflight{CenterVersion: centerVersion, RemoteVersion: remoteStatus.AppVersion, CenterRuntime: centerStatus.RuntimeID, RemoteRuntime: remoteStatus.RuntimeID, EnvironmentID: sc.EnvironmentID, RepositoryID: repoResult.Repo.ID, RepositoryURL: repoResult.Repo.GitRemoteIdentity.RemoteURL, Coordinator: sc.CoordinatorTerminal}, nil
}

func supervisedRuntimeStatus(ctx context.Context, client orcaClient, environment string) (orcaRuntimeStatus, error) {
	args := []string{"status", "--json"}
	if environment != "" {
		args = []string{"status", "--environment", environment, "--json"}
	}
	out, err := client.call(ctx, positiveDuration(client.cfg.CommandTimeout, 30*time.Second), args...)
	if err != nil {
		return orcaRuntimeStatus{}, err
	}
	envelope, err := parseEnvelope(out)
	if err != nil {
		return orcaRuntimeStatus{}, err
	}
	var result struct {
		Runtime orcaRuntimeStatus `json:"runtime"`
	}
	if err = json.Unmarshal(envelope.Result, &result); err != nil {
		return orcaRuntimeStatus{}, err
	}
	status := result.Runtime
	if envelope.Meta.RuntimeID == "" || status.RuntimeID != "" && status.RuntimeID != envelope.Meta.RuntimeID {
		return orcaRuntimeStatus{}, errors.New("Orca status envelope/runtime identity mismatch")
	}
	status.RuntimeID = envelope.Meta.RuntimeID
	return status, nil
}

func verifySavedEnvironment(ctx context.Context, client orcaClient, environmentID, connectionID, runtimeID string) error {
	out, err := client.call(ctx, positiveDuration(client.cfg.CommandTimeout, 30*time.Second), "environment", "list", "--json")
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

func (b orcaSupervisedBackend) createOrchestrationRun(ctx context.Context, workflowRunID, objective string) (string, string, error) {
	if !stableExecutionID(workflowRunID) || strings.TrimSpace(objective) == "" {
		return "", "", &BackendError{Code: BackendRejected, Diagnostic: "supervised Orca Run requires a fixed workflow identity and objective"}
	}
	if err := validateSupervisedOrcaConfig(&b.config); err != nil {
		return "", "", &BackendError{Code: BackendRejected, Diagnostic: "supervised Orca configuration is invalid", Cause: err}
	}
	pair, err := probeSupervisedOrcaTarget(ctx, b.config)
	if err != nil {
		return "", "", &BackendError{Code: BackendUnreachable, Diagnostic: "supervised Orca preflight failed before Run creation", Cause: err}
	}
	sc := b.config.Orca.Supervised
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "run-create", "--objective", objective, "--from", sc.CoordinatorTerminal, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		if runID, reconcileErr := b.currentRunForObjective(ctx, objective, pair.CenterRuntime); reconcileErr == nil {
			return runID, "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca Run was found by its fixed objective, but its platform request UUID is unavailable"}
		}
		return "", "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised Orca Run creation may have been applied; no platform request UUID was received and exact reconciliation failed", Cause: errors.Join(callErr, parseErr)}
	}
	var result struct {
		Run struct {
			ID string `json:"id"`
		} `json:"run"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || result.Run.ID == "" {
		if runID, reconcileErr := b.currentRunForObjective(ctx, objective, pair.CenterRuntime); reconcileErr == nil {
			return runID, "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca Run was found by its fixed objective, but its platform request UUID is unavailable"}
		}
		return "", "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca Run creation receipt lacks its stable Run ID", Cause: err}
	}
	if env.Meta.RuntimeID != pair.CenterRuntime {
		return "", "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca Run creation receipt came from a different center runtime"}
	}
	if result.Mutation.RequestID == "" {
		return result.Run.ID, "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca Run was created but its platform request UUID was omitted; persist the exact Run and reconcile before another mutation"}
	}
	return result.Run.ID, result.Mutation.RequestID, nil
}

func (b orcaSupervisedBackend) currentRunForObjective(ctx context.Context, objective, centerRuntime string) (string, error) {
	sc := b.config.Orca.Supervised
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "run-current", "--from", sc.CoordinatorTerminal, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return "", errors.Join(callErr, parseErr)
	}
	if env.Meta.RuntimeID != centerRuntime {
		return "", errors.New("coordinator runtime changed while reconciling run-create")
	}
	var result struct {
		Run struct {
			ID        string `json:"id"`
			Objective string `json:"objective"`
		} `json:"run"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || result.Run.ID == "" || result.Run.Objective != objective {
		return "", errors.New("current coordinator Run does not uniquely match the fixed objective")
	}
	return result.Run.ID, nil
}

// startWorker uses only the supervised worker API. It never switches to a
// local process, terminal-v2 start, or guessed worktree selector.
func (b orcaSupervisedBackend) startWorker(ctx context.Context, workflowRunID, attemptID, objective, taskSpec, worktreeSelector, worktreeName, repositorySelector, baseCommit string) (supervisedStartReceipt, error) {
	if strings.TrimSpace(workflowRunID) == "" || !stableExecutionID(attemptID) || strings.TrimSpace(taskSpec) == "" || strings.TrimSpace(objective) == "" {
		return supervisedStartReceipt{}, &BackendError{Code: BackendRejected, Diagnostic: "supervised worker start lacks a fixed Run/attempt or task spec"}
	}
	if len([]byte(taskSpec)) > 64<<10 {
		return supervisedStartReceipt{}, &BackendError{Code: BackendRejected, Diagnostic: "supervised task spec exceeds the 64 KiB local request bound"}
	}
	sc := b.config.Orca.Supervised
	if sc == nil || strings.TrimSpace(sc.EnvironmentID) == "" || strings.TrimSpace(sc.CoordinatorTerminal) == "" || b.config.Model == "" || b.config.ReasoningEffort == "" {
		return supervisedStartReceipt{}, &BackendError{Code: BackendRejected, Diagnostic: "supervised worker start requires fixed environment, coordinator, model, and effort"}
	}
	pair, preflightErr := probeSupervisedOrcaTarget(ctx, b.config)
	if preflightErr != nil {
		return supervisedStartReceipt{}, &BackendError{Code: BackendUnreachable, Diagnostic: "supervised target preflight failed before worker start", Cause: preflightErr}
	}
	args := []string{"orchestration", "worker-start", "--spec", taskSpec, "--task-title", objective, "--on", sc.EnvironmentID, "--agent", "codex", "--model", b.config.Model, "--effort", b.config.ReasoningEffort, "--run", workflowRunID, "--from", sc.CoordinatorTerminal, "--json"}
	switch {
	case worktreeSelector == "new-top-level":
		if strings.TrimSpace(repositorySelector) == "" || strings.TrimSpace(worktreeName) == "" || !commitHash.MatchString(baseCommit) {
			return supervisedStartReceipt{}, &BackendError{Code: BackendRejected, Diagnostic: "new supervised worktree requires an exact repo selector, name, and fixed base commit"}
		}
		args = append(args, "--worktree", "new-top-level", "--repo", repositorySelector, "--name", worktreeName, "--base-branch", baseCommit)
	case strings.HasPrefix(worktreeSelector, "id:"):
		selectedWorktree := strings.TrimPrefix(worktreeSelector, "id:")
		if selectedWorktree == "" || !strings.HasPrefix(selectedWorktree, pair.RepositoryID+"::") {
			return supervisedStartReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "existing supervised worktree selector does not belong to the pinned remote repository"}
		}
		args = append(args, "--worktree", worktreeSelector)
	default:
		return supervisedStartReceipt{}, &BackendError{Code: BackendRejected, Diagnostic: "supervised worktree must be new-top-level or an exact id:<repo-id>::<path> selector"}
	}
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), args...)
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return supervisedStartReceipt{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised worker start may have been applied; reconcile using its exact retry request", Cause: errors.Join(callErr, parseErr)}
	}
	var result struct {
		RunID      string `json:"runId"`
		TaskID     string `json:"taskId"`
		DispatchID string `json:"dispatchId"`
		State      string `json:"state"`
		Stage      string `json:"stage"`
		Server     struct {
			EnvironmentID string `json:"environmentId"`
		} `json:"server"`
		Worker struct {
			WorktreeID string `json:"worktreeId"`
			Effects    []struct {
				Kind   string `json:"kind"`
				Action string `json:"action"`
				ID     string `json:"id"`
			} `json:"effects"`
		} `json:"worker"`
		Launch struct {
			Effective struct {
				Agent  string `json:"agent"`
				Model  string `json:"model"`
				Effort string `json:"effort"`
			} `json:"effective"`
		} `json:"launch"`
		Effects []struct {
			Kind   string `json:"kind"`
			Action string `json:"action"`
			ID     string `json:"id"`
		} `json:"effects"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return supervisedStartReceipt{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised worker start receipt is malformed", Cause: err}
	}
	startReceipt := supervisedStartReceipt{RunID: result.RunID, TaskID: result.TaskID, DispatchID: result.DispatchID, WorktreeID: result.Worker.WorktreeID, CenterRuntimeID: pair.CenterRuntime, PlatformRequestID: result.Mutation.RequestID, State: result.State, Stage: result.Stage}
	if result.RunID != workflowRunID || result.TaskID == "" || result.DispatchID == "" || result.Server.EnvironmentID != sc.EnvironmentID || result.Launch.Effective.Agent != "codex" || result.Launch.Effective.Model != b.config.Model || result.Launch.Effective.Effort != b.config.ReasoningEffort {
		return startReceipt, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised worker start receipt does not match the fixed Run/environment/launch identity"}
	}
	if result.Mutation.RequestID == "" {
		return startReceipt, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "worker-start receipt omitted Orca's platform request UUID; preserve the exact Task/Dispatch and do not retry with a generated ID"}
	}
	if env.Meta.RuntimeID != pair.CenterRuntime {
		return startReceipt, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "worker-start receipt came from a different center runtime"}
	}
	worktreeID := result.Worker.WorktreeID
	for _, effect := range append(result.Effects, result.Worker.Effects...) {
		if worktreeID == "" && effect.Kind == "worktree" && (effect.Action == "created" || effect.Action == "created_top_level" || effect.Action == "reused") {
			worktreeID = effect.ID
		}
	}
	if worktreeID == "" {
		return startReceipt, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised worker start omitted exact worktree identity"}
	}
	startReceipt.WorktreeID = worktreeID
	if worktreeSelector == "new-top-level" && !strings.HasPrefix(worktreeID, pair.RepositoryID+"::") {
		return startReceipt, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "new supervised worktree belongs to a different remote repository"}
	}
	if worktreeSelector != "new-top-level" && worktreeID != strings.TrimPrefix(worktreeSelector, "id:") {
		return startReceipt, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised worker started in a different worktree"}
	}
	if result.State != "ready" || result.Stage != "input_accepted" {
		return supervisedStartReceipt{RunID: result.RunID, TaskID: result.TaskID, DispatchID: result.DispatchID, WorktreeID: worktreeID, PlatformRequestID: result.Mutation.RequestID, State: result.State, Stage: result.Stage}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised worker did not reach input_accepted; reconcile exact Dispatch before retry"}
	}
	return supervisedStartReceipt{RunID: result.RunID, TaskID: result.TaskID, DispatchID: result.DispatchID, WorktreeID: worktreeID, CenterRuntimeID: pair.CenterRuntime, PlatformRequestID: result.Mutation.RequestID, State: result.State, Stage: result.Stage}, nil
}

func (b orcaSupervisedBackend) showWorker(ctx context.Context, expected supervisedStartReceipt) (supervisedWorkerObservation, error) {
	if expected.RunID == "" || expected.TaskID == "" || expected.DispatchID == "" || expected.WorktreeID == "" {
		return supervisedWorkerObservation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "worker inspection requires exact Run/Task/Dispatch/worktree identity"}
	}
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "worker-show", "--dispatch", expected.DispatchID, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return supervisedWorkerObservation{}, &BackendError{Code: BackendUnreachable, Diagnostic: "exact supervised Dispatch inspection failed", Cause: errors.Join(callErr, parseErr)}
	}
	var result struct {
		Dispatch struct {
			ID     string `json:"id"`
			RunID  string `json:"runId"`
			TaskID string `json:"taskId"`
			Status string `json:"status"`
		} `json:"dispatch"`
		Worker struct {
			DispatchID string `json:"dispatchId"`
			WorktreeID string `json:"worktreeId"`
		} `json:"worker"`
		Projection struct {
			Liveness struct {
				Verdict string `json:"verdict"`
			} `json:"liveness"`
			Host struct {
				ID string `json:"id"`
			} `json:"host"`
		} `json:"projection"`
		Server struct {
			EnvironmentID string `json:"environmentId"`
		} `json:"server"`
		RemoteRuntimeEpoch string `json:"remoteRuntimeEpoch"`
		Terminal           struct {
			Handle        string          `json:"handle"`
			IncarnationID string          `json:"incarnationId"`
			WorktreeID    string          `json:"worktreeId"`
			WorktreePath  string          `json:"worktreePath"`
			Branch        string          `json:"branch"`
			AgentWait     json.RawMessage `json:"agentWait"`
		} `json:"terminal"`
		Observation struct {
			ExactWorker bool            `json:"exactWorker"`
			AgentWait   json.RawMessage `json:"agentWait"`
		} `json:"observation"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return supervisedWorkerObservation{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "supervised worker-show response is malformed", Cause: err}
	}
	if env.Meta.RuntimeID != expected.CenterRuntimeID || result.Dispatch.ID != expected.DispatchID || result.Dispatch.RunID != expected.RunID || result.Dispatch.TaskID != expected.TaskID || result.Worker.DispatchID != expected.DispatchID || result.Worker.WorktreeID != expected.WorktreeID || result.Terminal.WorktreeID != expected.WorktreeID || result.Server.EnvironmentID != b.config.Orca.Supervised.EnvironmentID || result.Projection.Host.ID != b.config.Orca.Supervised.EnvironmentID || result.RemoteRuntimeEpoch == "" || !result.Observation.ExactWorker {
		return supervisedWorkerObservation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "worker-show does not match the fixed Run/Task/Dispatch/host/worktree identity"}
	}
	if result.Terminal.Handle == "" || result.Terminal.IncarnationID == "" || result.Terminal.WorktreePath == "" || result.Terminal.Branch == "" {
		return supervisedWorkerObservation{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "supervised agent terminal or workspace identity is incomplete"}
	}
	identity := ProcessIdentity{Kind: "orca_supervised", RuntimeID: result.RemoteRuntimeEpoch, CoordinatorRuntimeID: expected.CenterRuntimeID, Terminal: result.Terminal.Handle, Incarnation: result.Terminal.IncarnationID, OrchestrationRun: expected.RunID, Task: expected.TaskID, Dispatch: expected.DispatchID, ExecutionHost: result.Projection.Host.ID, WorktreeID: expected.WorktreeID}
	wait := result.Observation.AgentWait
	if len(wait) == 0 {
		wait = result.Terminal.AgentWait
	}
	waitState := "unknown"
	if len(wait) > 0 {
		waitState = "present"
		if string(wait) == "null" {
			waitState = "none"
		}
	}
	return supervisedWorkerObservation{Identity: identity, Liveness: result.Projection.Liveness.Verdict, DispatchState: result.Dispatch.Status, AgentWaitState: waitState, WorktreePath: result.Terminal.WorktreePath, Branch: strings.TrimPrefix(result.Terminal.Branch, "refs/heads/"), ObservedAt: now()}, nil
}

func (b orcaSupervisedBackend) StartMinimalStep(ctx context.Context, task Task, run Run, workflow WorkflowRunRecord, execution ExecutionRecord, attempt StepAttemptRecord, input MinimalStepInput, resultPath string) (StartReceipt, error) {
	if err := validateSupervisedOrcaConfig(&b.config); err != nil {
		return StartReceipt{}, &BackendError{Code: BackendRejected, Diagnostic: "supervised Orca configuration is invalid", Cause: err}
	}
	if execution.ExecutionID != run.ExecutionID || execution.WorkflowRunID != run.RunID || workflow.WorkflowRunID != run.RunID || workflow.WorkflowVersion != MinimalWorkflowVersion {
		return StartReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised step belongs to a different local workflow/execution"}
	}
	if attempt.ExecutionID != execution.ExecutionID || attempt.AttemptID != input.AttemptID || attempt.StepID != input.StepID || input.ProjectIdentity != b.config.identity || input.TaskID != run.DomainTaskID || input.WorkflowRunID != workflow.WorkflowRunID || input.ExecutionID != execution.ExecutionID || workflow.Runner.Model != b.config.Model || workflow.Runner.ReasoningEffort != b.config.ReasoningEffort {
		return StartReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised step input/launch differs from the pinned workflow and attempt"}
	}
	pair, err := probeSupervisedOrcaTarget(ctx, b.config)
	if err != nil {
		return StartReceipt{}, &BackendError{Code: BackendUnreachable, Diagnostic: "supervised step preflight failed", Cause: err}
	}
	r := cloneRun(run)
	fixedTarget := targetFor(b.config)
	if r.Target.ID == "" {
		r.Target = fixedTarget
	} else if r.Target.ID != fixedTarget.ID || r.Target.ConnectionID != fixedTarget.ConnectionID || r.Target.ExternalRuntime != fixedTarget.ExternalRuntime {
		return StartReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "persisted execution target differs from the fixed supervised connection"}
	}
	orcaRunID := r.Target.OrchestrationRun
	if orcaRunID == "" {
		return StartReceipt{}, &BackendError{Code: BackendRejected, Diagnostic: "supervised Run creation must be journaled before worker start"}
	}
	base := deref(run.BaseCommit)
	if base == "" && run.WorkspaceIntent != nil {
		base = run.WorkspaceIntent.BaseCommit
	}
	if !commitHash.MatchString(base) {
		return StartReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised step lacks its fixed base commit"}
	}
	worktreeSelector, worktreeName, repoSelector := "", "", ""
	if attempt.StepID == "prepare" && r.Target.WorktreeID == "" {
		worktreeSelector = "new-top-level"
		worktreeName = "steerline-" + run.RunID[:12]
		repoSelector = b.config.Orca.Supervised.RepositorySelector
	} else {
		if r.Target.WorktreeID == "" {
			return StartReceipt{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "non-prepare supervised step has no fixed worktree ID"}
		}
		worktreeSelector = "id:" + r.Target.WorktreeID
	}
	objective := fmt.Sprintf("SteerLine %s %s", run.RunID[:12], attempt.StepID)
	taskSpec, err := supervisedStepTaskSpec(b.config, task, r, workflow, attempt, input, pair, base)
	if err != nil {
		return StartReceipt{}, err
	}
	started, err := b.startWorker(ctx, orcaRunID, attempt.AttemptID, objective, taskSpec, worktreeSelector, worktreeName, repoSelector, base)
	if err != nil {
		if started.RunID != "" {
			r.Target.OrchestrationRun = started.RunID
		}
		if started.TaskID != "" {
			r.Target.ExternalTask = started.TaskID
		}
		if started.DispatchID != "" {
			r.Target.Dispatch = started.DispatchID
		}
		if started.WorktreeID != "" {
			r.Target.WorktreeID = started.WorktreeID
		}
		if started.DispatchID != "" {
			r.Target.ExecutionHost = b.config.Orca.Supervised.EnvironmentID
		}
		return StartReceipt{Disposition: "outcome_unknown", PlatformRequestID: started.PlatformRequestID, Run: r, ResidualResources: []string{"worker-start may have created a remote Task, Dispatch, terminal, or worktree"}}, err
	}
	r.Target.OrchestrationRun = orcaRunID
	r.Target.WorktreeID = started.WorktreeID
	r.Target.ExternalTask = started.TaskID
	r.Target.Dispatch = started.DispatchID
	r.Target.ExecutionHost = b.config.Orca.Supervised.EnvironmentID
	observation, err := b.showWorker(ctx, started)
	if err != nil {
		return StartReceipt{Disposition: "outcome_unknown", PlatformRequestID: started.PlatformRequestID, Run: r, ResidualResources: []string{"exact Dispatch exists but its runtime/worktree identity could not be verified"}}, err
	}
	if observation.Liveness != "live" && observation.Liveness != "exited" {
		return StartReceipt{Disposition: "outcome_unknown", PlatformRequestID: started.PlatformRequestID, Run: r, ResidualResources: []string{"exact Dispatch liveness is unverifiable"}}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised worker liveness is unverifiable"}
	}
	expectedHead := base
	if attempt.StepID != "prepare" {
		expectedHead = attempt.InputRevision
	}
	r.WorktreePath = ptr(observation.WorktreePath)
	r.WorktreeBranch = ptr(observation.Branch)
	r.BaseCommit = ptr(base)
	if r.WorkspaceIntent == nil {
		r.WorkspaceIntent = &WorkspaceIntent{}
	}
	r.WorkspaceIntent.Path = observation.WorktreePath
	r.WorkspaceIntent.Branch = observation.Branch
	r.WorkspaceIntent.BaseCommit = base
	r.Target.Branch = observation.Branch
	r.Target.BaseRevision = base
	ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: r.Target, Agent: AgentSpec{Name: "codex"}, RunID: run.RunID, AttemptID: attempt.AttemptID, WorkspacePath: observation.WorktreePath, WorkspaceBranch: observation.Branch, BaseCommit: base, External: observation.Identity, Generation: observation.Identity.RuntimeID + ":" + observation.Identity.Incarnation}
	workspace, err := b.inspectSupervisedWorktree(ctx, pair, started.WorktreeID, observation.WorktreePath, observation.Branch, base, expectedHead)
	if err != nil {
		return StartReceipt{Disposition: "outcome_unknown", PlatformRequestID: started.PlatformRequestID, Ref: ref, Run: r, ResidualResources: []string{"exact Dispatch and remote worktree are retained for reconciliation"}}, err
	}
	r.Target.WorktreeID = started.WorktreeID
	r.Target.Branch = workspace.Branch
	r.Target.BaseRevision = workspace.Base
	r.Target.ExternalTask = started.TaskID
	r.Target.Dispatch = started.DispatchID
	r.Target.ExecutionHost = observation.Identity.ExecutionHost
	r.Target.OrchestrationRun = orcaRunID
	r.WorktreePath = ptr(workspace.Worktree)
	r.WorktreeBranch = ptr(workspace.Branch)
	r.BaseCommit = ptr(workspace.Base)
	if r.WorkspaceIntent == nil {
		r.WorkspaceIntent = &WorkspaceIntent{}
	}
	r.WorkspaceIntent.Path = workspace.Worktree
	r.WorkspaceIntent.Branch = workspace.Branch
	r.WorkspaceIntent.BaseCommit = workspace.Base
	ref.Target = r.Target
	ref.WorkspacePath = workspace.Worktree
	ref.WorkspaceBranch = workspace.Branch
	ref.BaseCommit = workspace.Base
	observerCtx, cancelObserver := context.WithCancel(context.Background())
	worker := &Worker{proc: &supervisedProcess{identity: observation.Identity, cancel: cancelObserver}, Events: make(chan WorkerEvent, 16), Done: make(chan error, 1), cancelObservation: cancelObserver, observationDone: make(chan struct{})}
	go b.observeWorker(observerCtx, worker, ref, attempt, input, resultPath)
	return StartReceipt{Disposition: "accepted", PlatformRequestID: started.PlatformRequestID, Worker: worker, Ref: ref, Run: r}, nil
}

// reattachSupervisedWorker observes only the original persisted Dispatch. It
// never creates a Run, worker, terminal, or worktree during service recovery.
func (s *Service) reattachSupervisedWorker(run *Run) error {
	if s.worker != nil || run == nil || run.Process == nil || run.FinishTaskStatus != "pending" && run.FinishTaskStatus != "waiting_decision" {
		return errors.New("supervised recovery requires one active, bound execution")
	}
	workflow, execution := s.minimalRecords(run)
	attempt := s.minimalAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil || attempt.ResultPath == "" || attempt.OperationID == "" || attempt.Status != "dispatched" && attempt.Status != "running" && attempt.Status != "waiting_decision" {
		return errors.New("supervised recovery lacks its current workflow, attempt, or result path")
	}
	var operation *OperationRecord
	for i := range s.State.Operations {
		if s.State.Operations[i].OperationID == attempt.OperationID {
			operation = &s.State.Operations[i]
			break
		}
	}
	if operation == nil || operation.Kind != "dispatch_step" || operation.Status != "confirmed" || operation.PlatformRequestID == "" || operation.PayloadDigest != operationDigest(operation.Request) {
		return errors.New("supervised start operation is not durably confirmed")
	}
	var request struct {
		Input MinimalStepInput `json:"input"`
	}
	if err := json.Unmarshal(operation.Request, &request); err != nil {
		return err
	}
	input := request.Input
	if input.ProjectIdentity != s.Config.identity || input.TaskID != run.DomainTaskID || input.WorkflowRunID != run.RunID || input.ExecutionID != run.ExecutionID || input.AttemptID != attempt.AttemptID || input.StepID != attempt.StepID {
		return errors.New("supervised start operation input differs from the persisted attempt")
	}
	ref, err := (executionRecords{root: s.Config.RunDir}).loadBinding(run.RunID, attempt.AttemptID)
	if err != nil {
		return err
	}
	if ref.Protocol != ProtocolOrcaSupervised || ref.External != *run.Process || ref.Target != run.Target || ref.AttemptID != attempt.AttemptID {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised recovery binding differs from the original Run and attempt"}
	}
	backend := orcaSupervisedBackend{config: s.Config}
	observation, err := backend.Observe(context.Background(), ref)
	if err != nil {
		return err
	}
	if observation.Liveness != "live" && observation.Liveness != "exited" {
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "original supervised Dispatch liveness remains unverifiable"}
	}
	observerCtx, cancelObserver := context.WithCancel(context.Background())
	worker := &Worker{proc: &supervisedProcess{identity: ref.External, cancel: cancelObserver}, Events: make(chan WorkerEvent, 16), Done: make(chan error, 1), cancelObservation: cancelObserver, observationDone: make(chan struct{})}
	s.worker, s.active = worker, run
	go backend.observeWorker(observerCtx, worker, ref, *attempt, input, attempt.ResultPath)
	return nil
}

func supervisedStepTaskSpec(c Config, task Task, run Run, workflow WorkflowRunRecord, attempt StepAttemptRecord, input MinimalStepInput, pair supervisedPreflight, base string) (string, error) {
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	contract := minimalStepContract(workflow.ProjectSteps, attempt.StepID)
	contractBytes, err := json.Marshal(contract)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Protocol: one SteerLine minimal-v1 %s step. This Dispatch does not own task claim/close and must not call Beads, start/finish all steps, push, create/merge a PR unless publish_pr is the fixed step, or edit outside the assigned Orca worktree.\n", attempt.StepID)
	fmt.Fprintf(&b, "The center has pinned model %s at effort %s. Before any edit, run `codex --version` and require exact output %q. If it differs, stop before editing and report failure.\n", c.Model, c.ReasoningEffort, c.Orca.Supervised.ExpectedCodexVersion)
	fmt.Fprintf(&b, "Local identity: project=%s task=%s workflow_run=%s execution=%s attempt=%s step=%s input_revision=%s. Fixed remote repository selector=%s repository_id=%s origin=%s base_commit=%s.\n", input.ProjectIdentity, input.TaskID, input.WorkflowRunID, input.ExecutionID, input.AttemptID, input.StepID, input.InputRevision, c.Orca.Supervised.RepositorySelector, pair.RepositoryID, pair.RepositoryURL, base)
	fmt.Fprintf(&b, "MinimalStepResultEnvelope identity fields to copy verbatim from the Step Input (do not derive, shorten, transpose, or replace them with Orca IDs): project_identity=%q; task_id=%q; workflow_run_id=%q; execution_id=%q; attempt_id=%q; result.step_id=%q. The outer status payload separately uses Orca taskId/dispatchId from the Dispatch preamble.\n", input.ProjectIdentity, input.TaskID, input.WorkflowRunID, input.ExecutionID, input.AttemptID, attempt.StepID)
	identityJSON, err := json.Marshal(struct {
		ProjectIdentity   string `json:"project_identity"`
		TaskID            string `json:"task_id"`
		WorkflowRunID     string `json:"workflow_run_id"`
		ExecutionID       string `json:"execution_id"`
		AttemptID         string `json:"attempt_id"`
		InputRevision     string `json:"input_revision"`
		CandidateRevision string `json:"candidate_revision"`
	}{input.ProjectIdentity, input.TaskID, input.WorkflowRunID, input.ExecutionID, input.AttemptID, input.InputRevision, input.CandidateRevision})
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "To avoid hand-transcription errors in local IDs and commit hashes, decode this exact UTF-8 base64 input object in pwsh.exe and use its properties when building step_result: $ids = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s')) | ConvertFrom-Json. Use $ids.input_revision for result.input_revision and, for verify/review/publish_pr, $ids.candidate_revision for result.revision, check.revision and the ancestry command. Do not type project_identity, attempt_id or candidate_revision by hand; the center rejects even one altered character. Before verify, review, or publish_pr, compare `git rev-parse HEAD` with $ids.candidate_revision and report a failed step if they differ; prepare has no candidate revision and instead compares HEAD to the fixed base.\n", base64.StdEncoding.EncodeToString(identityJSON))
	if run.Target.WorktreeID != "" {
		fmt.Fprintf(&b, "Existing exact worktree_id=%s path=%s branch=%s; verify these facts through Orca and Git before editing.\n", run.Target.WorktreeID, deref(run.WorktreePath), deref(run.WorktreeBranch))
	} else {
		b.WriteString("Orca worker-start has already created and assigned the new-top-level worktree for this prepare Dispatch. Do not call worktree create, git worktree add, or change to a second checkout. Stay in the assigned current worktree. Verify its exact ID/path via `orca worktree current --json`, then verify origin, branch, initial HEAD equal to the fixed base, clean status, and base ancestry before reporting success. Do not edit during prepare.\n")
	}
	b.WriteString("For every step, inspect the current repository/worktree/branch/HEAD/status and report WorkspaceEvidence from those commands. In WorkspaceEvidence, repository must exactly echo project_identity from the input; worktree must be the absolute path returned by `orca worktree current --json`; branch must be `git branch --show-current`; base/head must be full commit hashes; dirty must reflect `git status --porcelain`; verified_at must be an RFC3339 UTC time. Do not assume that Orca ready proves the step started. Execute only the contract for this step and return one MinimalStepResultEnvelope in a status payload; include schema_version, project_identity, task_id, workflow_run_id, execution_id, attempt_id, and result. The result must use only keys allowed by the provided JSON schema (additionalProperties is false): step_id, succeeded, input_revision, result_id, prerequisite_result_ids, workspace, and step-specific gate fields. For prepare, step_id must be prepare; include workspace and base_ancestor, but do not add worktree_id, origin, origin_verified, head_matches_base, clean_status, worktree_is_new_top_level, or other undeclared fields. Orca and Git observations in this task spec are already checked independently by the center. Set base_ancestor=true only when `git merge-base --is-ancestor <fixed_base> HEAD` exits 0. For implement, leave a clean committed revision. For verify, return each required check with exact command, script_version, exit_code, passed, evidence_ref, and actual_commit.\n")
	b.WriteString("For implement, include result.revision equal to the actual clean committed HEAD. For verify, include result.revision equal to the pinned candidate_revision, and run each required check against that exact commit. Each result.checks entry must include name, status, revision, command, script_version, exit_code, passed, evidence_ref, and actual_commit; use the observed command exit code and HEAD rather than inferring success from prose.\n")
	b.WriteString("For implement, use only these result keys: step_id, succeeded, input_revision, revision, result_id, prerequisite_result_ids, workspace, base_ancestor. If a decision was applied, first obtain acceptance of its separate status proof; do not add a bare applied_decision_id to the final result or report success without application proof. For verify, use only those shared keys plus checks; each check uses only name, status, revision, command, script_version, exit_code, passed, evidence_ref, actual_commit. Workspace uses only repository, worktree, branch, base, head, dirty, verified_at. Never add clean_status, origin, taskId, dispatchId, or any explanatory flags inside result or workspace; the strict schema rejects extras.\n")
	if attempt.StepID == "verify" {
		fmt.Fprintf(&b, "Before sending verify status, set `$candidate = $ids.candidate_revision`, compare `(git rev-parse HEAD).Trim()` to `$candidate`, then run `git merge-base --is-ancestor %s $candidate` and record its actual exit code. Do not substitute a manually typed hash or run the check against a different commit. The verify result MUST contain the exact key base_ancestor=true only when this command exits zero; omitting base_ancestor rejects the result even if every required check passed.\n", base)
		b.WriteString("If a required verify command exits nonzero, report the actual check with status=failed, passed=false, its nonzero exit_code and actual_commit, and set result.succeeded=false; omit failure_kind for a deterministic check failure. After Orca accepts that structured result, send worker_done outcome succeeded because the verify step executed and delivered a conclusive failure receipt; the center, not the worker, decides repair. Use worker_done outcome failed only when the step itself could not produce a valid receipt.\n")
	}
	if attempt.StepID == "review" {
		fmt.Fprintf(&b, "Review the already verified candidate %s read-only. Inspect the actual diff and report review_verdict=passed or review_verdict=changes_requested; describe concrete findings in the status body. Do not edit, commit, push, create a PR, or run a later step. The review result must keep revision, input_revision and workspace.head at that same candidate, keep the worktree clean, and report the actual base ancestry check. A changes_requested verdict is a conclusive review result: set succeeded=true, send worker_done outcome succeeded after Orca accepts its structured status, and let the center decide repair or budget exhaustion. Use failure_kind only if no valid review verdict could be produced.\n", input.CandidateRevision)
		b.WriteString("For review, use only these result keys: step_id, succeeded, input_revision, revision, result_id, prerequisite_result_ids, workspace, base_ancestor, and review_verdict. Do not report checks, pr_url, published_head, or a replacement revision.\n")
	}
	if attempt.StepID == "implement" || attempt.StepID == "verify" || attempt.StepID == "review" || attempt.StepID == "publish_pr" {
		revision := "<actual full committed HEAD>"
		if attempt.StepID == "verify" || attempt.StepID == "review" || attempt.StepID == "publish_pr" {
			revision = input.CandidateRevision
		}
		resultTemplate := map[string]any{"step_id": attempt.StepID, "succeeded": true, "input_revision": input.InputRevision, "revision": revision, "result_id": "<new unique result ID>", "prerequisite_result_ids": input.PrerequisiteResultIDs, "workspace": map[string]any{"repository": input.ProjectIdentity, "worktree": "<actual Orca absolute worktree path>", "branch": "<actual Git branch>", "base": base, "head": revision, "dirty": false, "verified_at": "<actual RFC3339 UTC observation time>"}, "base_ancestor": true}
		if attempt.StepID == "verify" {
			checks := make([]map[string]any, 0, len(workflow.ProjectSteps.Verify.RequiredChecks))
			for _, name := range workflow.ProjectSteps.Verify.RequiredChecks {
				checks = append(checks, map[string]any{"name": name, "status": "passed", "revision": revision, "command": strings.Join(workflow.ProjectSteps.Verify.Command, " "), "script_version": workflow.ProjectSteps.Verify.Entry, "exit_code": 0, "passed": true, "evidence_ref": "<actual evidence reference>", "actual_commit": revision})
			}
			resultTemplate["checks"] = checks
		}
		if attempt.StepID == "review" {
			resultTemplate["review_verdict"] = "<passed or changes_requested from the actual review>"
		}
		if attempt.StepID == "publish_pr" {
			resultTemplate["pr_url"] = "<exact newly created PR URL>"
			resultTemplate["published_head"] = revision
			fmt.Fprintf(&b, "Publish only this verified candidate %s from branch %s to the fixed base branch. Create a draft PR in the configured test repository, do not merge it, and return its exact URL. Before sending status, confirm actual HEAD is still the candidate, the worktree is clean, and `git merge-base --is-ancestor %s %s` exits zero; include base_ancestor=true. If the fixed source branch, exact head, and base already identify one PR, treat its URL only as a read-only reconciliation of the original publication; do not create a duplicate or search by approximate title.\n", revision, deref(run.WorktreeBranch), base, revision)
		}
		template := map[string]any{"schema_version": 1, "project_identity": input.ProjectIdentity, "task_id": input.TaskID, "workflow_run_id": input.WorkflowRunID, "execution_id": input.ExecutionID, "attempt_id": input.AttemptID, "result": resultTemplate}
		if templateBytes, marshalErr := json.Marshal(template); marshalErr == nil {
			b.WriteString("Exact step_result object template; replace placeholders with observed facts, adjust checks only from actual command receipts, and do not add any keys: " + string(templateBytes) + "\n")
		}
	}
	b.WriteString("Use the Dispatch preamble's exact taskId and dispatchId. Send one type=status message whose JSON object has taskId, dispatchId, remote_codex_version, and step_result; put this JSON only in the CLI's --payload argument. The message body is plain prose and is not read as a structured step result. Reject the payload before sending if its UTF-8 size exceeds 8192 bytes. For the status command, call your exec_command tool with shell set to pwsh.exe and cmd set to the PowerShell script itself; verify $PSVersionTable.PSVersion.Major is 7 inside that same call. Never launch pwsh.exe -Command from a Windows PowerShell 5.1 command, because its outer shell expands the script before pwsh sees it. In that direct PowerShell 7 script, generate JSON with ConvertTo-Json -Compress -Depth 16, validate it locally, and invoke the preamble-authorized Orca executable directly with its exact --from/--dispatch-capability arguments and --payload as one string argument. Do not copy the capability into a helper process, source file, or environment variable. Do not use --body or structured task/dispatch payload flags with --payload. If direct PowerShell 7 invocation is rejected or cannot prove acceptance, report failure without another wrapper or malformed send. Then send exactly one worker_done using the preamble's --task-id/--dispatch-id and outcome flags, and its required three-sentence body. Do not report succeeded unless the status payload was accepted.\n")
	fmt.Fprintf(&b, "For the final result status, the top-level payload MUST be {\"taskId\":<Orca task ID>,\"dispatchId\":<Orca Dispatch ID>,\"remote_codex_version\":%q,\"step_result\":<the complete MinimalStepResultEnvelope>}. Validate these four top-level keys before sending. A bare MinimalStepResultEnvelope is rejected even when its inner fields are correct.\n", c.Orca.Supervised.ExpectedCodexVersion)
	b.WriteString("The type=status CLI argv is: orca orchestration send --from <preamble sender handle> --dispatch-capability <preamble capability> --type status --subject <short subject> --payload <one JSON argument> --json. Federated Dispatch messages must omit --run and --to; Orca routes them to the Run home from the exact preamble sender and capability. The worker_done CLI argv uses the same preamble --from/--dispatch-capability and its exact --task-id/--dispatch-id/--outcome. Parse the JSON response and require ok=true and a nonempty result.mutation.requestId; this proves Orca accepted the send. Do not require result.message.from_handle to equal dispatch:<id> in the immediate send receipt; the center verifies exact Dispatch identity when it consumes the inbox message. Plain text output is not a JSON receipt. Do not append --task-id, --dispatch-id, --outcome, --body, --phase, --files-modified, or --report-path to a status send with --payload; those separate structured flags conflict with --payload. Use --task-id/--dispatch-id/--outcome only on the later worker_done command.\n")
	b.WriteString("If this step needs a business answer, use the Dispatch preamble's Orca ask command with --json, the question, and any exact option strings; do not settle or start a replacement step while waiting. Parse and save the original ask message ID from that JSON response, and resume that same ID after a timeout. A plain ask without --json prints only the answer text and loses the ID needed for proof; do not issue a second question to recover it. When the answer returns and has actually been applied, send a separate type=status message through the same direct pwsh.exe CLI path with taskId, dispatchId, remote_codex_version, decision_applied set to the original question message ID, applied_decision_attempt_id and applied_decision_execution_id copied from Step Input, and applied_answer_body exactly equal to the answer received from Orca; omit step_result from this proof message. The center checks the echoed answer against its saved answer and binds the fixed answer digest. A bare question ID is never application proof. Continue the same step attempt, then send its one final step_result and worker_done. Orca reply acceptance alone does not prove you applied the answer.\n")
	fmt.Fprintf(&b, "Task title: %s\nTask description: %s\nWorkflow definition: %s\nStep contract: %s\nStep input: %s\n", task.Title, task.Description, workflow.Definition.ID, string(contractBytes), string(inputBytes))
	if b.Len() > 64<<10 {
		return "", &BackendError{Code: BackendRejected, Diagnostic: "serialized supervised step task exceeds the 64 KiB task-spec bound"}
	}
	return b.String(), nil
}

func (b orcaSupervisedBackend) inspectSupervisedWorktree(ctx context.Context, pair supervisedPreflight, worktreeID, observedPath, observedBranch, base, expectedHead string) (WorkspaceInspection, error) {
	if worktreeID == "" || !strings.HasPrefix(worktreeID, pair.RepositoryID+"::") || base == "" {
		return WorkspaceInspection{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "remote workspace inspection lacks exact worktree/repository/base identity"}
	}
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "worktree", "show", "--environment", pair.EnvironmentID, "--worktree", "id:"+worktreeID, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return WorkspaceInspection{}, &BackendError{Code: BackendUnreachable, Diagnostic: "remote worktree metadata could not be inspected", Cause: errors.Join(callErr, parseErr)}
	}
	if env.Meta.RuntimeID != pair.RemoteRuntime {
		return WorkspaceInspection{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "remote worktree metadata came from a different runtime epoch"}
	}
	var result struct {
		Worktree struct {
			ID     string `json:"id"`
			RepoID string `json:"repoId"`
			Path   string `json:"path"`
			Head   string `json:"head"`
			Branch string `json:"branch"`
		} `json:"worktree"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return WorkspaceInspection{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "remote worktree metadata is malformed", Cause: err}
	}
	branch := strings.TrimPrefix(result.Worktree.Branch, "refs/heads/")
	if result.Worktree.ID != worktreeID || result.Worktree.RepoID != pair.RepositoryID || result.Worktree.Path == "" || result.Worktree.Path != observedPath || branch == "" || branch != observedBranch || !commitHash.MatchString(result.Worktree.Head) || expectedHead != "" && result.Worktree.Head != expectedHead {
		return WorkspaceInspection{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "remote worktree metadata differs from the exact assigned repository/path/branch/revision"}
	}
	return WorkspaceInspection{Repository: b.config.identity, Worktree: result.Worktree.Path, Branch: branch, Base: base, Head: result.Worktree.Head, Dirty: false, CheckedAt: now()}, nil
}

func (b orcaSupervisedBackend) checkDelivery(ctx context.Context, identity ProcessIdentity) (supervisedDelivery, error) {
	sc := b.config.Orca.Supervised
	if sc == nil || identity.Kind != "orca_supervised" || identity.OrchestrationRun == "" || identity.CoordinatorRuntimeID == "" {
		return supervisedDelivery{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised inbox read lacks the fixed coordinator and Orca Run identity"}
	}
	waitMillis := duration(sc.PollInterval).Milliseconds()
	if waitMillis <= 0 {
		waitMillis = 2000
	}
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, time.Duration(waitMillis+5000)*time.Millisecond), "orchestration", "check", "--terminal", sc.CoordinatorTerminal, "--run", identity.OrchestrationRun, "--wait", "--types", "worker_done,status,question,escalation", "--timeout-ms", strconv.FormatInt(waitMillis, 10), "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return supervisedDelivery{}, &BackendError{Code: BackendUnreachable, Diagnostic: "supervised coordinator inbox read failed", Cause: errors.Join(callErr, parseErr)}
	}
	if env.Meta.RuntimeID != identity.CoordinatorRuntimeID {
		return supervisedDelivery{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised coordinator runtime changed; preserve the original Dispatch binding"}
	}
	var result struct {
		RunID          string             `json:"runId"`
		DeliveryID     string             `json:"deliveryId"`
		Messages       []OrcaInboxMessage `json:"messages"`
		TimedOut       bool               `json:"timedOut"`
		ConnectionLost bool               `json:"connectionLost"`
		Cancelled      bool               `json:"cancelled"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return supervisedDelivery{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "supervised inbox delivery is malformed", Cause: err}
	}
	if result.RunID != identity.OrchestrationRun {
		return supervisedDelivery{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised inbox returned a different Orca Run"}
	}
	if result.ConnectionLost {
		return supervisedDelivery{}, &BackendError{Code: BackendUnreachable, Diagnostic: "supervised inbox connection was lost"}
	}
	if result.Cancelled && ctx.Err() != nil {
		return supervisedDelivery{}, ctx.Err()
	}
	if len(result.Messages) > 0 && result.DeliveryID == "" {
		return supervisedDelivery{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "supervised inbox returned messages without a durable Delivery ID"}
	}
	return supervisedDelivery{ID: result.DeliveryID, Messages: result.Messages, TimedOut: result.TimedOut}, nil
}

func (b orcaSupervisedBackend) acknowledgeDelivery(ctx context.Context, identity ProcessIdentity, deliveryID string) (string, error) {
	if deliveryID == "" || identity.CoordinatorRuntimeID == "" || identity.OrchestrationRun == "" {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised Delivery acknowledgement lacks exact identity"}
	}
	sc := b.config.Orca.Supervised
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "check", "--terminal", sc.CoordinatorTerminal, "--run", identity.OrchestrationRun, "--ack", deliveryID, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised Delivery acknowledgement is uncertain; reconcile the exact Delivery ID", Cause: errors.Join(callErr, parseErr)}
	}
	if env.Meta.RuntimeID != identity.CoordinatorRuntimeID {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Delivery acknowledgement came from a different coordinator runtime"}
	}
	var result struct {
		Acknowledged string `json:"acknowledged"`
		Mutation     struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || result.Acknowledged != deliveryID {
		return "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca did not confirm acknowledgement of the exact Delivery", Cause: err}
	}
	return result.Mutation.RequestID, nil
}

func supervisedMessagePayload(message OrcaInboxMessage) ([]byte, error) {
	if len(message.Payload) == 0 || string(message.Payload) == "null" {
		return nil, nil
	}
	var text string
	if err := json.Unmarshal(message.Payload, &text); err == nil {
		return []byte(text), nil
	}
	return append([]byte(nil), message.Payload...), nil
}

func (b orcaSupervisedBackend) stepResultFromMessages(messages []OrcaInboxMessage, identity ProcessIdentity, attempt StepAttemptRecord, expected MinimalStepInput, resultPath string) (bool, error) {
	found := false
	for _, message := range messages {
		if message.Type != "status" {
			continue
		}
		payload, err := supervisedMessagePayload(message)
		if err != nil {
			return false, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "supervised status payload encoding is invalid", Cause: err}
		}
		if len(payload) == 0 {
			continue
		}
		if len(payload) > maxSupervisedMessageBytes {
			if message.FromHandle != "dispatch:"+identity.Dispatch {
				continue
			}
			return false, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "supervised structured status exceeds the fixed 8 KiB payload limit"}
		}
		var status struct {
			TaskID             string          `json:"taskId"`
			DispatchID         string          `json:"dispatchId"`
			RemoteCodexVersion string          `json:"remote_codex_version"`
			DecisionAppliedID  string          `json:"decision_applied"`
			StepResult         json.RawMessage `json:"step_result"`
		}
		if err = json.Unmarshal(payload, &status); err != nil {
			if message.FromHandle != "dispatch:"+identity.Dispatch {
				continue
			}
			return false, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "supervised status payload is not valid JSON", Cause: err}
		}
		if status.TaskID != identity.Task || status.DispatchID != identity.Dispatch {
			if message.FromHandle == "dispatch:"+identity.Dispatch {
				return false, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised status payload identity conflicts with its Dispatch sender"}
			}
			continue // A late status belongs to another attempt; it cannot complete this one.
		}
		if status.RemoteCodexVersion != b.config.Orca.Supervised.ExpectedCodexVersion {
			return false, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "remote Codex CLI version differs from the pinned capability configuration"}
		}
		if status.DecisionAppliedID != "" && len(status.StepResult) == 0 {
			continue // A separate status proves answer application, not step completion.
		}
		if len(status.StepResult) == 0 || len(status.StepResult) > maxSupervisedMessageBytes {
			return false, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "matching status message omitted a bounded step_result"}
		}
		var envelope MinimalStepResultEnvelope
		if err = decode(status.StepResult, &envelope); err != nil {
			return false, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "supervised step result envelope is invalid", Cause: err}
		}
		identityMismatches := []string{}
		if canonical(envelope.ProjectIdentity) != canonical(expected.ProjectIdentity) {
			identityMismatches = append(identityMismatches, fmt.Sprintf("project_identity (got %q, expected %q)", envelope.ProjectIdentity, expected.ProjectIdentity))
		}
		if envelope.TaskID != expected.TaskID {
			identityMismatches = append(identityMismatches, "task_id")
		}
		if envelope.WorkflowRunID != expected.WorkflowRunID {
			identityMismatches = append(identityMismatches, "workflow_run_id")
		}
		if envelope.ExecutionID != expected.ExecutionID {
			identityMismatches = append(identityMismatches, "execution_id")
		}
		if envelope.AttemptID != expected.AttemptID {
			identityMismatches = append(identityMismatches, "attempt_id")
		}
		if envelope.Result.StepID != attempt.StepID {
			identityMismatches = append(identityMismatches, "step_id")
		}
		if len(identityMismatches) > 0 {
			return false, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised step result identity mismatch: " + strings.Join(identityMismatches, ", ")}
		}
		encoded, err := json.Marshal(envelope)
		if err != nil {
			return false, err
		}
		if existing, readErr := os.ReadFile(resultPath); readErr == nil {
			if operationDigest(existing) != operationDigest(encoded) {
				return false, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "duplicate supervised result ID carried different result content"}
			}
		} else if !os.IsNotExist(readErr) {
			return false, readErr
		} else if err = AtomicWrite(resultPath, append(encoded, '\n')); err != nil {
			return false, err
		}
		found = true
	}
	return found, nil
}

type supervisedProcess struct {
	identity ProcessIdentity
	cancel   context.CancelFunc
	once     sync.Once
}

func (p *supervisedProcess) Identity() ProcessIdentity { return p.identity }
func (p *supervisedProcess) Kill()                     { p.Close() } // Stops only the local observer; remote cancel is unsupported.
func (p *supervisedProcess) Close()                    { p.once.Do(p.cancel) }

func (b orcaSupervisedBackend) observeWorker(ctx context.Context, worker *Worker, ref ExecutionRef, attempt StepAttemptRecord, input MinimalStepInput, resultPath string) {
	defer close(worker.Events)
	defer close(worker.observationDone)
	identity := ref.External
	finish := func(err error) {
		worker.Done <- err
	}
	outputCursor, outputSource := attempt.OutputCursor, attempt.OutputSource
	turnStartedObserved := attempt.Status == "running" || attempt.Status == "waiting_decision"
	hostWaitReported := false
	var settledWithoutMessageAt time.Time
	var identityUnverifiableAt time.Time
	for {
		if ctx.Err() != nil {
			finish(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised observer detached while remote worker may still be live", Cause: ctx.Err()})
			return
		}
		output, outputErr := b.ReadOutput(ctx, ref, outputCursor, 100)
		turnEvidence := outputErr == nil && !turnStartedObserved && supervisedTranscriptTurnStarted(output)
		if outputErr == nil && (output.NextCursor != outputCursor || output.Source != outputSource || output.Gap || output.SourceChanged || turnEvidence) {
			activity := supervisedOutputActivity(output, input.WorkflowRunID, input.TaskID)
			event := WorkerEvent{RunID: input.WorkflowRunID, AttemptID: attempt.AttemptID, Summary: activity.Title, Activity: activity, RemoteRunID: identity.OrchestrationRun, OutputSource: output.Source, OutputCursor: output.NextCursor, OutputGap: output.Gap || output.SourceChanged, TurnStarted: turnEvidence, Processed: make(chan error, 1)}
			if activity.Kind == "" {
				event.Activity = ActivityEvent{}
			}
			select {
			case worker.Events <- event:
			case <-ctx.Done():
				finish(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised output arrived while observation was detached", Cause: ctx.Err()})
				return
			}
			select {
			case persistErr := <-event.Processed:
				if persistErr != nil {
					finish(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised output was read but its cursor was not durably saved", Cause: persistErr})
					return
				}
				outputCursor, outputSource = output.NextCursor, output.Source
				turnStartedObserved = turnStartedObserved || turnEvidence
			case <-ctx.Done():
				finish(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised output cursor persistence was interrupted", Cause: ctx.Err()})
				return
			}
		} else if outputErr != nil {
			var outputBackendErr *BackendError
			if errors.As(outputErr, &outputBackendErr) && (outputBackendErr.Code == BackendIdentityMismatch || outputBackendErr.Code == BackendProtocolInvalid) {
				finish(outputErr)
				return
			}
		}
		delivery, err := b.checkDelivery(ctx, identity)
		if err != nil {
			if ctx.Err() != nil {
				finish(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised observer stopped without remote settlement proof", Cause: ctx.Err()})
				return
			}
			var backendErr *BackendError
			if errors.As(err, &backendErr) && backendErr.Code != BackendUnreachable {
				finish(err)
				return
			}
			// A missed inbox poll does not prove the worker stopped. Keep the slot
			// occupied and retry the same exact coordinator/Run identity.
			if sleepErr := waitContext(ctx, duration(b.config.Orca.Supervised.PollInterval)); sleepErr != nil {
				continue
			}
			continue
		}
		if len(delivery.Messages) > 0 {
			resultFound, resultErr := b.stepResultFromMessages(delivery.Messages, identity, attempt, input, resultPath)
			var doneOutcome string
			var doneBody string
			unhandledEscalation := false
			for _, message := range delivery.Messages {
				if message.Type == "escalation" && supervisedMessageMatches(message, identity) {
					unhandledEscalation = true
				}
				if message.Type != "worker_done" {
					continue
				}
				payload, payloadErr := supervisedMessagePayload(message)
				if payloadErr != nil {
					if message.FromHandle == "dispatch:"+identity.Dispatch {
						resultErr = &BackendError{Code: BackendProtocolInvalid, Diagnostic: "worker_done payload encoding is invalid", Cause: payloadErr}
					}
					continue
				}
				var done struct {
					TaskID     string `json:"taskId"`
					DispatchID string `json:"dispatchId"`
					Outcome    string `json:"outcome"`
				}
				if err = json.Unmarshal(payload, &done); err != nil {
					if message.FromHandle == "dispatch:"+identity.Dispatch {
						resultErr = &BackendError{Code: BackendProtocolInvalid, Diagnostic: "worker_done payload is not valid JSON", Cause: err}
					}
					continue
				}
				if done.TaskID != identity.Task || done.DispatchID != identity.Dispatch {
					if message.FromHandle == "dispatch:"+identity.Dispatch {
						resultErr = &BackendError{Code: BackendIdentityMismatch, Diagnostic: "worker_done payload identity conflicts with its Dispatch sender"}
					}
					continue
				}
				doneOutcome, doneBody = done.Outcome, message.Body
			}
			activity := ActivityEvent{V: ActivityVersion, TS: now(), Source: "orca-supervised", RunID: attemptResultRunID(input), ItemID: delivery.ID, Kind: "message", Title: "Orca inbox Delivery", Detail: summarizeSupervisedMessages(delivery.Messages)}
			event := WorkerEvent{RunID: attemptResultRunID(input), AttemptID: attempt.AttemptID, Summary: activity.Title, Activity: activity, RemoteRunID: identity.OrchestrationRun, RemoteDeliveryID: delivery.ID, RemoteMessages: delivery.Messages, Acknowledge: func() (string, error) { return b.acknowledgeDelivery(ctx, identity, delivery.ID) }, Processed: make(chan error, 1)}
			select {
			case worker.Events <- event:
			case <-ctx.Done():
				finish(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised Delivery arrived while observation was detached", Cause: ctx.Err()})
				return
			}
			select {
			case ackErr := <-event.Processed:
				if ackErr != nil {
					finish(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised Delivery was persisted but acknowledgement is uncertain", Cause: ackErr})
					return
				}
			case <-ctx.Done():
				finish(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised Delivery acknowledgement was interrupted", Cause: ctx.Err()})
				return
			}
			if resultErr != nil {
				finish(resultErr)
				return
			}
			if unhandledEscalation {
				finish(&BackendError{Code: BackendUnsupported, Diagnostic: "supervised escalation was persisted but requires explicit human handling"})
				return
			}
			if doneOutcome != "" {
				worker.RemoteOutcome = doneOutcome
				if doneOutcome != "succeeded" {
					finish(&BackendError{Code: BackendRejected, Diagnostic: "supervised worker_done reported failure: " + clip(doneBody, 1000)})
					return
				}
				if !resultFound {
					if _, statErr := os.Stat(resultPath); statErr != nil {
						finish(&BackendError{Code: BackendProtocolInvalid, Diagnostic: "successful worker_done has no matching structured step result", Cause: statErr})
						return
					}
				}
				finish(nil)
				return
			}
		}
		if delivery.TimedOut {
			expected := supervisedStartReceipt{RunID: identity.OrchestrationRun, TaskID: identity.Task, DispatchID: identity.Dispatch, WorktreeID: identity.WorktreeID, CenterRuntimeID: identity.CoordinatorRuntimeID}
			observation, inspectErr := b.showWorker(ctx, expected)
			if inspectErr != nil {
				var backendErr *BackendError
				if errors.As(inspectErr, &backendErr) && (backendErr.Code == BackendIdentityMismatch || backendErr.Code == BackendProtocolInvalid) {
					// Orca may briefly expose an incomplete projection while a
					// Dispatch settles. Keep the original binding and only re-read;
					// never substitute a terminal or create another worker.
					if identityUnverifiableAt.IsZero() {
						identityUnverifiableAt = now()
					}
					if now().Sub(identityUnverifiableAt) >= time.Minute {
						finish(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: "original supervised Dispatch identity remained unverifiable after bounded read-only reconciliation", Cause: inspectErr})
						return
					}
					continue
				}
				if errors.As(inspectErr, &backendErr) && backendErr.Code != BackendUnreachable {
					finish(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: "original supervised Dispatch identity became unverifiable", Cause: inspectErr})
					return
				}
				continue
			}
			identityUnverifiableAt = time.Time{}
			if observation.AgentWaitState == "present" && !hostWaitReported || observation.AgentWaitState == "none" && hostWaitReported {
				pending := observation.AgentWaitState == "present"
				activity := ActivityEvent{V: ActivityVersion, TS: now(), Source: "orca-supervised:worker-show", RunID: input.WorkflowRunID, TaskID: input.TaskID, ItemID: "host-wait:" + identity.Dispatch, Kind: "error", Status: "manual_host_check", Title: "Remote host interaction needs manual handling", Detail: "Orca reported agentWait for the exact worker. Inspect the execution host; SteerLine will not answer this as a business question.", Meta: map[string]any{"dispatch_id": identity.Dispatch, "agent_wait": "present"}}
				if !pending {
					activity.Kind, activity.Status, activity.Title = "message", "completed", "Remote host interaction cleared"
					activity.Detail = "Orca now reports agentWait=null for the exact worker. Continue observing the original Dispatch; this alone does not prove the business step completed."
					activity.Meta["agent_wait"] = "none"
				}
				event := WorkerEvent{RunID: input.WorkflowRunID, AttemptID: attempt.AttemptID, Summary: activity.Title, Activity: activity, RemoteRunID: identity.OrchestrationRun, Processed: make(chan error, 1)}
				select {
				case worker.Events <- event:
				case <-ctx.Done():
					finish(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised host wait was observed while the center detached", Cause: ctx.Err()})
					return
				}
				select {
				case persistErr := <-event.Processed:
					if persistErr != nil {
						finish(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised host wait could not be persisted", Cause: persistErr})
						return
					}
					hostWaitReported = pending
				case <-ctx.Done():
					finish(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised host wait persistence was interrupted", Cause: ctx.Err()})
					return
				}
			}
			if observation.Liveness == "exited" || observation.DispatchState == "completed" || observation.DispatchState == "failed" {
				// Worker settlement and inbox relay are separate observations. A
				// completed Dispatch can become visible before its final Delivery.
				if settledWithoutMessageAt.IsZero() {
					settledWithoutMessageAt = now()
				}
				if now().Sub(settledWithoutMessageAt) >= time.Minute {
					finish(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised worker settled but no matching worker_done appeared in the coordinator inbox after a bounded reconciliation window"})
					return
				}
			} else {
				settledWithoutMessageAt = time.Time{}
			}
			continue
		}
	}
}

func supervisedMessageMatches(message OrcaInboxMessage, identity ProcessIdentity) bool {
	payload, err := supervisedMessagePayload(message)
	if err == nil && len(payload) > 0 {
		var fields struct {
			TaskID     string `json:"taskId"`
			DispatchID string `json:"dispatchId"`
		}
		if json.Unmarshal(payload, &fields) == nil && fields.TaskID == identity.Task && fields.DispatchID == identity.Dispatch {
			return true
		}
	}
	return message.FromHandle == "dispatch:"+identity.Dispatch
}

func attemptResultRunID(input MinimalStepInput) string { return input.WorkflowRunID }

func summarizeSupervisedMessages(messages []OrcaInboxMessage) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		value := message.Type
		if message.Subject != "" {
			value += ": " + message.Subject
		}
		if message.Body != "" {
			value += " — " + message.Body
		}
		parts = append(parts, clip(value, activityDetailMax))
	}
	return clip(strings.Join(parts, "\n"), activityDetailMax)
}

func supervisedTranscriptTurnStarted(batch OutputBatch) bool {
	if batch.Source != "transcript" {
		return false
	}
	var result struct {
		Transcript struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		} `json:"transcript"`
	}
	if json.Unmarshal(batch.Data, &result) != nil {
		return false
	}
	for _, message := range result.Transcript.Messages {
		if message.Role == "assistant" {
			return true
		}
	}
	return false
}

func supervisedOutputActivity(batch OutputBatch, runID, taskID string) ActivityEvent {
	var result struct {
		Transcript struct {
			Messages []struct {
				Role   string `json:"role"`
				Blocks []struct {
					Type   string `json:"type"`
					Text   string `json:"text"`
					Output string `json:"output"`
				} `json:"blocks"`
			} `json:"messages"`
		} `json:"transcript"`
		Terminal struct {
			Tail []string `json:"tail"`
		} `json:"terminal"`
	}
	_ = json.Unmarshal(batch.Data, &result)
	parts := make([]string, 0)
	for _, message := range result.Transcript.Messages {
		if message.Role != "assistant" && message.Role != "tool" {
			continue
		}
		for _, block := range message.Blocks {
			value := block.Text
			if value == "" {
				value = block.Output
			}
			if strings.TrimSpace(value) != "" {
				parts = append(parts, strings.TrimSpace(value))
			}
		}
	}
	if len(parts) == 0 && len(result.Terminal.Tail) > 0 {
		parts = append(parts, strings.Join(result.Terminal.Tail, "\n"))
	}
	joined := strings.Join(parts, "\n")
	kind, status, title := "message", "", "Remote worker output"
	detail := clip(joined, activityDetailMax)
	if strings.Contains(joined, "orchestrator_helper_launch_canceled") || strings.Contains(joined, "ShellExecuteExW failed to launch setup helper: 1223") {
		kind, status, title = "error", "manual_host_check", "Remote host command launch needs manual verification"
		detail = "The execution host rejected a worker command before it ran. Inspect host permissions and the exact worker; SteerLine will not treat this as a business question or retry the command automatically."
	}
	if batch.Gap || batch.SourceChanged {
		kind, title = "error", "Remote output gap"
		if status == "manual_host_check" {
			detail = "Orca reported an output gap and a host command launch failure. Inspect both the retained output and host permissions manually."
		} else if detail == "" {
			detail = "Orca reported truncated history or a changed output source; inspect output metadata."
		}
	}
	itemID := ""
	if batch.NextCursor != "" {
		itemID = operationDigest([]byte(batch.Source + "\x00" + batch.NextCursor))[:24]
	}
	return ActivityEvent{V: ActivityVersion, TS: batch.ObservedAt, Source: "orca-supervised:" + batch.Source, RunID: runID, TaskID: taskID, ItemID: itemID, Kind: kind, Status: status, Title: title, Detail: detail, Meta: map[string]any{"source": batch.Source, "cursor": batch.NextCursor, "gap": batch.Gap, "source_changed": batch.SourceChanged}}
}

func waitContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		duration = 2 * time.Second
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type orcaSupervisedBackend struct{ config Config }

func (b orcaSupervisedBackend) Identity() (BackendKind, ExecutionProtocol) {
	return BackendOrca, ProtocolOrcaSupervised
}

func (b orcaSupervisedBackend) Probe(ctx context.Context, target ExecutionTarget, agent AgentSpec) (CapabilityReport, error) {
	fixedTarget := targetFor(b.config)
	if agent.Name != "codex" || !sameSupervisedBaseTarget(target, fixedTarget) {
		return CapabilityReport{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised Orca probe target or agent differs from fixed configuration"}
	}
	target = fixedTarget
	pair, err := probeSupervisedOrcaTarget(ctx, b.config)
	if err != nil {
		return CapabilityReport{}, &BackendError{Code: BackendUnreachable, Diagnostic: "supervised Orca target preflight failed", Cause: err}
	}
	report := capabilityReport(BackendOrca, ProtocolOrcaSupervised, target, agent, pair.RemoteVersion)
	report.CenterVersion = pair.CenterVersion
	report.ExpectedAgentVersion = b.config.Orca.Supervised.ExpectedCodexVersion
	report.Capabilities = append(report.Capabilities,
		Capability{Name: "message_inbox", Status: CapabilityNotTested},
		Capability{Name: "remote_cancellation", Status: CapabilityUnsupported, Evidence: "Spec 03 fixes remote_cancel_available=false"},
	)
	for i := range report.Capabilities {
		if report.Capabilities[i].Name == "stop" || report.Capabilities[i].Name == "retry" {
			report.Capabilities[i].Status = CapabilityUnsupported
		}
		if report.Capabilities[i].Name == "resource_settlement" {
			report.Capabilities[i].Status = CapabilityNotTested
		}
	}
	return report, nil
}

func (orcaSupervisedBackend) Start(context.Context, AttemptSpec) (StartReceipt, error) {
	return StartReceipt{}, unsupported("supervised Orca worker start adapter")
}
func (b orcaSupervisedBackend) Observe(ctx context.Context, ref ExecutionRef) (Observation, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return Observation{}, err
	}
	show, err := b.showWorker(ctx, supervisedStartReceipt{RunID: ref.External.OrchestrationRun, TaskID: ref.External.Task, DispatchID: ref.External.Dispatch, WorktreeID: ref.External.WorktreeID, CenterRuntimeID: ref.External.CoordinatorRuntimeID})
	if err != nil {
		return Observation{}, err
	}
	if show.Identity != ref.External {
		return Observation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised worker observation returned a different exact execution identity"}
	}
	connection := "connected"
	if show.Liveness == "unverifiable" {
		connection = "unverifiable"
	}
	diagnostic := show.DispatchState
	if show.AgentWaitState == "present" {
		diagnostic = "host_interaction_pending"
	}
	return Observation{Liveness: show.Liveness, Connection: connection, ObservedAt: show.ObservedAt, Source: "orca-supervised-worker-show", Diagnostic: diagnostic}, nil
}
func (b orcaSupervisedBackend) InspectWorkspace(ctx context.Context, ref ExecutionRef) (WorkspaceInspection, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return WorkspaceInspection{}, err
	}
	pair, err := probeSupervisedOrcaTarget(ctx, b.config)
	if err != nil {
		return WorkspaceInspection{}, &BackendError{Code: BackendUnreachable, Diagnostic: "supervised workspace preflight failed", Cause: err}
	}
	return b.inspectSupervisedWorktree(ctx, pair, ref.External.WorktreeID, ref.WorkspacePath, ref.WorkspaceBranch, ref.BaseCommit, "")
}
func (b orcaSupervisedBackend) ReadOutput(ctx context.Context, ref ExecutionRef, cursor string, limit int) (OutputBatch, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return OutputBatch{}, err
	}
	if limit <= 0 || limit > 1000 {
		return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "supervised output limit must be 1–1000 rows"}
	}
	args := []string{"orchestration", "worker-read", "--dispatch", ref.External.Dispatch, "--source", "auto", "--limit", strconv.Itoa(limit), "--json"}
	if cursor != "" {
		args = append(args, "--cursor", cursor)
	}
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), args...)
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return OutputBatch{}, &BackendError{Code: BackendUnreachable, Diagnostic: "supervised worker output read failed", Cause: errors.Join(callErr, parseErr)}
	}
	if env.Meta.RuntimeID != ref.External.CoordinatorRuntimeID {
		return OutputBatch{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "worker output came from a different coordinator runtime"}
	}
	var result struct {
		DispatchID      string          `json:"dispatchId"`
		Source          string          `json:"source"`
		Cursor          string          `json:"cursor"`
		NextCursor      string          `json:"nextCursor"`
		SourceChanged   bool            `json:"sourceChanged"`
		ContentComplete bool            `json:"contentComplete"`
		Clipping        []string        `json:"clipping"`
		FallbackReason  string          `json:"fallbackReason"`
		Transcript      json.RawMessage `json:"transcript"`
		Terminal        json.RawMessage `json:"terminal"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "supervised worker output response is malformed", Cause: err}
	}
	if result.DispatchID != ref.External.Dispatch || result.Source == "" {
		return OutputBatch{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised worker output source does not match the fixed Dispatch"}
	}
	next := result.Cursor
	if result.NextCursor != "" {
		next = result.NextCursor
	}
	if next == "" {
		return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "supervised worker output omitted its source cursor"}
	}
	if len(env.Result) > 1<<20 {
		return OutputBatch{}, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "supervised worker output exceeded the 1 MiB response bound"}
	}
	return OutputBatch{Data: append([]byte(nil), env.Result...), Source: result.Source, ObservedAt: now(), NextCursor: next, Gap: len(result.Clipping) > 0, SourceChanged: result.SourceChanged || result.FallbackReason == "source_changed"}, nil
}
func (orcaSupervisedBackend) Reply(context.Context, ExecutionRef, string, Operation, string) (ReplyReceipt, error) {
	return ReplyReceipt{}, unsupported("supervised Orca worker reply adapter")
}

func (b orcaSupervisedBackend) replyQuestion(ctx context.Context, ref ExecutionRef, questionID, answer string) (string, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return "", err
	}
	if questionID == "" || strings.TrimSpace(answer) == "" || len([]byte(answer)) > maxSupervisedMessageBytes {
		return "", &BackendError{Code: BackendRejected, Diagnostic: "supervised reply requires a bounded answer and original question ID"}
	}
	observation, err := b.Observe(ctx, ref)
	if err != nil {
		return "", err
	}
	if observation.Liveness != "live" {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised reply requires the original live Dispatch"}
	}
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "reply", "--id", questionID, "--body", answer, "--run", ref.External.OrchestrationRun, "--from", b.config.Orca.Supervised.CoordinatorTerminal, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised reply outcome is uncertain; reconcile the exact question before retry", Cause: errors.Join(callErr, parseErr)}
	}
	if env.Meta.RuntimeID != ref.External.CoordinatorRuntimeID {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised reply receipt came from a different coordinator runtime"}
	}
	var result struct {
		Message struct {
			FromHandle string `json:"from_handle"`
			ToHandle   string `json:"to_handle"`
		} `json:"message"`
		Question struct {
			MessageID  string `json:"message_id"`
			RunID      string `json:"run_id"`
			DispatchID string `json:"dispatch_id"`
			Status     string `json:"status"`
			AnswerBody string `json:"answer_body"`
		} `json:"question"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || result.Question.MessageID != questionID || result.Question.RunID != ref.External.OrchestrationRun || result.Question.DispatchID != ref.External.Dispatch || result.Question.AnswerBody != answer || result.Question.Status != "answered" || result.Message.FromHandle != "run:"+ref.External.OrchestrationRun || result.Message.ToHandle != "dispatch:"+ref.External.Dispatch || result.Mutation.RequestID == "" {
		return "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca did not confirm the exact question answer and request ID", Cause: err}
	}
	return result.Mutation.RequestID, nil
}

// findAnsweredQuestion reads Orca's durable inbox history by the original
// question thread. It is used only after an uncertain reply receipt, never to
// guess a new Dispatch or to resend an answer.
func (b orcaSupervisedBackend) findAnsweredQuestion(ctx context.Context, ref ExecutionRef, questionID, answer string) (string, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return "", err
	}
	if questionID == "" || answer == "" {
		return "", &BackendError{Code: BackendRejected, Diagnostic: "question reconciliation requires the original ID and answer"}
	}
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "inbox", "--limit", "1000", "--full", "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return "", &BackendError{Code: BackendUnreachable, Diagnostic: "read-only question reconciliation is unavailable", Cause: errors.Join(callErr, parseErr)}
	}
	if env.Meta.RuntimeID != ref.External.CoordinatorRuntimeID {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "question history came from a different coordinator runtime"}
	}
	if len(env.Result) > 2<<20 {
		return "", &BackendError{Code: BackendProtocolInvalid, Diagnostic: "question history exceeded the bounded response size"}
	}
	var result struct {
		Messages []struct {
			ID         string `json:"id"`
			FromHandle string `json:"from_handle"`
			ToHandle   string `json:"to_handle"`
			ThreadID   string `json:"thread_id"`
			Body       string `json:"body"`
			Type       string `json:"type"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return "", &BackendError{Code: BackendProtocolInvalid, Diagnostic: "question history is malformed", Cause: err}
	}
	found := ""
	for _, message := range result.Messages {
		if message.ThreadID != questionID || message.FromHandle != "run:"+ref.External.OrchestrationRun || message.ToHandle != "dispatch:"+ref.External.Dispatch || message.Type != "status" {
			continue
		}
		if message.Body != answer || message.ID == "" || found != "" {
			return "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "original question has conflicting or duplicate answer history"}
		}
		found = message.ID
	}
	if found == "" {
		return "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "original question answer is not yet uniquely visible in read-only inbox history"}
	}
	return found, nil
}
func (orcaSupervisedBackend) StopExecution(context.Context, ExecutionRef, Operation, *Worker) (StopReceipt, error) {
	return StopReceipt{}, unsupported("remote cancellation is disabled for supervised Orca")
}
func (orcaSupervisedBackend) DetachObservation(_ context.Context, worker *Worker) error {
	if worker != nil {
		worker.Close()
	}
	return nil
}
func (b orcaSupervisedBackend) SettleResources(ctx context.Context, ref ExecutionRef, outcome string) (SettlementReceipt, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return SettlementReceipt{}, err
	}
	if outcome != "succeeded" && outcome != "failed" {
		return SettlementReceipt{}, &BackendError{Code: BackendRejected, Diagnostic: "supervised resource release requires an accepted worker_done outcome"}
	}
	show, err := b.showWorker(ctx, supervisedStartReceipt{RunID: ref.External.OrchestrationRun, TaskID: ref.External.Task, DispatchID: ref.External.Dispatch, WorktreeID: ref.External.WorktreeID, CenterRuntimeID: ref.External.CoordinatorRuntimeID})
	if err != nil {
		return SettlementReceipt{}, err
	}
	if show.DispatchState != "completed" && show.DispatchState != "failed" {
		return SettlementReceipt{ResidualResources: []string{"dispatch:" + ref.External.Dispatch}}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised worker is not settled; release refused"}
	}
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "worker-release", "--dispatch", ref.External.Dispatch, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return SettlementReceipt{Disposition: "outcome_unknown", ResidualResources: []string{"dispatch:" + ref.External.Dispatch}}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "settled worker release outcome is uncertain; reconcile the exact Dispatch", Cause: errors.Join(callErr, parseErr)}
	}
	if env.Meta.RuntimeID != ref.External.CoordinatorRuntimeID {
		return SettlementReceipt{Disposition: "outcome_unknown", ResidualResources: []string{"dispatch:" + ref.External.Dispatch}}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "worker release receipt came from a different coordinator runtime"}
	}
	var result struct {
		DispatchID string   `json:"dispatchId"`
		State      string   `json:"state"`
		Residual   []string `json:"residualResources"`
		Mutation   struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || result.DispatchID != ref.External.Dispatch {
		return SettlementReceipt{Disposition: "outcome_unknown", ResidualResources: []string{"dispatch:" + ref.External.Dispatch}}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "worker release receipt does not identify the exact Dispatch", Cause: err}
	}
	if result.State != "released" && result.State != "already_released" {
		if len(result.Residual) > 0 {
			return SettlementReceipt{Disposition: "residual", PlatformRequestID: result.Mutation.RequestID, ResidualResources: result.Residual}, nil
		}
		return SettlementReceipt{Disposition: "residual", PlatformRequestID: result.Mutation.RequestID, ResidualResources: []string{"dispatch:" + ref.External.Dispatch}}, nil
	}
	return SettlementReceipt{Disposition: "released", PlatformRequestID: result.Mutation.RequestID}, nil
}

func validateSupervisedRef(c Config, ref ExecutionRef) error {
	if ref.Backend != BackendOrca || ref.Protocol != ProtocolOrcaSupervised || !sameSupervisedBaseTarget(ref.Target, targetFor(c)) || ref.Target.OrchestrationRun != ref.External.OrchestrationRun || ref.Target.ExternalTask != ref.External.Task || ref.Target.Dispatch != ref.External.Dispatch || ref.Target.ExecutionHost != ref.External.ExecutionHost || ref.Target.WorktreeID != ref.External.WorktreeID || ref.External.Kind != "orca_supervised" || ref.External.RuntimeID == "" || ref.External.CoordinatorRuntimeID == "" || ref.External.OrchestrationRun == "" || ref.External.Task == "" || ref.External.Dispatch == "" || ref.External.ExecutionHost == "" || ref.External.WorktreeID == "" {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised Orca reference does not match the exact configured target and Dispatch identity"}
	}
	return nil
}

func sameSupervisedBaseTarget(a, b ExecutionTarget) bool {
	return a.ID != "" && a.ID == b.ID && a.ConnectionID == b.ConnectionID && a.ExternalRuntime == b.ExternalRuntime
}
func (b orcaSupervisedBackend) ReconcileOperation(ctx context.Context, op Operation) (Operation, error) {
	return reconcileRecordedOperation(ctx, b.config.RunDir, op)
}
