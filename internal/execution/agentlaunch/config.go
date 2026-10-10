// Package agentlaunch compiles agent settings into an opaque terminal command.
// Transport adapters do not interpret model or engine flags.
package agentlaunch

import (
	"errors"
	"fmt"
	"strings"
)

// Config targets a PowerShell terminal on the execution host.
type Config struct {
	Executable      string   `json:"executable"`
	Args            []string `json:"args,omitempty"`
	Model           string   `json:"model,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`
}

func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	copy := *c
	copy.Args = append([]string(nil), c.Args...)
	return &copy
}
func quote(text string) string { return "'" + strings.ReplaceAll(text, "'", "''") + "'" }

func (c *Config) Command(agent string) (string, error) {
	if c == nil {
		return "", nil
	}
	if strings.TrimSpace(c.Executable) == "" || strings.ContainsAny(c.Executable+c.Model+c.ReasoningEffort, "\x00\r\n") {
		return "", errors.New("invalid agent launch executable or preferences")
	}
	args := append([]string(nil), c.Args...)
	for _, arg := range args {
		if strings.ContainsAny(arg, "\x00\r\n") {
			return "", errors.New("invalid agent launch argument")
		}
	}
	if c.Model != "" || c.ReasoningEffort != "" {
		if agent != "codex" {
			return "", errors.New("model preference compiler is unavailable for this agent")
		}
		if c.Model == "" {
			return "", errors.New("reasoning effort requires a model")
		}
		switch c.ReasoningEffort {
		case "", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		default:
			return "", errors.New("invalid reasoning effort")
		}
		for _, arg := range args {
			if arg == "--" || strings.HasPrefix(arg, "-m") && !strings.HasPrefix(arg, "--") || arg == "--model" || strings.HasPrefix(arg, "--model=") || strings.HasPrefix(arg, "-c") && !strings.HasPrefix(arg, "--") || arg == "--config" || strings.HasPrefix(arg, "--config=") {
				return "", errors.New("agent arguments cannot override declared model preferences")
			}
		}
		args = append(args, "--model", c.Model)
		if c.ReasoningEffort != "" {
			args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", c.ReasoningEffort))
		}
	}
	if agent == "codex" {
		// Animated repaints prevent the host terminal's generic quiet/idle
		// condition from settling. This is an agent-owned, session-only option.
		args = append(args, "-c", "tui.animations=false")
		// Establish the native session before transport waits for its identity.
		// This readiness input authorizes no business work, tools or file changes.
		args = append(args, "Initialize this agent session. Reply READY only. Do not use tools or change files.")
	}
	parts := []string{"&", quote(c.Executable)}
	for _, arg := range args {
		parts = append(parts, quote(arg))
	}
	return strings.Join(parts, " "), nil
}
func (c *Config) Display() []string {
	if c == nil {
		return nil
	}
	lines := []string{"代理启动：通用命令（覆盖原生默认启动）· 执行主机 PowerShell · 权限由启动参数与主机设置决定"}
	if c.Model != "" {
		lines = append(lines, "Model："+c.Model)
	}
	if c.ReasoningEffort != "" {
		lines = append(lines, "Reasoning effort："+c.ReasoningEffort)
	}
	return lines
}
