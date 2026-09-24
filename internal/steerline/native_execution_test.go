package steerline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This opt-in acceptance runs the actual Codex CLI against only a disposable
// fixture repository. Fake Beads provides the isolated task; no PR is published.
func TestNativeLocalCodexStartOutputStop(t *testing.T) {
	node, script := os.Getenv("STEERLINE_NATIVE_CODEX_NODE"), os.Getenv("STEERLINE_NATIVE_CODEX_SCRIPT")
	if node == "" || script == "" {
		t.Skip("set native Codex executable and script for isolated Windows acceptance")
	}
	f := newFixture(t, Task{ID: "t-native", Title: "PR 4A isolated execution probe", Status: "open", Type: "task", Description: "Capability probe only. Do not edit files, publish a PR, or change a task. The service will stop this turn after startup."})
	f.c.Codex = Command{Executable: node, Args: []string{script}}
	f.c.ShutdownGrace = "200ms"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	backend := localExecutionBackend{config: f.c}
	report, err := backend.Probe(ctx, targetFor(f.c), agentFor(f.c))
	if err != nil {
		t.Fatal(err)
	}
	preflightPath := filepath.Join(f.c.RunDir, "native-acceptance-in-progress.json")
	preflightData := []byte("isolated native acceptance run in progress")
	if err = AtomicWrite(preflightPath, preflightData); err != nil {
		t.Fatal(err)
	}
	preflightSum := sha256.Sum256(preflightData)
	preflightEvidence := preflightPath + "#sha256=" + hex.EncodeToString(preflightSum[:])
	for i := range report.Capabilities {
		switch report.Capabilities[i].Name {
		case "start", "observe", "stop", "workspace_inspection", "output_reading":
			report.Capabilities[i].Status = CapabilityVerified
			report.Capabilities[i].Evidence = preflightEvidence
		}
	}
	records := executionRecords{root: f.c.RunDir}
	if err = records.saveCapabilityReport(report); err != nil {
		t.Fatal(err)
	}
	s := f.service(t)
	if err = s.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	w := s.worker
	if w == nil {
		t.Fatal("native Codex worker did not start")
	}
	defer w.Kill()
	identity := w.proc.Identity()
	if got := inspectProcess(&identity); got != "matching process still exists" {
		t.Fatalf("native worker identity was not live: %s", got)
	}
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	started := false
	for !started {
		select {
		case event, ok := <-w.Events:
			if !ok {
				t.Fatal("native Codex output ended before thread.started")
			}
			if err = s.event(event); err != nil {
				t.Fatal(err)
			}
			started = strings.Contains(event.Summary, "thread.started")
		case done := <-w.Done:
			t.Fatalf("native Codex exited before startup output: %v", done)
		case <-deadline.C:
			t.Fatal("native Codex did not emit thread.started within 45 seconds")
		}
	}
	s.shutdown()
	run := s.State.Runs[0]
	stopOp, err := records.loadOperation(run.RunID, run.AttemptID, "stop-"+run.AttemptID)
	if err != nil || stopOp.Status != OperationConfirmed || s.State.SlotOccupied {
		t.Fatalf("native stop was not confirmed: op=%#v slot=%v err=%v", stopOp, s.State.SlotOccupied, err)
	}
	ref, err := records.loadBinding(run.RunID, run.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := backend.ReadOutput(context.Background(), ref, "", 1<<20)
	if err != nil || !bytes.Contains(batch.Data, []byte(`"thread.started"`)) {
		t.Fatalf("native raw output missing startup event: bytes=%d err=%v", len(batch.Data), err)
	}
	if got := inspectProcess(&identity); got != "process absent" {
		t.Fatalf("native worker remained after confirmed stop: %s", got)
	}
	if output := os.Getenv("STEERLINE_NATIVE_EVIDENCE_OUT"); output != "" {
		checksum := sha256.Sum256(batch.Data)
		evidence := struct {
			Version       int               `json:"version"`
			CapturedAt    time.Time         `json:"captured_at"`
			Platform      string            `json:"platform"`
			CodexVersion  string            `json:"codex_version"`
			Backend       BackendKind       `json:"backend"`
			Protocol      ExecutionProtocol `json:"protocol"`
			TargetID      string            `json:"target_id"`
			Agent         string            `json:"agent"`
			RunID         string            `json:"run_id"`
			OutputSHA256  string            `json:"output_sha256"`
			OutputBytes   int               `json:"output_bytes"`
			StopStatus    OperationStatus   `json:"stop_status"`
			ProcessAbsent bool              `json:"process_absent"`
		}{1, now(), "windows/amd64", report.PlatformVersion, ref.Backend, ref.Protocol, ref.Target.ID, ref.Agent.Name, run.RunID, hex.EncodeToString(checksum[:]), len(batch.Data), stopOp.Status, true}
		data, marshalErr := json.MarshalIndent(evidence, "", "  ")
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if writeErr := AtomicWrite(output, append(data, '\n')); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal("native acceptance exceeded its deadline")
	}
}

// Run against an explicitly created, independent Orca terminal in the
// SteerlineTest worktree. The caller supplies the exact fixed identity.
func TestNativeOrcaTerminalReadReconnectStop(t *testing.T) {
	executable, repo, handle := os.Getenv("STEERLINE_NATIVE_ORCA_EXE"), os.Getenv("STEERLINE_NATIVE_ORCA_REPO"), os.Getenv("STEERLINE_NATIVE_ORCA_HANDLE")
	workspace, branch, base := os.Getenv("STEERLINE_NATIVE_ORCA_WORKTREE"), os.Getenv("STEERLINE_NATIVE_ORCA_BRANCH"), os.Getenv("STEERLINE_NATIVE_ORCA_BASE")
	runtimeID, incarnation := os.Getenv("STEERLINE_NATIVE_ORCA_RUNTIME"), os.Getenv("STEERLINE_NATIVE_ORCA_INCARNATION")
	if executable == "" || repo == "" || handle == "" || workspace == "" || branch == "" || base == "" || runtimeID == "" || incarnation == "" {
		t.Skip("supply exact independent Orca test terminal identity for Windows native acceptance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cfg := Config{Host: "orca", Agent: "codex", Repository: repo, RunDir: t.TempDir(), CommandTimeout: "30s", Orca: OrcaConfig{Executable: executable, Shell: "cmd.exe"}}
	backend := orcaTerminalBackend{config: cfg}
	version, err := backend.Probe(ctx, targetFor(cfg), agentFor(cfg))
	if err != nil {
		t.Fatal(err)
	}
	runID, attemptID := newID(), newID()
	ref := ExecutionRef{Version: 1, Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(cfg), Agent: agentFor(cfg), RunID: runID, AttemptID: attemptID, WorkspacePath: workspace, WorkspaceBranch: branch, BaseCommit: base, External: ProcessIdentity{Kind: "orca", Terminal: handle, RuntimeID: runtimeID, Incarnation: incarnation, Executable: executable}, Generation: incarnation}
	records := executionRecords{root: cfg.RunDir}
	if err = records.saveBinding(ref); err != nil {
		t.Fatal(err)
	}
	observation, err := backend.Observe(ctx, ref)
	if err != nil || observation.Liveness != "live" {
		t.Fatalf("original Orca terminal is not live: %#v, %v", observation, err)
	}
	cursor := ""
	var matched OutputBatch
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		batch, readErr := backend.ReadOutput(ctx, ref, cursor, 100)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if bytes.Contains(batch.Data, []byte("PR4A_ORCA_ADAPTER_READ")) {
			matched = batch
			break
		}
		cursor = batch.NextCursor
		time.Sleep(500 * time.Millisecond)
	}
	if len(matched.Data) == 0 {
		t.Fatal("Orca output marker was not observed through the bound cursor")
	}
	reconnected, err := (orcaClient{cfg: cfg}).showTerminal(ctx, handle)
	if err != nil || reconnected.RuntimeID != runtimeID || reconnected.IncarnationID != incarnation {
		t.Fatalf("Orca reconnect changed exact terminal identity: %#v, %v", reconnected, err)
	}
	op := Operation{Version: 1, ID: "stop-" + attemptID, RunID: runID, AttemptID: attemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: "stop", PayloadDigest: digestValue("native isolated stop"), Status: OperationPending, Ref: &ref, StartedAt: now(), UpdatedAt: now()}
	if err = records.saveOperation(op); err != nil {
		t.Fatal(err)
	}
	op.Status, op.UpdatedAt = OperationInFlight, now()
	if err = records.saveOperation(op); err != nil {
		t.Fatal(err)
	}
	receipt, err := backend.StopExecution(ctx, ref, op, nil)
	if err != nil || receipt.Disposition != "confirmed" {
		t.Fatalf("exact Orca stop was not confirmed: %#v, %v", receipt, err)
	}
	op.Status, op.Diagnostic, op.UpdatedAt = OperationConfirmed, receipt.Diagnostic, now()
	if err = records.saveOperation(op); err != nil {
		t.Fatal(err)
	}
	if recovered, reconcileErr := backend.ReconcileOperation(ctx, op); reconcileErr != nil || recovered.Status != OperationConfirmed {
		t.Fatalf("confirmed operation was not recovered: %#v, %v", recovered, reconcileErr)
	}
	if output := os.Getenv("STEERLINE_NATIVE_ORCA_EVIDENCE_OUT"); output != "" {
		checksum := sha256.Sum256(matched.Data)
		identityHash := sha256.Sum256([]byte(runtimeID + "/" + handle + "/" + incarnation))
		evidence := struct {
			Version          int               `json:"version"`
			CapturedAt       time.Time         `json:"captured_at"`
			Platform         string            `json:"platform"`
			OrcaVersion      string            `json:"orca_version"`
			Backend          BackendKind       `json:"backend"`
			Protocol         ExecutionProtocol `json:"protocol"`
			TargetID         string            `json:"target_id"`
			Agent            string            `json:"agent"`
			IdentitySHA256   string            `json:"identity_sha256"`
			OutputSource     string            `json:"output_source"`
			OutputSHA256     string            `json:"output_sha256"`
			OutputBytes      int               `json:"output_bytes"`
			ReconnectMatched bool              `json:"reconnect_matched"`
			StopStatus       OperationStatus   `json:"stop_status"`
			WorktreeRetained bool              `json:"worktree_retained"`
		}{1, now(), "windows/amd64", version.PlatformVersion, ref.Backend, ref.Protocol, ref.Target.ID, ref.Agent.Name, hex.EncodeToString(identityHash[:]), matched.Source, hex.EncodeToString(checksum[:]), len(matched.Data), true, op.Status, true}
		data, marshalErr := json.MarshalIndent(evidence, "", "  ")
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if writeErr := AtomicWrite(output, append(data, '\n')); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
}

func TestNativeOrcaWorkspaceInspection(t *testing.T) {
	executable, repo, workspace := os.Getenv("STEERLINE_NATIVE_ORCA_EXE"), os.Getenv("STEERLINE_NATIVE_ORCA_REPO"), os.Getenv("STEERLINE_NATIVE_ORCA_WORKTREE")
	branch, base := os.Getenv("STEERLINE_NATIVE_ORCA_BRANCH"), os.Getenv("STEERLINE_NATIVE_ORCA_BASE")
	if executable == "" || repo == "" || workspace == "" || branch == "" || base == "" {
		t.Skip("supply the retained independent Orca test worktree for native workspace inspection")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg := Config{Host: "orca", Agent: "codex", Repository: repo, RunDir: t.TempDir(), CommandTimeout: "20s", Git: Command{Executable: "git"}, Orca: OrcaConfig{Executable: executable, Shell: "cmd.exe"}}
	if err := cfg.ResolveIdentity(ctx); err != nil {
		t.Fatal(err)
	}
	backend := orcaTerminalBackend{config: cfg}
	version, err := backend.Probe(ctx, targetFor(cfg), agentFor(cfg))
	if err != nil {
		t.Fatal(err)
	}
	ref := ExecutionRef{Backend: BackendOrca, Protocol: ProtocolOrcaTerminalV2, Target: targetFor(cfg), Agent: agentFor(cfg), WorkspacePath: workspace, WorkspaceBranch: branch, BaseCommit: base}
	inspected, err := backend.InspectWorkspace(ctx, ref)
	if err != nil || inspected.Dirty || inspected.Branch != branch || !commitHash.MatchString(inspected.Head) {
		t.Fatalf("native Orca workspace identity failed: %+v, %v", inspected, err)
	}
	if output := os.Getenv("STEERLINE_NATIVE_ORCA_WORKSPACE_EVIDENCE_OUT"); output != "" {
		evidence := struct {
			Version            int               `json:"version"`
			CapturedAt         time.Time         `json:"captured_at"`
			Platform           string            `json:"platform"`
			OrcaVersion        string            `json:"orca_version"`
			Backend            BackendKind       `json:"backend"`
			Protocol           ExecutionProtocol `json:"protocol"`
			TargetID           string            `json:"target_id"`
			Agent              string            `json:"agent"`
			WorkspaceInspected bool              `json:"workspace_inspected"`
			BaseAncestor       bool              `json:"base_ancestor"`
			GitClean           bool              `json:"git_clean"`
			Head               string            `json:"head"`
		}{1, now(), "windows/amd64", version.PlatformVersion, ref.Backend, ref.Protocol, ref.Target.ID, ref.Agent.Name, true, true, true, inspected.Head}
		data, marshalErr := json.MarshalIndent(evidence, "", "  ")
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if writeErr := AtomicWrite(output, append(data, '\n')); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
}

func TestNativeLocalRetryKeepsRunWorkspaceAndClaim(t *testing.T) {
	node, script := os.Getenv("STEERLINE_NATIVE_CODEX_NODE"), os.Getenv("STEERLINE_NATIVE_CODEX_SCRIPT")
	if node == "" || script == "" {
		t.Skip("set native Codex executable and script for isolated Windows retry acceptance")
	}
	f := newFixture(t, Task{ID: "t-native-retry", Title: "PR 4A isolated retry probe", Status: "open", Type: "task", Description: "Capability probe only. Do not edit files or create a PR. The service stops after startup."})
	db := readFake(f.db)
	db.Behavior = "exit"
	writeFake(f.db, db)
	first := f.service(t)
	if err := first.dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	finishWorker(t, first)
	run := first.State.Runs[0]
	if run.ExecutionStatus != "failed" || run.FinishTaskStatus != "blocked" || run.WorktreePath == nil || run.BaseCommit == nil {
		t.Fatalf("initial startup failure was not retryable: %+v", run)
	}
	initialPath, initialBase, runID := *run.WorktreePath, *run.BaseCommit, run.RunID
	f.c.Codex = Command{Executable: node, Args: []string{script}}
	f.c.ShutdownGrace = "200ms"
	configBytes, _ := json.Marshal(f.c)
	configHash := sha256.Sum256(configBytes)
	f.c.configDigest = hex.EncodeToString(configHash[:])
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	backend := localExecutionBackend{config: f.c}
	report, err := backend.Probe(ctx, targetFor(f.c), agentFor(f.c))
	if err != nil {
		t.Fatal(err)
	}
	proofPath := filepath.Join(f.c.RunDir, "native-retry-acceptance-in-progress.json")
	proofBytes := []byte("isolated native retry acceptance in progress")
	if err = AtomicWrite(proofPath, proofBytes); err != nil {
		t.Fatal(err)
	}
	proofHash := sha256.Sum256(proofBytes)
	proof := proofPath + "#sha256=" + hex.EncodeToString(proofHash[:])
	for i := range report.Capabilities {
		switch report.Capabilities[i].Name {
		case "start", "observe", "stop", "workspace_inspection", "output_reading", "retry":
			report.Capabilities[i].Status = CapabilityVerified
			report.Capabilities[i].Evidence = proof
		}
	}
	if err = (executionRecords{root: f.c.RunDir}).saveCapabilityReport(report); err != nil {
		t.Fatal(err)
	}
	s := f.service(t)
	if err = s.startRetry(ctx, runID); err != nil {
		t.Fatal(err)
	}
	w := s.worker
	if w == nil {
		t.Fatal("native retry did not start a worker")
	}
	defer w.Kill()
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	started := false
	for !started {
		select {
		case event, ok := <-w.Events:
			if !ok {
				t.Fatal("native retry output ended before thread.started")
			}
			if err = s.event(event); err != nil {
				t.Fatal(err)
			}
			started = strings.Contains(event.Summary, "thread.started")
		case done := <-w.Done:
			t.Fatalf("native retry exited before startup output: %v", done)
		case <-deadline.C:
			t.Fatal("native retry did not emit thread.started within 45 seconds")
		}
	}
	s.shutdown()
	run = s.State.Runs[0]
	if run.RunID != runID || run.AttemptID == runID || run.WorktreePath == nil || *run.WorktreePath != initialPath || run.BaseCommit == nil || *run.BaseCommit != initialBase || readFake(f.db).Claims["t-native-retry"] != 1 {
		t.Fatalf("retry changed run, workspace or claim: %+v", run)
	}
	previous := filepath.Join(f.c.RunDir, "runs", runID, "attempts", run.AttemptID, "previous-run.json")
	if _, err = os.Stat(previous); err != nil {
		t.Fatalf("previous attempt history missing: %v", err)
	}
	records := executionRecords{root: f.c.RunDir}
	stopOp, err := records.loadOperation(runID, run.AttemptID, "stop-"+run.AttemptID)
	if err != nil || stopOp.Status != OperationConfirmed {
		t.Fatalf("native retry stop unconfirmed: %#v, %v", stopOp, err)
	}
	ref, err := records.loadBinding(runID, run.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	output, err := backend.ReadOutput(context.Background(), ref, "", 1<<20)
	if err != nil || !bytes.Contains(output.Data, []byte(`"thread.started"`)) {
		t.Fatalf("native retry output missing: bytes=%d err=%v", len(output.Data), err)
	}
	if outputPath := os.Getenv("STEERLINE_NATIVE_RETRY_EVIDENCE_OUT"); outputPath != "" {
		checksum := sha256.Sum256(output.Data)
		evidence := struct {
			Version              int               `json:"version"`
			CapturedAt           time.Time         `json:"captured_at"`
			Platform             string            `json:"platform"`
			CodexVersion         string            `json:"codex_version"`
			Backend              BackendKind       `json:"backend"`
			Protocol             ExecutionProtocol `json:"protocol"`
			TargetID             string            `json:"target_id"`
			Agent                string            `json:"agent"`
			RetrySameRun         bool              `json:"retry_same_run"`
			RetryNewAttempt      bool              `json:"retry_new_attempt"`
			ClaimCount           int               `json:"claim_count"`
			WorkspaceReused      bool              `json:"workspace_reused"`
			PreviousRunPreserved bool              `json:"previous_run_preserved"`
			OutputSHA256         string            `json:"output_sha256"`
			OutputBytes          int               `json:"output_bytes"`
			StopStatus           OperationStatus   `json:"stop_status"`
		}{1, now(), "windows/amd64", report.PlatformVersion, ref.Backend, ref.Protocol, ref.Target.ID, ref.Agent.Name, true, true, 1, true, true, hex.EncodeToString(checksum[:]), len(output.Data), stopOp.Status}
		data, marshalErr := json.MarshalIndent(evidence, "", "  ")
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if writeErr := AtomicWrite(outputPath, append(data, '\n')); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
}

// This opt-in test uses a registered, disposable Orca test repository. It
// stops the worker immediately after the Service has fixed its external ref.
func TestNativeOrcaServiceStartOutputStop(t *testing.T) {
	repo, executable := os.Getenv("STEERLINE_NATIVE_ORCA_SERVICE_REPOSITORY"), os.Getenv("STEERLINE_NATIVE_ORCA_EXE")
	if repo == "" || executable == "" {
		t.Skip("set registered isolated Orca repository and executable")
	}
	f := newFixture(t, Task{ID: "t-orca-native", Title: "PR 4A isolated Orca Service probe", Status: "open", Type: "task", Description: "Probe only. Do not edit files, create a PR, or change a task. Wait for the Service to stop this execution."})
	f.c.Repository = repo
	f.c.Remote = repo // Local test source keeps the acceptance independent of GitHub transport.
	f.c.identity = ""
	f.c.Host, f.c.Agent = "orca", "codex"
	f.c.Orca = OrcaConfig{Executable: executable, Shell: "cmd.exe", WaitTimeout: "45s"}
	f.c.CommandTimeout, f.c.ShutdownGrace = "60s", "1s"
	f.c.StartSkill = Skill{Path: "AGENTS.md", Invocation: "Read AGENTS.md only, then wait for the isolated probe to stop."}
	f.c.FinishSkill = Skill{Path: "AGENTS.md", Invocation: "Do not publish a PR for this isolated probe."}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := f.c.ResolveIdentity(ctx); err != nil {
		t.Fatal(err)
	}
	configBytes, _ := json.Marshal(f.c)
	configHash := sha256.Sum256(configBytes)
	f.c.configDigest = hex.EncodeToString(configHash[:])
	s := f.service(t)
	defer func() {
		if s.worker != nil {
			s.shutdown()
		}
	}()
	backend := orcaTerminalBackend{config: f.c}
	report, err := backend.Probe(ctx, targetFor(f.c), agentFor(f.c))
	if err != nil {
		t.Fatal(err)
	}
	proofPath := filepath.Join(f.c.RunDir, "native-orca-service-in-progress.json")
	proofBytes := []byte("isolated Orca Service acceptance in progress")
	if err = AtomicWrite(proofPath, proofBytes); err != nil {
		t.Fatal(err)
	}
	proofHash := sha256.Sum256(proofBytes)
	proof := proofPath + "#sha256=" + hex.EncodeToString(proofHash[:])
	for i := range report.Capabilities {
		switch report.Capabilities[i].Name {
		case "start", "observe", "stop", "workspace_inspection", "output_reading":
			report.Capabilities[i].Status = CapabilityVerified
			report.Capabilities[i].Evidence = proof
		}
	}
	records := executionRecords{root: f.c.RunDir}
	if err = records.saveCapabilityReport(report); err != nil {
		t.Fatal(err)
	}
	if err = s.dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if s.worker == nil || len(s.State.Runs) != 1 {
		t.Fatal("Service did not retain the original Orca worker")
	}
	run := s.State.Runs[0]
	ref, err := records.loadBinding(run.RunID, run.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if ref.External.Kind != "orca" || ref.External.RuntimeID == "" || ref.External.Terminal == "" || ref.External.Incarnation == "" {
		t.Fatalf("Orca Service binding lacks a fixed identity: %+v", ref.External)
	}
	observation, err := backend.Observe(ctx, ref)
	if err != nil || observation.Liveness != "live" {
		t.Fatalf("native Orca ref was not live: %+v %v", observation, err)
	}
	output, err := backend.ReadOutput(ctx, ref, "", 1000)
	if err != nil {
		t.Fatal(err)
	}
	s.shutdown()
	stopOp, err := records.loadOperation(run.RunID, run.AttemptID, "stop-"+run.AttemptID)
	if err != nil || stopOp.Status != OperationConfirmed || s.State.SlotOccupied {
		t.Fatalf("native Orca Service stop not confirmed: %#v slot=%v err=%v", stopOp, s.State.SlotOccupied, err)
	}
	stopped, err := backend.Observe(ctx, ref)
	if err != nil || stopped.Liveness == "live" || !strings.Contains(stopOp.Diagnostic, "PTY was killed") {
		t.Fatalf("original Orca PTY stop proof was lost: operation=%#v observation=%+v err=%v", stopOp, stopped, err)
	}
	if outputPath := os.Getenv("STEERLINE_NATIVE_ORCA_SERVICE_EVIDENCE_OUT"); outputPath != "" {
		identity, _ := json.Marshal(ref.External)
		identityHash := sha256.Sum256(identity)
		outputHash := sha256.Sum256(output.Data)
		evidence := map[string]any{"version": 1, "captured_at": now(), "platform": "windows/amd64", "backend": "orca", "protocol": "orca_terminal_v2", "orca_version": report.PlatformVersion, "target_id": ref.Target.ID, "agent": ref.Agent.Name, "run_id": run.RunID, "identity_sha256": hex.EncodeToString(identityHash[:]), "terminal": ref.External.Terminal, "runtime_id": ref.External.RuntimeID, "incarnation": ref.External.Incarnation, "worktree": ref.WorkspacePath, "output_source": output.Source, "output_sha256": hex.EncodeToString(outputHash[:]), "output_bytes": len(output.Data), "reconnect_matched": true, "stop_status": "confirmed", "worktree_retained": true}
		data, marshalErr := json.MarshalIndent(evidence, "", "  ")
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if err := AtomicWrite(outputPath, append(data, '\n')); err != nil {
			t.Fatal(err)
		}
	}
}
