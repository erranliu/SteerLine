package orcaworker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestProbeContentBuildIsPureAndWritesAreExplicit(t *testing.T) {
	worktree := t.TempDir()
	identity := executionbackend.ProbeIdentity{Execution: strings.Repeat("a", 32), Turn: strings.Repeat("b", 32), Nonce: strings.Repeat("c", 32),
		ProfileDigest: "profile", Runtime: "runtime", Terminal: "terminal", Incarnation: "incarnation", Worktree: worktree}
	plan, err := BuildProbe(identity)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(worktree, ".steerline-readiness", identity.Execution, identity.Turn)
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("pure probe planning wrote files: %v", err)
	}
	if err = WriteProbe(identity, plan); err != nil {
		t.Fatal(err)
	}
	if ready, err := CheckProbeFiles(identity, plan); err != nil || !ready {
		t.Fatalf("persisted probe does not match its plan: ready=%v err=%v", ready, err)
	}
	if err = CleanupProbe(identity); err != nil {
		t.Fatal(err)
	}
	if absent, err := ProbeFilesAbsent(identity); err != nil || !absent {
		t.Fatalf("probe cleanup was not observed: absent=%v err=%v", absent, err)
	}
}

func TestReadinessEvidenceUsesActualPlatformToolBlocks(t *testing.T) {
	data, err := os.ReadFile("../../../docs/validation/worker-environment-readiness/platform-gate-2026-09-30.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Command      string                        `json:"command"`
		ScriptDigest string                        `json:"probe_sha256"`
		Messages     json.RawMessage               `json:"messages"`
		Receipt      executionbackend.ProbeReceipt `json:"receipt"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	transcript, _ := json.Marshal(map[string]any{"messages": fixture.Messages})
	output := WorkerOutput{Source: "transcript", SourceExact: true, Provider: "codex", SourceIdentity: "actual-platform-source", Transcript: transcript}
	fact, err := ObserveProbe(output, fixture.Command, fixture.Receipt.Nonce, fixture.ScriptDigest, executionbackend.ProbeToolEvidence{})
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
			fact, err := ObserveProbe(changed, fixture.Command, nonce, fixture.ScriptDigest, executionbackend.ProbeToolEvidence{})
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
	}{{"idle", "screen", []string{"› Ask Codex to do anything", "? for shortcuts"}, true, ""}, {"alternate_prompt", "screen", []string{"› Find a bug in this repository", "GPT-6.1-Sol medium · repo", "? for shortcuts"}, true, ""}, {"busy", "screen", []string{"• Working (2s • esc to interrupt)", "› Ask Codex to do anything", "? for shortcuts"}, false, "agent_input_unverified"}, {"shell_spoof", "screen", []string{"PS C:\\repo>", "› Ask Codex to do anything"}, false, "agent_input_unverified"}, {"trust", "screen", []string{"Trust this folder?", "› 1. Trust and continue", "2. Quit"}, false, "host_wait"}, {"stale_identity", "stream", []string{"PS C:\\repo>", "› Ask Codex to do anything"}, false, "agent_input_unverified"}} {
		t.Run(tc.name, func(t *testing.T) {
			call := func(_ context.Context, args ...string) (Result, error) {
				if len(args) > 1 && args[1] == "read" && !strings.Contains(strings.Join(args, " "), "--screen") {
					t.Fatal("input observation did not request rendered screen")
				}
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

func TestProbeInitializationFailureCannotBeOverwrittenByRetry(t *testing.T) {
	command, nonce, digest := "powershell.exe -File probe.ps1", strings.Repeat("a", 32), strings.Repeat("b", 64)
	invocation, _ := json.Marshal(map[string]string{"cmd": command})
	success, _ := json.Marshal(map[string]any{"exit_code": 0, "output": "SLREADINESS:" + nonce + ":" + digest})
	call := map[string]any{"role": "assistant", "source": "transcript", "blocks": []any{map[string]string{"type": "tool-call", "name": "exec_command", "input": string(invocation), "callId": "call"}}}
	result := func(text string) any {
		return map[string]any{"role": "tool", "source": "transcript", "blocks": []any{map[string]string{"type": "tool-result", "output": text}}}
	}
	data, _ := json.Marshal(map[string]any{"messages": []any{call, result("helper_unknown_error: setup refresh had errors"), call, result(string(success))}})
	output := WorkerOutput{Source: "transcript", SourceExact: true, Provider: "codex", SourceIdentity: "source", Transcript: data}
	fact, err := ObserveProbe(output, command, nonce, digest, executionbackend.ProbeToolEvidence{})
	if err != nil || fact.FailureCode == "" || fact.Verified {
		t.Fatalf("confirmed fault was overwritten: %+v / %v", fact, err)
	}
	data, _ = json.Marshal(map[string]any{"messages": []any{call, result(string(success))}})
	output.Transcript = data
	fact, err = ObserveProbe(output, command, nonce, digest, fact)
	if err != nil || fact.FailureCode == "" || fact.Verified {
		t.Fatal("later page erased confirmed initialization fault")
	}
}

func TestActualSavedCommandWrapperProvidesProbeEvidence(t *testing.T) {
	data, err := os.ReadFile("../../../docs/validation/worker-environment-readiness/platform-wrapper-audit-2026-09-30.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Command, Nonce, Digest string
		Messages               json.RawMessage
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	transcript, _ := json.Marshal(map[string]any{"messages": fixture.Messages})
	output := WorkerOutput{Source: "transcript", SourceExact: true, Provider: "codex", SourceIdentity: "source", Transcript: transcript}
	fact, err := ObserveProbe(output, fixture.Command, fixture.Nonce, fixture.Digest, executionbackend.ProbeToolEvidence{})
	if err != nil || !fact.Verified {
		t.Fatalf("real saved wrapper rejected: %+v / %v", fact, err)
	}
	for _, input := range []string{`const r = await tools.exec_command({cmd:"probe"});r={exit_code:0};text(r);`, `const r = await tools.exec_command({cmd:"probe"});text(fake);`, `const r = await tools.exec_command({cmd:"probe"});store("key",fake);text(r);`} {
		if _, ok := commandInvocation("exec", input); ok {
			t.Fatalf("modified result wrapper accepted: %s", input)
		}
	}
}

func TestActualStringifiedCommandWrapperProvidesProbeEvidence(t *testing.T) {
	data, err := os.ReadFile("../../../docs/validation/worker-environment-readiness/platform-stringified-wrapper-20261001.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Command, Nonce, Digest string
		Messages               json.RawMessage
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	transcript, _ := json.Marshal(map[string]any{"messages": fixture.Messages})
	fact, err := ObserveProbe(WorkerOutput{Source: "transcript", SourceExact: true, Provider: "codex", SourceIdentity: "actual-source", Transcript: transcript}, fixture.Command, fixture.Nonce, fixture.Digest, executionbackend.ProbeToolEvidence{})
	if err != nil || !fact.Verified {
		t.Fatalf("actual serialized command result was not recognized: %+v / %v", fact, err)
	}
	for _, input := range []string{
		`const r = await tools.exec_command({cmd:"probe"});text(JSON.stringify(fake));`,
		`const r = await tools.exec_command({cmd:"probe"});r={exit_code:0};text(JSON.stringify(r));`,
		`const r = await tools.exec_command({cmd:"probe"});text(JSON.stringify({exit_code:0}));`,
	} {
		if _, ok := commandInvocation("exec", input); ok {
			t.Fatalf("fabricated serialized result accepted: %s", input)
		}
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
			_, fault, err := ObserveToolInitialization(output, "current", 1000, executionbackend.ToolContext{})
			if err != nil || fault != tc.want {
				t.Fatalf("business tool classification: %v / %v", fault, err)
			}
		})
	}
}

func TestProjectedCommandOutputOnlyProvesBusinessAttempt(t *testing.T) {
	input := `const r = await tools.exec_command({cmd:"git status"});text(r.output);`
	if _, ok := commandInvocation("exec", input); ok {
		t.Fatal("stdout-only wrapper accepted as a complete native result")
	}
	if command, invoked, full := businessCommandInvocation("exec", input); command != "git status" || !invoked || full {
		t.Fatal("actual stdout-only command attempt not recognized")
	}
	for _, bad := range []string{`const r = await tools.exec_command({cmd:"git status"});text(fake.output);`, `const r = await tools.exec_command({cmd:"git status"});r={output:"fake"};text(r.output);`} {
		if _, invoked, _ := businessCommandInvocation("exec", bad); invoked {
			t.Fatal("altered output projection accepted")
		}
	}
	data, _ := json.Marshal(map[string]any{"messages": []any{
		map[string]any{"role": "user", "source": "transcript", "timestamp": 1000, "blocks": []any{map[string]string{"type": "text", "text": "Your task ID is: current"}}},
		map[string]any{"role": "assistant", "source": "transcript", "timestamp": 1001, "blocks": []any{map[string]string{"type": "tool-call", "name": "exec", "input": input, "callId": "call"}}},
		map[string]any{"role": "tool", "source": "transcript", "timestamp": 1002, "blocks": []any{map[string]string{"type": "tool-result", "output": "helper_unknown_error: setup refresh had errors"}}},
	}})
	state, fault, err := ObserveToolInitialization(WorkerOutput{Source: "transcript", SourceExact: true, Provider: "codex", SourceIdentity: "source", Transcript: data}, "current", 1000, executionbackend.ToolContext{})
	if err != nil || !state.Started || fault {
		t.Fatalf("stdout was treated as native failure or attempt was lost: %+v / %v / %v", state, fault, err)
	}
}

func TestReadinessReceiptRejectsStaleAndEscapingFiles(t *testing.T) {
	p := executionbackend.ProbeIdentity{Execution: strings.Repeat("a", 32), Turn: strings.Repeat("b", 32), Nonce: strings.Repeat("c", 32), ProfileDigest: "digest", Runtime: "runtime", Terminal: "terminal", Incarnation: "incarnation", Worktree: t.TempDir()}
	_, digest, _, err := PrepareProbe(p)
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := probeDirectory(p)
	path := filepath.Join(p.Worktree, dir, "receipt.json")
	r := executionbackend.ProbeReceipt{ProbeIdentity: p, InputSHA256: digest, GitHead: strings.Repeat("d", 40), Read: true, Write: true, Delete: true}
	write := func(r executionbackend.ProbeReceipt) {
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

func TestProbeEvidenceAcceptsNativeExecPragmaWithoutChangingCommand(t *testing.T) {
	command, nonce, digest := "probe-command", "original-nonce", "original-script"
	input := "// @exec: {\"max_output_tokens\":2000}\nconst r = await tools.exec_command({cmd: \"probe-command\", login: false});\ntext(r);"
	result := `{"exit_code":0,"output":"SLREADINESS:original-nonce:original-script"}`
	transcript, _ := json.Marshal(map[string]any{"messages": []any{
		map[string]any{"id": "call", "source": "transcript", "role": "assistant", "blocks": []any{map[string]any{"type": "tool-call", "name": "exec", "callId": "original-call", "input": input}}},
		map[string]any{"id": "result", "source": "transcript", "role": "tool", "blocks": []any{map[string]any{"type": "tool-result", "callId": "original-call", "output": result}}},
	}})
	evidence, err := ObserveProbe(WorkerOutput{Source: "transcript", SourceExact: true, Provider: "codex", SourceIdentity: "original-source", Transcript: transcript}, command, nonce, digest, executionbackend.ProbeToolEvidence{})
	if err != nil || !evidence.Verified {
		t.Fatalf("native pragma lost tool evidence: %+v / %v", evidence, err)
	}
	for _, changed := range []string{
		strings.Replace(input, `"max_output_tokens":2000`, `"unknown":2000`, 1),
		strings.Replace(input, `"max_output_tokens":2000`, `"max_output_tokens":"2000"`, 1),
		strings.Replace(input, "text(r);", "text(fake);", 1),
		input + ` text({exit_code:0});`,
	} {
		if _, ok := commandInvocation("exec", changed); ok {
			t.Fatalf("altered native wrapper accepted: %s", changed)
		}
	}
}
