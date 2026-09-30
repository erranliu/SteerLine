package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

func (b orcaSupervisedBackend) createOrchestrationRun(ctx context.Context, workflowRunID, objective string) (string, string, string, error) {
	if !stableExecutionID(workflowRunID) || strings.TrimSpace(objective) == "" {
		return "", "", "", &BackendError{Code: BackendRejected, Diagnostic: "supervised Orca Run requires a fixed workflow identity and objective"}
	}
	if err := validateSupervisedOrcaConfig(&b.config); err != nil {
		return "", "", "", &BackendError{Code: BackendRejected, Diagnostic: "supervised Orca configuration is invalid", Cause: err}
	}
	pair, err := probeSupervisedOrcaTarget(ctx, b.config)
	if err != nil {
		return "", "", "", &BackendError{Code: BackendUnreachable, Diagnostic: "supervised Orca preflight failed before Run creation", Cause: err}
	}
	coordinator, err := b.createRunCoordinator(ctx, workflowRunID, pair.CenterRuntime)
	if err != nil {
		return "", "", "", err
	}
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "run-create", "--objective", objective, "--from", coordinator, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		if runID, reconcileErr := b.currentRunForObjective(ctx, objective, pair.CenterRuntime, coordinator); reconcileErr == nil {
			return runID, "", coordinator, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca Run was found by its fixed objective, but its platform request UUID is unavailable"}
		}
		return "", "", coordinator, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised Orca Run creation may have been applied; no platform request UUID was received and exact reconciliation failed", Cause: errors.Join(callErr, parseErr)}
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
		if runID, reconcileErr := b.currentRunForObjective(ctx, objective, pair.CenterRuntime, coordinator); reconcileErr == nil {
			return runID, "", coordinator, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca Run was found by its fixed objective, but its platform request UUID is unavailable"}
		}
		return "", "", coordinator, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca Run creation receipt lacks its stable Run ID", Cause: err}
	}
	if env.Meta.RuntimeID != pair.CenterRuntime {
		return "", "", coordinator, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca Run creation receipt came from a different center runtime"}
	}
	if result.Mutation.RequestID == "" {
		return result.Run.ID, "", coordinator, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "Orca Run was created but its platform request UUID was omitted; persist the exact Run and reconcile before another mutation"}
	}
	return result.Run.ID, result.Mutation.RequestID, coordinator, nil
}

func (b orcaSupervisedBackend) currentRunForObjective(ctx context.Context, objective, centerRuntime, coordinator string) (string, error) {
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "run-current", "--from", coordinator, "--json")
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

// The configured terminal is only a bootstrap/preflight surface. Orca binds a
// coordinator terminal to one Run, so concurrent WorkflowRuns own distinct
// terminals on the center host.
func (b orcaSupervisedBackend) createRunCoordinator(ctx context.Context, workflowRunID, centerRuntime string) (string, error) {
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "terminal", "create", "--worktree", "path:"+b.config.Repository, "--shell", "powershell", "--title", "SteerLine coordinator "+workflowRunID, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "per-Run coordinator creation is uncertain; inspect the original WorkflowRun before retry", Cause: errors.Join(callErr, parseErr)}
	}
	var result struct {
		Terminal struct {
			Handle string `json:"handle"`
		} `json:"terminal"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || env.Meta.RuntimeID != centerRuntime || result.Terminal.Handle == "" {
		return "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "per-Run coordinator receipt lacks an exact handle or center runtime", Cause: err}
	}
	return result.Terminal.Handle, nil
}

func (b orcaSupervisedBackend) verifiedRunCoordinator(ctx context.Context, ref ExecutionRef) (string, error) {
	coordinator := ref.Target.CoordinatorTerminal
	if coordinator == "" && b.config.Orca.Supervised != nil {
		coordinator = b.config.Orca.Supervised.CoordinatorTerminal
	}
	if !stableExecutionID(ref.RunID) || ref.External.OrchestrationRun == "" || ref.External.CoordinatorRuntimeID == "" || coordinator == "" {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised Run lacks its pinned coordinator binding"}
	}
	client := orcaClient{cfg: b.config}
	out, callErr := client.call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "run-show", "--id", ref.External.OrchestrationRun, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return "", &BackendError{Code: BackendUnreachable, Diagnostic: "original Orca Run cannot be inspected", Cause: errors.Join(callErr, parseErr)}
	}
	var result struct {
		Run struct {
			ID          string `json:"id"`
			Coordinator string `json:"coordinator_handle"`
		} `json:"run"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || env.Meta.RuntimeID != ref.External.CoordinatorRuntimeID || result.Run.ID != ref.External.OrchestrationRun || result.Run.Coordinator != coordinator {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca Run is bound to another coordinator", Cause: err}
	}
	return coordinator, nil
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
	// Orca may spend its full 60-second worker setup window creating the
	// remote worktree before returning the mutation receipt. Keep the CLI
	// alive long enough to receive that identity on a healthy connection.
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, max(positiveDuration(b.config.CommandTimeout, 30*time.Second), 2*time.Minute), args...)
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return supervisedStartReceipt{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "supervised worker start may have been applied; inspect the exact Run and do not issue another start without a platform request UUID", Cause: errors.Join(callErr, parseErr)}
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

