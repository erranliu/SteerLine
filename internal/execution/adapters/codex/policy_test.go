package codexworker

import (
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func policyFixture() executionbackend.ExecutionPolicy {
	return executionbackend.ExecutionPolicy{Filesystem: executionbackend.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"}
}

func TestExplicitHostPolicyMapsBothWindowsWorkersToFullAccess(t *testing.T) {
	policy := policyFixture()
	policy.Filesystem.WriteScope, policy.Filesystem.GitMetadataWrite = "host", false
	for _, host := range []string{"local", "orca"} {
		permissions, err := CompilePolicy(policy, "windows", host, "codex", "", "", nil)
		if err != nil || permissions.Sandbox != "danger-full-access" || permissions.WindowsSandbox != "" || permissions.ApprovalPolicy != "never" || !permissions.NetworkAccess || permissions.Contract != FullAccessPolicyContract {
			t.Fatalf("full-access mapping missing for %s: %+v/%v", host, permissions, err)
		}
		args := strings.Join(permissions.Args(), " ")
		if strings.Contains(args, "windows.sandbox") || strings.Contains(args, "sandbox_workspace_write") {
			t.Fatal("full access injected sandbox initialization/settings")
		}
		params, err := TurnStartParams("worktree", "session", "command", "prompt", permissions.Sandbox, "model", "medium", nil)
		if err != nil || params["sandboxPolicy"].(map[string]any)["type"] != "dangerFullAccess" || params["approvalPolicy"] != "never" {
			t.Fatal("turn policy differs from compiled full-access launch")
		}
		if host == "orca" {
			launch, err := ManagedCommand(ManagedCommandOptions{Policy: policy, Host: host, Agent: "codex", Executable: "codex.exe", Sandbox: permissions.Sandbox})
			if err != nil || !strings.Contains(strings.Join(launch.Args, " "), "sandbox_mode=danger-full-access") {
				t.Fatalf("managed Orca launch lost full access: %+v/%v", launch, err)
			}
		}
		if _, err := CompilePolicy(policy, "windows", host, "codex", "elevated", "", nil); err == nil {
			t.Fatal("full access claimed a Windows sandbox implementation")
		}
	}
	policy.Network = "deny"
	if _, err := CompilePolicy(policy, "windows", "local", "codex", "", "", nil); err == nil {
		t.Fatal("network deny was silently widened to full access")
	}
}

func TestProjectPolicyCompilerSupportsOnlyVerifiedTargetMappings(t *testing.T) {
	for _, tc := range []struct {
		host, mode string
		ok         bool
	}{
		{"local", "", true}, {"local", "elevated", true}, {"local", "unelevated", false}, {"orca", "unelevated", true}, {"orca", "elevated", true}, {"orca", "", false}, {"orca_supervised", "", false},
	} {
		t.Run(tc.host+"-"+tc.mode, func(t *testing.T) {
			p, err := CompilePolicy(policyFixture(), "windows", tc.host, "codex", tc.mode, "", nil)
			if (err == nil) != tc.ok {
				t.Fatalf("unexpected support: %+v / %v", p, err)
			}
			if tc.ok && (p.Sandbox != "workspace-write" || p.ApprovalPolicy != "never" || !p.NetworkAccess || p.Contract != PolicyContract) {
				t.Fatalf("policy not managed: %+v", p)
			}
		})
	}
	p := policyFixture()
	p.Filesystem.WriteScope = "none"
	p.Filesystem.GitMetadataWrite = false
	p.Network = "deny"
	compiled, err := CompilePolicy(p, "windows", "local", "codex", "unelevated", "", nil)
	if err != nil || compiled.Sandbox != "read-only" || compiled.NetworkAccess {
		t.Fatalf("explicit local read-only denied: %+v/%v", compiled, err)
	}
	for _, mutate := range []func(*executionbackend.ExecutionPolicy){func(p *executionbackend.ExecutionPolicy) { p.Network = "deny" }, func(p *executionbackend.ExecutionPolicy) { p.Filesystem.GitMetadataWrite = false }, func(p *executionbackend.ExecutionPolicy) {
		p.Filesystem.WriteScope = "host"
		p.Filesystem.GitMetadataWrite = false
		p.Network = "deny"
	}} {
		p := policyFixture()
		mutate(&p)
		if _, err := CompilePolicy(p, "windows", "local", "codex", "", "", nil); err == nil {
			t.Fatalf("unsupported policy was widened: %+v", p)
		}
	}
	for _, target := range []struct{ os, agent string }{{"linux", "codex"}, {"windows", "pi"}} {
		if _, err := CompilePolicy(policyFixture(), target.os, "orca", target.agent, "unelevated", "", nil); err == nil {
			t.Fatal("unproven target accepted")
		}
	}
	if VerifyPolicyVersion("codex-cli 0.159.2") != nil || VerifyPolicyVersion("codex-cli 0.159.3") == nil {
		t.Fatal("mapping was not bound to native version")
	}
}

func TestProjectPolicyRejectsAllManagedOverridesBeforeNativeCalls(t *testing.T) {
	for _, host := range []string{"local", "orca"} {
		for _, args := range [][]string{{"-c", "sandbox_mode='danger-full-access'"}, {"--config=sandbox_workspace_write.network_access=true"}, {"-a", "on-request"}, {"--ask-for-approval=never"}, {"-sworkspace-write"}, {"--sandbox", "read-only"}, {"--add-dir", "C:/"}, {"--add-dir=C:/"}, {"-C", "C:/"}, {"--cd=C:/"}, {"--model", "x"}, {"-mx"}, {"--profile=x"}, {"-p", "x"}, {"--enable=foo"}, {"--dangerously-bypass-approvals-and-sandbox"}, {"exec"}, {"--unknown"}} {
			if _, err := CompilePolicy(policyFixture(), "windows", host, "codex", "elevated", "", args); err == nil {
				t.Fatalf("%s override accepted: %v", host, args)
			}
		}
		if _, err := CompilePolicy(policyFixture(), "windows", host, "codex", "elevated", "native-profile", nil); err == nil || !strings.Contains(err.Error(), "profile") {
			t.Fatal("native profile injection accepted")
		}
	}
}

func TestNetworkDenyRejectsLiveSearchAndDisablesInheritedSearch(t *testing.T) {
	policy := policyFixture()
	policy.Filesystem.WriteScope, policy.Filesystem.GitMetadataWrite, policy.Network = "none", false, "deny"
	if _, err := CompilePolicy(policy, "windows", "local", "codex", "unelevated", "", []string{"--search"}); err == nil {
		t.Error("network deny accepted hosted live search")
	}
	permissions, err := CompilePolicy(policy, "windows", "local", "codex", "unelevated", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(permissions.Args(), " "), `web_search="disabled"`) {
		t.Error("network deny inherited the user's hosted search configuration")
	}
}
