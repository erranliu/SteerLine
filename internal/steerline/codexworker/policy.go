package codexworker

import (
	"fmt"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

const PolicyContract = "codex-policy-v1:0.159.2:explicit-roots:no-temp:never"

const FullAccessPolicyContract = "codex-policy-v1:0.159.2:host-write:network-allow:never"

const PolicyCodexVersion = "codex-cli 0.159.2"

type Permissions struct {
	Sandbox        string `json:"sandbox"`
	WindowsSandbox string `json:"windows_sandbox"`
	NetworkAccess  bool   `json:"network_access"`
	ApprovalPolicy string `json:"approval_policy"`
	Contract       string `json:"contract"`
}

// CompilePolicy rejects unsupported combinations before any native operation.
// Version verification is separate from settings and actual worker tool proof.
func CompilePolicy(p executionbackend.ExecutionPolicy, goos, host, agent, windowsMode, profile string, args []string) (Permissions, error) {
	if err := p.Validate(); err != nil {
		return Permissions{}, err
	}
	if profile != "" {
		return Permissions{}, fmt.Errorf("native Codex profile overrides are unsupported")
	}
	if err := ValidateManagedArgs(args); err != nil {
		return Permissions{}, err
	}
	if p.Network == "deny" {
		for _, arg := range args {
			if arg == "--search" {
				return Permissions{}, fmt.Errorf("execution_policy.network=deny does not permit hosted live web search")
			}
		}
	}
	if host != "local" && host != "orca" || agent != "codex" {
		return Permissions{}, fmt.Errorf("target %s/%s has no verified mapping for requested execution_policy", host, agent)
	}
	if windowsMode != "" && !ValidWindowsSandbox(windowsMode) {
		return Permissions{}, fmt.Errorf("codex.windows_sandbox must be elevated or unelevated")
	}
	if goos == "windows" && p.Filesystem.WriteScope == "host" && p.Network == "allow" {
		if windowsMode != "" {
			return Permissions{}, fmt.Errorf("host write_scope does not use codex.windows_sandbox")
		}
		return Permissions{Sandbox: "danger-full-access", NetworkAccess: true, ApprovalPolicy: "never", Contract: FullAccessPolicyContract}, nil
	}
	if goos == "windows" && host == "local" && p.Filesystem.WriteScope == "none" && p.Network == "deny" {
		windowsMode = LocalWindowsSandboxDefault(goos, "read-only", windowsMode)
		return Permissions{Sandbox: "read-only", WindowsSandbox: windowsMode, ApprovalPolicy: "never", Contract: PolicyContract}, nil
	}
	// Only the mandatory Windows combination has native tool evidence in v1.
	if goos != "windows" || p.Filesystem.WriteScope != "workspace" || !p.Filesystem.GitMetadataWrite || p.Network != "allow" {
		return Permissions{}, fmt.Errorf("requested execution_policy has no verified mapping for %s/%s (requires Windows, workspace, git_metadata_write=true, network=allow)", goos, host)
	}
	if host == "local" {
		windowsMode = LocalWindowsSandboxDefault(goos, "workspace-write", windowsMode)
	}
	if host == "orca" && windowsMode == "" {
		return Permissions{}, fmt.Errorf("Orca requires explicit codex.windows_sandbox")
	}
	if host == "local" {
		if _, err := LocalSandboxArgs(goos, "workspace-write", windowsMode, args); err != nil {
			return Permissions{}, err
		}
	}
	return Permissions{Sandbox: "workspace-write", WindowsSandbox: windowsMode, NetworkAccess: true, ApprovalPolicy: "never", Contract: PolicyContract}, nil
}

func VerifyPolicyVersion(version string) error {
	if strings.TrimSpace(version) != PolicyCodexVersion {
		return fmt.Errorf("execution_policy native mapping requires %s; observed %s", PolicyCodexVersion, strings.TrimSpace(version))
	}
	return nil
}

func (p Permissions) Args() []string {
	args := []string{"--config", "sandbox_mode=" + p.Sandbox, "--config", "approval_policy=never"}
	if p.WindowsSandbox != "" {
		args = append(args, "--config", "windows.sandbox="+p.WindowsSandbox)
	}
	if !p.NetworkAccess {
		args = append(args, "--config", `web_search="disabled"`)
	}
	if p.Sandbox == "workspace-write" {
		args = append(args, "--config", "sandbox_workspace_write.writable_roots=[]", "--config", "sandbox_workspace_write.exclude_tmpdir_env_var=true", "--config", "sandbox_workspace_write.exclude_slash_tmp=true", "--config", "sandbox_workspace_write.network_access=true")
	}
	return args
}
