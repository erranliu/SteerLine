package steerline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestLiveLocalCapabilityEvidence is an opt-in probe for the installed Codex
// executable. It uses an isolated repository and the production local spawner,
// output reader, process observer, and stop boundary.
func TestLiveLocalCapabilityEvidence(t *testing.T) {
	output := os.Getenv("STEERLINE_LOCAL_CAPABILITY_EVIDENCE_OUT")
	executable := os.Getenv("STEERLINE_LOCAL_CAPABILITY_CODEX")
	if output == "" || executable == "" {
		t.Skip("set STEERLINE_LOCAL_CAPABILITY_EVIDENCE_OUT and STEERLINE_LOCAL_CAPABILITY_CODEX")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "--quiet")
	cmd.Dir = repo
	if result, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("initialize isolated repository: %v: %s", err, result)
	}
	c := Config{Repository: repo, RunDir: filepath.Join(root, "state"), Host: "local", Agent: "codex", Codex: Command{Executable: executable}, CommandTimeout: "30s"}
	backend := localExecutionBackend{config: c}
	target := targetFor(c)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	probe, err := backend.Probe(ctx, target, agentFor(c))
	if err != nil {
		t.Fatal(err)
	}
	runID := newID()
	runDir := filepath.Join(c.RunDir, "runs", runID)
	if err = os.MkdirAll(runDir, 0700); err != nil {
		t.Fatal(err)
	}
	worker, err := (LocalSpawner{}).Start(ctx, SpawnSpec{
		RunID: runID, AttemptID: runID, Dir: repo,
		EventsPath: filepath.Join(runDir, "events.jsonl"),
		StderrPath: filepath.Join(runDir, "stderr.txt"),
		Command:    Command{Executable: executable, Args: []string{"exec", "--json", "--ephemeral", "--sandbox", "workspace-write", "-"}},
		Stdin:      strings.NewReader("Reply with exactly READY."), Timeout: 75 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	settled := false
	defer func() {
		if !settled {
			worker.Kill()
			select {
			case <-worker.Done:
			case <-time.After(10 * time.Second):
			}
		}
	}()
	ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendLocal, Protocol: ProtocolLocalProcess, Target: target, Agent: agentFor(c), RunID: runID, AttemptID: runID, External: worker.proc.Identity()}
	var first OutputBatch
	for {
		first, err = backend.ReadOutput(ctx, ref, "", 1<<20)
		if err == nil && len(first.Data) > 0 {
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("Codex produced no readable native output: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err = backend.Observe(ctx, ref); err != nil {
		t.Fatal(err)
	}
	stop, err := backend.StopExecution(ctx, ref, Operation{}, worker)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-worker.Done:
		settled = true
	case <-ctx.Done():
		t.Fatal("local worker did not exit after stop")
	}
	stop, err = backend.StopExecution(ctx, ref, Operation{}, worker)
	if err != nil || stop.Disposition != "confirmed" {
		t.Fatalf("original local process stop was not confirmed: %+v / %v", stop, err)
	}
	observation, err := backend.Observe(ctx, ref)
	if err != nil || observation.Liveness != "exited" {
		t.Fatalf("original local process is not proven absent: %+v / %v", observation, err)
	}
	read, err := backend.ReadOutput(ctx, ref, "", 1<<20)
	if err != nil || len(read.Data) == 0 {
		t.Fatalf("local output was not retained: %d bytes / %v", len(read.Data), err)
	}
	sum := sha256.Sum256(read.Data)
	evidence := map[string]any{
		"version": 1, "captured_at": time.Now().UTC(), "platform": runtime.GOOS + "/" + runtime.GOARCH,
		"backend": BackendLocal, "protocol": ProtocolLocalProcess, "target_id": target.ID,
		"agent": "codex", "codex_version": probe.PlatformVersion,
		"run_id": runID, "process_identity": ref.External,
		"output_sha256": hex.EncodeToString(sum[:]), "output_bytes": len(read.Data), "output_source": read.Source,
		"stop_status": OperationConfirmed, "process_absent": true,
	}
	body, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, append(body, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("captured %d bytes of %s output and confirmed original process exit: %s", len(read.Data), probe.PlatformVersion, output)
}
