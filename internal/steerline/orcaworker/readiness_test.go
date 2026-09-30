package orcaworker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadinessEvidenceUsesActualPlatformToolBlocks(t *testing.T) {
	data, err := os.ReadFile("../../../docs/validation/worker-environment-readiness/platform-gate-2026-09-30.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Command      string          `json:"command"`
		ScriptDigest string          `json:"probe_sha256"`
		Messages     json.RawMessage `json:"messages"`
		Receipt      ProbeReceipt    `json:"receipt"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	transcript, _ := json.Marshal(map[string]any{"messages": fixture.Messages})
	output := WorkerOutput{Source: "transcript", SourceExact: true, Provider: "codex", SourceIdentity: "actual-platform-source", Transcript: transcript}
	fact, err := ObserveProbe(output, fixture.Command, fixture.Receipt.Nonce, fixture.ScriptDigest, ProbeToolEvidence{})
	if err != nil || !fact.Verified || fact.CallID == "" || fact.MessageID == "" {
		t.Fatalf("real tool event was not recognized: %+v / %v", fact, err)
	}
	for _, name := range []string{"assistant", "screen", "inexact", "wrong_nonce", "nonzero", "apply_patch"} {
		t.Run(name, func(t *testing.T) {
			changed, nonce := output, fixture.Receipt.Nonce
			switch name {
			case "assistant":
				changed.Transcript = []byte(strings.ReplaceAll(string(transcript), `"role":"tool"`, `"role":"assistant"`))
			case "screen":
				changed.Source = "terminal"
			case "inexact":
				changed.SourceExact = false
			case "wrong_nonce":
				nonce = strings.Repeat("a", 32)
			case "nonzero":
				changed.Transcript = []byte(strings.ReplaceAll(string(transcript), `\"exit_code\":0`, `\"exit_code\":1`))
			case "apply_patch":
				changed.Transcript = []byte(strings.ReplaceAll(string(transcript), `"name":"exec"`, `"name":"apply_patch"`))
			}
			fact, err := ObserveProbe(changed, fixture.Command, nonce, fixture.ScriptDigest, ProbeToolEvidence{})
			if err != nil || fact.Verified {
				t.Fatalf("unsupported evidence accepted: %+v / %v", fact, err)
			}
		})
	}
}

func TestAgentInputNeverTargetsTrustMenuOrParentShell(t *testing.T) {
	identity := TerminalIdentity{Kind: "orca-native", Handle: "original", RuntimeID: "runtime", Incarnation: "inc"}
	for _, tc := range []struct {
		name, source string
		tail         []string
		ready        bool
		reason       string
	}{{"idle", "screen", []string{"› Ask Codex to do anything"}, true, ""}, {"trust", "screen", []string{"Trust this folder?", "› 1. Trust and continue", "2. Quit"}, false, "host_wait"}, {"stale_identity", "stream", []string{"PS C:\\repo>", "› Ask Codex to do anything"}, false, "agent_input_unverified"}} {
		t.Run(tc.name, func(t *testing.T) {
			call := func(_ context.Context, args ...string) (Result, error) {
				payload := map[string]any{"terminal": map[string]any{"handle": "original", "incarnationId": "inc", "connected": true, "source": tc.source, "tail": tc.tail}}
				data, _ := json.Marshal(payload)
				return Result{RuntimeID: "runtime", Payload: data}, nil
			}
			fact, ready, err := InspectAgentInput(context.Background(), call, identity)
			if err != nil || ready != tc.ready || fact.Reason != tc.reason {
				t.Fatalf("input qualification: %+v / %v / %v", fact, ready, err)
			}
		})
	}
}

func TestBusinessToolFactsRequireCurrentTaskAndActualCommand(t *testing.T) {
	for _, tc := range []struct {
		name, task, role, invocation, result string
		want                                 bool
	}{{"initialization", "current", "tool", `{"cmd":"git status"}`, "helper_unknown_error: setup refresh had errors", true}, {"ordinary_failure", "current", "tool", `{"cmd":"git status"}`, `{"exit_code":1,"output":"helper_unknown_error: setup refresh had errors"}`, false}, {"old_task", "previous", "tool", `{"cmd":"git status"}`, "helper_unknown_error: setup refresh had errors", false}, {"assistant_quote", "current", "assistant", `{"cmd":"git status"}`, "helper_unknown_error: setup refresh had errors", false}, {"generic_js", "current", "tool", `text("git status")`, "helper_unknown_error: setup refresh had errors", false}} {
		t.Run(tc.name, func(t *testing.T) {
			name := "exec_command"
			if tc.name == "generic_js" {
				name = "exec"
			}
			messages := []any{map[string]any{"role": "user", "source": "transcript", "timestamp": 1000, "blocks": []any{map[string]string{"type": "text", "text": "Your task ID is: " + tc.task}}}, map[string]any{"role": "assistant", "source": "transcript", "timestamp": 1001, "blocks": []any{map[string]string{"type": "tool-call", "name": name, "input": tc.invocation, "callId": "original-call"}}}, map[string]any{"role": tc.role, "source": "transcript", "timestamp": 1002, "blocks": []any{map[string]string{"type": "tool-result", "output": tc.result}}}}
			data, _ := json.Marshal(map[string]any{"messages": messages})
			output := WorkerOutput{Source: "transcript", SourceExact: true, Provider: "codex", SourceIdentity: "current-source", Transcript: data}
			_, fault, err := ObserveToolInitialization(output, "current", 1000, ToolContext{})
			if err != nil || fault != tc.want {
				t.Fatalf("business tool classification: %v / %v", fault, err)
			}
		})
	}
}