// A settled Dispatch no longer has to expose its former live terminal in
// worker-show. Check its durable identities before recovering its inbox or
// releasing its resources; the original terminal identity remains in ref.
func (b orcaSupervisedBackend) settledWorker(ctx context.Context, ref ExecutionRef, outcome string) (string, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return "", err
	}
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "worker-show", "--dispatch", ref.External.Dispatch, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return "", &BackendError{Code: BackendUnreachable, Diagnostic: "settled supervised Dispatch inspection failed", Cause: errors.Join(callErr, parseErr)}
	}
	var result struct {
		Dispatch struct {
			ID, RunID, TaskID, Status string
		} `json:"dispatch"`
		Worker struct {
			DispatchID, WorktreeID, AgentTerminalHandle, State string
		} `json:"worker"`
		Projection struct {
			Host struct {
				ID string `json:"id"`
			} `json:"host"`
		} `json:"projection"`
		Server struct {
			EnvironmentID string `json:"environmentId"`
		} `json:"server"`
		RemoteRuntimeEpoch string `json:"remoteRuntimeEpoch"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return "", &BackendError{Code: BackendProtocolInvalid, Diagnostic: "settled supervised worker-show response is malformed", Cause: err}
	}
	if env.Meta.RuntimeID != ref.External.CoordinatorRuntimeID || result.Dispatch.ID != ref.External.Dispatch || result.Dispatch.RunID != ref.External.OrchestrationRun || result.Dispatch.TaskID != ref.External.Task || result.Worker.DispatchID != ref.External.Dispatch || result.Worker.WorktreeID != ref.External.WorktreeID || result.Worker.AgentTerminalHandle != ref.External.Terminal || result.Server.EnvironmentID != ref.External.ExecutionHost || result.Projection.Host.ID != ref.External.ExecutionHost || result.RemoteRuntimeEpoch != ref.External.RuntimeID {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "settled Dispatch differs from the original Run, worker, host, or terminal binding"}
	}
	state := ""
	switch {
	case result.Dispatch.Status == "completed" && result.Worker.State == "succeeded":
		state = "succeeded"
	case result.Dispatch.Status == "failed" && result.Worker.State == "failed":
		state = "failed"
	default:
		return "", &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "original supervised Dispatch has not settled conclusively"}
	}
	if outcome != "" && state != outcome {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "settled Dispatch outcome differs from accepted worker_done"}
	}
	return state, nil
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

func (b orcaSupervisedBackend) checkDelivery(ctx context.Context, ref ExecutionRef) (supervisedDelivery, error) {
	identity := ref.External
	sc := b.config.Orca.Supervised
	if sc == nil || identity.Kind != "orca_supervised" || identity.OrchestrationRun == "" || identity.CoordinatorRuntimeID == "" {
		return supervisedDelivery{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised inbox read lacks the fixed coordinator and Orca Run identity"}
	}
	waitMillis := duration(sc.PollInterval).Milliseconds()
	if waitMillis <= 0 {
		waitMillis = 2000
	}
	coordinator, err := b.verifiedRunCoordinator(ctx, ref)
	if err != nil {
		return supervisedDelivery{}, err
	}
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, time.Duration(waitMillis+5000)*time.Millisecond), "orchestration", "check", "--terminal", coordinator, "--run", identity.OrchestrationRun, "--wait", "--types", "worker_done,status,question,escalation", "--timeout-ms", strconv.FormatInt(waitMillis, 10), "--json")
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

func (b orcaSupervisedBackend) acknowledgeDelivery(ctx context.Context, ref ExecutionRef, deliveryID string) (string, error) {
	identity := ref.External
	if deliveryID == "" || identity.CoordinatorRuntimeID == "" || identity.OrchestrationRun == "" {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised Delivery acknowledgement lacks exact identity"}
	}
	coordinator, err := b.verifiedRunCoordinator(ctx, ref)
	if err != nil {
		return "", err
	}
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "check", "--terminal", coordinator, "--run", identity.OrchestrationRun, "--ack", deliveryID, "--json")
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

type orcaSupervisedBackend struct{ config Config }

func supervisedCodexVersionAccepted(expected, observed string) bool {
	expected = strings.TrimSpace(expected)
	observed = strings.TrimSpace(observed)
	return observed != "" && (expected == "" || observed == expected)
}

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
func (b orcaSupervisedBackend) replyQuestion(ctx context.Context, ref ExecutionRef, questionID, answer string) (string, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return "", err
	}
	if questionID == "" || strings.TrimSpace(answer) == "" {
		return "", &BackendError{Code: BackendRejected, Diagnostic: "supervised reply requires a bounded answer and original question ID"}
	}
	coordinator := ref.Target.CoordinatorTerminal
	if coordinator == "" && b.config.Orca.Supervised != nil {
		coordinator = b.config.Orca.Supervised.CoordinatorTerminal
	}
	serializedReply, err := json.Marshal(struct {
		QuestionID string `json:"question_id"`
		Body       string `json:"body"`
		Run        string `json:"run"`
		From       string `json:"from"`
		Dispatch   string `json:"dispatch"`
	}{questionID, answer, ref.External.OrchestrationRun, coordinator, ref.External.Dispatch})
	if err != nil || len(serializedReply) > maxSupervisedMessageBytes-512 {
		return "", &BackendError{Code: BackendRejected, Diagnostic: "supervised reply serialized message exceeds the conservative 8 KiB pre-send bound", Cause: err}
	}
	coordinator, err = b.verifiedRunCoordinator(ctx, ref)
	if err != nil {
		return "", err
	}
	observation, err := b.Observe(ctx, ref)
	if err != nil {
		return "", err
	}
	if observation.Liveness != "live" {
		return "", &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised reply requires the original live Dispatch"}
	}
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "reply", "--id", questionID, "--body", answer, "--run", ref.External.OrchestrationRun, "--from", coordinator, "--json")
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
func (b orcaSupervisedBackend) SettleResources(ctx context.Context, ref ExecutionRef, outcome string) (SettlementReceipt, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return SettlementReceipt{}, err
	}
	if outcome != "succeeded" && outcome != "failed" {
		return SettlementReceipt{}, &BackendError{Code: BackendRejected, Diagnostic: "supervised resource release requires an accepted worker_done outcome"}
	}
	if _, err := b.settledWorker(ctx, ref, outcome); err != nil {
		return SettlementReceipt{ResidualResources: []string{"dispatch:" + ref.External.Dispatch}}, err
	}
	out, callErr := (orcaClient{cfg: b.config}).call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "worker-release", "--dispatch", ref.External.Dispatch, "--json")
	env, parseErr := parseEnvelope(out)
	if parseErr != nil {
		return SettlementReceipt{Disposition: "outcome_unknown", ResidualResources: []string{"dispatch:" + ref.External.Dispatch}}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "settled worker release outcome is uncertain; reconcile the exact Dispatch", Cause: errors.Join(callErr, parseErr)}
	}
	if env.Meta.RuntimeID != ref.External.CoordinatorRuntimeID {
		return SettlementReceipt{Disposition: "outcome_unknown", ResidualResources: []string{"dispatch:" + ref.External.Dispatch}}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "worker release receipt came from a different coordinator runtime"}
	}
	var result struct {
		DispatchID string   `json:"dispatchId"`
		State      string   `json:"state"`
		Reason     string   `json:"reason"`
		Residual   []string `json:"residualResources"`
		Mutation   struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || result.DispatchID != ref.External.Dispatch {
		return SettlementReceipt{Disposition: "outcome_unknown", ResidualResources: []string{"dispatch:" + ref.External.Dispatch}}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "worker release receipt does not identify the exact Dispatch", Cause: err}
	}
	receipt := SettlementReceipt{Disposition: result.State, PlatformRequestID: result.Mutation.RequestID, ResidualResources: result.Residual, Diagnostic: result.Reason}
	if result.State != "released" && result.State != "already_released" && len(receipt.ResidualResources) == 0 {
		receipt.ResidualResources = []string{"dispatch:" + ref.External.Dispatch}
	}
	if callErr != nil {
		receipt.Disposition = "outcome_unknown"
		return receipt, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "worker release command returned an uncertain outcome", Cause: callErr}
	}
	switch result.State {
	case "released", "already_released":
		if result.Mutation.RequestID == "" || len(result.Residual) != 0 {
			receipt.Disposition = "outcome_unknown"
			return receipt, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "worker release receipt lacks a request ID or retains resources"}
		}
		receipt.Disposition = "released"
		return receipt, nil
	case "retained":
		return receipt, &BackendError{Code: BackendRejected, Diagnostic: "Orca retained the settled worker terminal: " + result.Reason}
	case "release_pending", "release_unknown":
		receipt.Disposition = "outcome_unknown"
		return receipt, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "worker release requires reconciliation: " + result.State}
	default:
		receipt.Disposition = "outcome_unknown"
		return receipt, &BackendError{Code: BackendProtocolInvalid, Diagnostic: "worker release returned an unsupported state: " + result.State}
	}
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

func savedSupervisedTargetConflicts(saved, bound ExecutionTarget) bool {
	return saved.OrchestrationRun != "" && saved.OrchestrationRun != bound.OrchestrationRun ||
		saved.ExternalTask != "" && saved.ExternalTask != bound.ExternalTask ||
		saved.Dispatch != "" && saved.Dispatch != bound.Dispatch ||
		saved.ExecutionHost != "" && saved.ExecutionHost != bound.ExecutionHost ||
		saved.WorktreeID != "" && saved.WorktreeID != bound.WorktreeID
}
