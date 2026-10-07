package codexworker

import (
	"encoding/json"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestPS2RejectsAllRetiredCodexOverrides(t *testing.T) {
	for _, field := range []string{"args", "profile", "windows_sandbox", "sandbox", "sandbox_mode", "approval_policy", "approvals_reviewer"} {
		for _, value := range []any{nil, "", []string{}, map[string]any{}} {
			raw, err := json.Marshal(map[string]any{"executable": "must-not-run", field: value})
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := NormalizeProfilePayload(raw, "windows", "local", "codex"); err == nil || !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("retired %s=%v accepted: %v", field, value, err)
			}
		}
	}
}

func TestPS2NativeResourceRejectsApprovalChanges(t *testing.T) {
	identity := PolicyResourceIdentity{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Generation: "generation", ConfigDigest: "profile"}
	permissions, err := NativePermissions("windows", "local", "codex")
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func([]string) []string{
		func(v []string) []string { return v },
		func(v []string) []string { return append(v, "--config", "approval_policy=never") },
		func(v []string) []string { return append(v, "--sandbox", "danger-full-access") },
		func(v []string) []string { return nil },
	} {
		args := mutate(permissions.Args())
		envelope, err := NewPolicyResourceEnvelope(identity, PolicyResourcePayload{NativeVersion: PolicyCodexVersion, Contract: PolicyContract, Settings: executionbackend.LaunchCommand{Executable: "codex.exe", Args: args, WorkingDirectory: "candidate"}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = DecodePolicyResourceEnvelope(envelope, identity)
		valid := len(args) == len(permissions.Args())
		if (err == nil) != valid {
			t.Fatal("current resource accepted invalid native launch settings", args, err)
		}
	}
}

func TestPS2NativeLaunchAndInputUseAutomaticReview(t *testing.T) {
	for _, host := range []string{"local", "orca"} {
		permissions, err := NativePermissions("windows", host, "codex")
		if err != nil {
			t.Fatal(err)
		}
		args := strings.Join(permissions.Args(), " ")
		for _, setting := range []string{"sandbox_mode=workspace-write", "windows.sandbox=unelevated", "approval_policy=on-request", "approvals_reviewer=auto_review", "network_access=true", "exclude_tmpdir_env_var=true", "exclude_slash_tmp=true"} {
			if !strings.Contains(args, setting) {
				t.Fatal("missing", setting, args)
			}
		}
		for _, forbidden := range []string{"=elevated", "danger-full-access", "approval_policy=never"} {
			if strings.Contains(args, forbidden) {
				t.Fatal("unexpected fallback", args)
			}
		}
	}
	thread, err := ThreadStartParams("candidate", "workspace-write", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := TurnStartParams("candidate", "thread", "command", "prompt", "workspace-write", "", "", []string{"candidate/.git/admin"})
	if err != nil {
		t.Fatal(err)
	}
	for _, params := range []map[string]any{thread, turn} {
		if params["approvalPolicy"] != "on-request" || params["approvalsReviewer"] != "auto_review" {
			t.Fatal("input changed native reviewer", params)
		}
	}
	for _, target := range [][3]string{{"linux", "local", "codex"}, {"windows", "orca_supervised", "codex"}, {"windows", "orca", "claude"}} {
		if _, err := NativePermissions(target[0], target[1], target[2]); err == nil {
			t.Fatal("misapplied native settings", target)
		}
	}
}
