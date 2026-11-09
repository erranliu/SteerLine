package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestProjectEnvironmentScriptReportsToolsWithoutChainChecks(t *testing.T) {
	powershell, err := exec.LookPath("powershell.exe")
	require(t, err)
	script, err := filepath.Abs("../../SteerlineFlow/scripts/verify-environment.ps1")
	require(t, err)
	root := t.TempDir()
	tools := filepath.Join(root, "tools with spaces")
	require(t, os.MkdirAll(tools, 0700))
	self, err := os.Executable()
	require(t, err)
	selfBytes, err := os.ReadFile(self)
	require(t, err)
	for _, name := range []string{"git.exe", "go.exe", "node.exe", "python.exe"} {
		require(t, os.WriteFile(filepath.Join(tools, name), selfBytes, 0700))
	}
	require(t, os.WriteFile(filepath.Join(tools, "npm.cmd"), []byte("@echo off\r\necho 10.0.0\r\n"), 0600))
	var previousDetails any
	for _, mode := range []string{"codex", "orca", "missing", "low", "error", "timeout", "requirements", "invalid-minimum", "changed-minimum", "crlf", "close-at-eof", "inline-fence", "closing-suffix"} {
		t.Run(mode, func(t *testing.T) {
			work := t.TempDir()
			document := "```steerline-environment\n" + `{"version":6,"requirements":{"go":"1.26.0","node":"24.0","python":"3.0"}}` + "\n```\n"
			if mode == "requirements" {
				document = "invalid requirements"
			}
			if mode == "invalid-minimum" {
				document = strings.Replace(document, `"go":"1.26.0"`, `"go":""`, 1)
			}
			if mode == "changed-minimum" {
				document = strings.Replace(document, `"go":"1.26.0"`, `"go":"99.0"`, 1)
			}
			switch mode {
			case "crlf":
				document = strings.ReplaceAll(document, "\n", "\r\n")
			case "close-at-eof":
				document = strings.TrimSuffix(document, "\n")
			case "inline-fence":
				document = "inline " + document
			case "closing-suffix":
				document = strings.TrimSuffix(document, "\n") + "suffix\n"
			}
			require(t, os.MkdirAll(filepath.Join(work, "SteerlineFlow"), 0700))
			require(t, os.WriteFile(filepath.Join(work, "SteerlineFlow", "environment.md"), []byte(document), 0600))
			initial := map[string]any{"version": float64(3), "project_identity": "project-测试", "environment_key": mode, "activity_id": "activity", "round": float64(1), "call_context": map[string]any{"adapter": mode, "target": map[string]any{"kind": "local", "environment": ""}}, "source_commit": "commit", "inputs_digest": "basis", "completed": false, "ready": false, "message": ""}
			raw, err := json.Marshal(initial)
			require(t, err)
			result := filepath.Join(work, "result.json")
			require(t, os.WriteFile(result, raw, 0600))
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, powershell, "-NoProfile", "-NonInteractive", "-File", script)
			cmd.Dir = work
			cmd.Env = os.Environ()
			toolPath := tools
			if mode == "missing" {
				toolPath = work
			}
			cmd.Env = environmentScriptTestEnv(cmd.Env, "PATH", toolPath)
			cmd.Env = environmentScriptTestEnv(cmd.Env, "STEERLINE_RESULT_FILE", result)
			cmd.Env = environmentScriptTestEnv(cmd.Env, "STEERLINE_TEST_ENVIRONMENT_TOOL", mode)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("script call failed: %v\n%s", err, output)
			}
			raw, err = os.ReadFile(result)
			require(t, err)
			var final map[string]any
			require(t, json.Unmarshal(raw, &final))
			for key, value := range initial {
				if key != "completed" && key != "ready" && key != "message" && !reflect.DeepEqual(value, final[key]) {
					t.Fatal("script rewrote invocation identity", key, value, final[key])
				}
			}
			wantReady := mode == "codex" || mode == "orca" || mode == "crlf" || mode == "close-at-eof"
			if final["completed"] != true || final["ready"] != wantReady || final["message"] == "" || strings.Contains(string(raw), "untrusted tool output") {
				t.Fatal(string(raw))
			}
			for failure, reason := range map[string]string{"missing": "missing from PATH", "low": "below required", "error": "query failed", "timeout": "timed out", "requirements": "Cannot read", "invalid-minimum": "Cannot read", "changed-minimum": "below required 99.0", "inline-fence": "Cannot read", "closing-suffix": "Cannot read"} {
				if mode == failure && !strings.Contains(string(raw), reason) {
					t.Fatal("lost diagnostic reason", string(raw))
				}
			}
			if wantReady {
				if previousDetails != nil && !reflect.DeepEqual(previousDetails, final["details"]) {
					t.Fatal("chain context changed project detection standards")
				}
				previousDetails = final["details"]
			}
			leftovers, err := filepath.Glob(result + ".*.tmp")
			require(t, err)
			if len(leftovers) != 0 {
				t.Fatal("atomic result temporary files left behind", leftovers)
			}
		})
	}
}

func environmentScriptTestEnv(values []string, key, value string) []string {
	out := make([]string, 0, len(values)+1)
	for _, item := range values {
		name, _, _ := strings.Cut(item, "=")
		if !strings.EqualFold(name, key) {
			out = append(out, item)
		}
	}
	return append(out, key+"="+value)
}
