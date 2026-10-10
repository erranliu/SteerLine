package codexworker

import (
	"fmt"
	"strings"
)

const PolicyContract = "codex-policy-v3:0.159.2:unelevated:explicit-roots:admin-temp:native-shell:git-no-maintenance:auto_review"
const PolicyCodexVersion = "codex-cli 0.159.2"

type Permissions struct {
	Sandbox           string `json:"sandbox"`
	WindowsSandbox    string `json:"windows_sandbox"`
	NetworkAccess     bool   `json:"network_access"`
	ApprovalPolicy    string `json:"approval_policy"`
	ApprovalsReviewer string `json:"approvals_reviewer"`
	Contract          string `json:"contract"`
}

// NativePermissions describes this adapter's single supported Windows launch.
// Other targets retain their own adapters and native admission checks.
func NativePermissions(goos, host, agent string) (Permissions, error) {
	if goos != "windows" || (host != "local" && host != "orca") || agent != "codex" {
		return Permissions{}, fmt.Errorf("fixed Codex Windows launch is unsupported for %s/%s/%s", goos, host, agent)
	}
	return Permissions{Sandbox: "workspace-write", WindowsSandbox: "unelevated", NetworkAccess: true, ApprovalPolicy: "on-request", ApprovalsReviewer: "auto_review", Contract: PolicyContract}, nil
}
func VerifyPolicyVersion(version string) error {
	if strings.TrimSpace(version) != PolicyCodexVersion {
		return fmt.Errorf("native Codex launch requires %s; observed %s", PolicyCodexVersion, strings.TrimSpace(version))
	}
	return nil
}
func (p Permissions) Args() []string {
	return []string{"--config", "sandbox_mode=" + p.Sandbox, "--config", "windows.sandbox=" + p.WindowsSandbox,
		"--config", "allow_login_shell=false",
		"--config", "approval_policy=" + p.ApprovalPolicy, "--config", "approvals_reviewer=" + p.ApprovalsReviewer,
		"--config", "sandbox_workspace_write.writable_roots=[]", "--config", "sandbox_workspace_write.exclude_tmpdir_env_var=true",
		"--config", "sandbox_workspace_write.exclude_slash_tmp=true", "--config", "sandbox_workspace_write.network_access=true"}
}
