package steerlineapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

// TestLiveLocalCapabilityEvidence is an opt-in probe for the installed Codex
// executable. It uses isolated native exec output and the production managed
// process observer/stop boundary. The report retains its legacy exec protocol
// scope; it does not certify current app-server permissions or turn semantics.
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
	c := Config{Repository: repo, RunDir: filepath.Join(root, "state"), Host: "local", Agent: "codex", CommandTimeout: "30s"}
	setTestCodexProfile(t, &c, codexworker.ProfileConfig{Executable: executable})
	entry, err := c.ExecutionDirectory.Resolve(c)
	require(t, err)
	backend := entry.Backend(c)
	stopper, ok := backend.(executionapi.ExecutionStopper)
	if !ok {
		t.Fatal("application backend has no managed stop port")
	}
	target := entry.Target(c)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	probe, err := backend.Probe(ctx, target, executionapi.AgentSpec{Name: c.Agent})
	if err != nil {
		t.Fatal(err)
	}
	runID := nativePolicyID()
	runDir := filepath.Join(c.RunDir, "runs", runID)
	if err = os.MkdirAll(runDir, 0700); err != nil {
		t.Fatal(err)
	}
	worker, err := startNativeExecProbe(ctx, c, executable, repo, runDir)
	if err != nil {
		t.Fatal(err)
	}
	settled := false
	defer func() {
		if !settled {
			worker.handle.Kill()
			select {
			case <-worker.Done:
			case <-time.After(10 * time.Second):
			}
		}
	}()
	ref := executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalCodexAppServer, Target: target, Agent: executionapi.AgentSpec{Name: c.Agent}, RunID: runID, AttemptID: runID, External: worker.handle.Identity()}
	var first executionapi.OutputBatch
	for {
		first, err = readNativeExecOutput(filepath.Join(runDir, "events.jsonl"), 1<<20)
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
	stop, err := stopper.StopExecution(ctx, ref, executionapi.Operation{}, worker.handle)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-worker.Done:
		settled = true
	case <-ctx.Done():
		t.Fatal("local worker did not exit after stop")
	}
	stop, err = stopper.StopExecution(ctx, ref, executionapi.Operation{}, worker.handle)
	if err != nil || stop.Disposition != "confirmed" {
		t.Fatalf("original local process stop was not confirmed: %+v / %v", stop, err)
	}
	observation, err := backend.Observe(ctx, ref)
	if err != nil || observation.Liveness != "exited" {
		t.Fatalf("original local process is not proven absent: %+v / %v", observation, err)
	}
	read, err := readNativeExecOutput(filepath.Join(runDir, "events.jsonl"), 1<<20)
	if err != nil || len(read.Data) == 0 {
		t.Fatalf("local output was not retained: %d bytes / %v", len(read.Data), err)
	}
	sum := sha256.Sum256(read.Data)
	evidence := map[string]any{
		"version": 1, "captured_at": time.Now().UTC(), "platform": runtime.GOOS + "/" + runtime.GOARCH,
		"backend": executionapi.BackendLocal, "protocol": executionapi.ProtocolLocalProcess, "target_id": target.ID,
		"agent": "codex", "codex_version": probe.PlatformVersion,
		"run_id": runID, "process_identity": ref.External,
		"output_sha256": hex.EncodeToString(sum[:]), "output_bytes": len(read.Data), "output_source": read.Source,
		"stop_status": executionapi.OperationConfirmed, "process_absent": true,
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

type nativeExecProbe struct {
	handle executionapi.StopHandle
	Done   chan error
}

func startNativeExecProbe(ctx context.Context, c Config, executable, repo, runDir string) (*nativeExecProbe, error) {
	raw, err := os.OpenFile(filepath.Join(runDir, "events.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	stderr, err := os.OpenFile(filepath.Join(runDir, "stderr.txt"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		raw.Close()
		return nil, err
	}
	command := exec.Command(executable, "exec", "--json", "--ephemeral", "--sandbox", "workspace-write", "-")
	command.Dir, command.Stdin, command.Stdout, command.Stderr = repo, strings.NewReader("Reply with exactly READY."), raw, stderr
	handle, err := c.ProcessHost.StartManaged(command)
	if err != nil {
		raw.Close()
		stderr.Close()
		return nil, err
	}
	probe := &nativeExecProbe{handle: handle, Done: make(chan error, 1)}
	go func() {
		waited := make(chan error, 1)
		go func() { waited <- command.Wait() }()
		timer := time.NewTimer(75 * time.Second)
		defer timer.Stop()
		var result error
		select {
		case result = <-waited:
		case <-ctx.Done():
			handle.Kill()
			<-waited
			result = ctx.Err()
		case <-timer.C:
			handle.Kill()
			result = <-waited
			if result == nil {
				result = context.DeadlineExceeded
			}
		}
		if err := raw.Sync(); result == nil {
			result = err
		}
		if err := raw.Close(); result == nil {
			result = err
		}
		if err := stderr.Close(); result == nil {
			result = err
		}
		handle.Close()
		probe.Done <- result
	}()
	return probe, nil
}
func readNativeExecOutput(path string, limit int) (executionapi.OutputBatch, error) {
	file, err := os.Open(path)
	if err != nil {
		return executionapi.OutputBatch{}, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, int64(limit)))
	return executionapi.OutputBatch{Data: append([]byte(nil), raw...), Source: "local-jsonl", ObservedAt: time.Now().UTC()}, err
}