func TestReadinessReceiptRejectsStaleAndEscapingFiles(t *testing.T) {
	p := ProbeIdentity{Execution: strings.Repeat("a", 32), Turn: strings.Repeat("b", 32), Nonce: strings.Repeat("c", 32), ProfileDigest: "digest", Runtime: "runtime", Terminal: "terminal", Incarnation: "incarnation", Worktree: t.TempDir()}
	_, digest, _, err := PrepareProbe(p)
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := probeDirectory(p)
	path := filepath.Join(p.Worktree, dir, "receipt.json")
	r := ProbeReceipt{ProbeIdentity: p, InputSHA256: digest, GitHead: strings.Repeat("d", 40), Read: true, Write: true, Delete: true}
	write := func(r ProbeReceipt) {
		t.Helper()
		data, _ := json.Marshal(r)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(r)
	for i := 0; i < 2; i++ {
		if receipt, err := ReadProbe(p, digest); err != nil || receipt == nil {
			t.Fatalf("receipt replay is not idempotent: %v", err)
		}
	}
	for _, name := range []string{"nonce", "digest", "identity", "operations"} {
		t.Run(name, func(t *testing.T) {
			invalid := r
			switch name {
			case "nonce":
				invalid.Nonce = strings.Repeat("e", 32)
			case "digest":
				invalid.InputSHA256 = "old"
			case "identity":
				invalid.Terminal = "other"
			case "operations":
				invalid.Delete = false
			}
			write(invalid)
			if _, err := ReadProbe(p, digest); err == nil {
				t.Fatal("invalid receipt accepted")
			}
		})
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	data, _ := json.Marshal(r)
	if err := os.WriteFile(outside, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err == nil {
		if _, err := ReadProbe(p, digest); err == nil {
			t.Fatal("external receipt symlink accepted")
		}
		_ = os.Remove(path)
	}
}

func TestManagedOrcaLaunchFreezesPermissionsAndRejectsArgs(t *testing.T) {
	base := FixedAgentOptions{Host: "orca", Agent: "codex", Executable: "codex.exe", Sandbox: "workspace-write", WindowsSandbox: "unelevated", Shell: "powershell.exe"}
	for _, model := range []string{"", "chosen-model"} {
		options := base
		options.Model = model
		if model != "" {
			options.ReasoningEffort = "high"
		}
		command, err := FixedAgentCommand(options)
		if err != nil || !strings.Contains(command, "'never'") || !strings.Contains(command, "'windows.sandbox=unelevated'") {
			t.Fatalf("permissions not frozen: %s / %v", command, err)
		}
		if model == "" && strings.Contains(command, "--model") {
			t.Fatal("default model was overwritten")
		}
	}
	for _, args := range [][]string{{"-a", "on-request"}, {"--ask-for-approval=on-request"}, {"-sworkspace-write"}, {"-c", "windows.sandbox=unelevated"}, {"-capproval_policy=never"}, {"--config=model='x'"}, {"--config", "sandbox_workspace_write.writable_roots=['C:/']"}, {"--profile=x"}, {"-p", "x"}, {"--add-dir=C:/"}, {"-C", "C:/"}, {"exec"}, {"--dangerously-bypass-approvals-and-sandbox"}, {"--approve-for-me"}, {"--remote", "ws://localhost"}, {"--worktree"}} {
		options := base
		options.Args = args
		if _, err := FixedAgentCommand(options); err == nil {
			t.Fatalf("conflicting args accepted: %v", args)
		}
	}
	for _, mode := range []string{"", "automatic", "danger-full-access"} {
		options := base
		options.WindowsSandbox = mode
		if _, err := FixedAgentCommand(options); err == nil {
			t.Fatalf("invalid windows sandbox accepted: %q", mode)
		}
	}
}
