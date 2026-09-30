package codexworker

import (
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"strings"
	"testing"
)

func policyFixture() executionapi.ExecutionPolicy {
	return executionapi.ExecutionPolicy{Filesystem: executionapi.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"}
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
	for _, mutate := range []func(*executionapi.ExecutionPolicy){func(p *executionapi.ExecutionPolicy) { p.Network = "deny" }, func(p *executionapi.ExecutionPolicy) { p.Filesystem.GitMetadataWrite = false }, func(p *executionapi.ExecutionPolicy) {
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
