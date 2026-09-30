package codexworker

import "fmt"

// Config is the native implementation input assembled from frozen records.
// It is never embedded in contracts or read from mutable project state.
type Config struct {
	Executable      string
	Args            []string
	Profile         string
	Sandbox         string
	WindowsSandbox  string
	Model           string
	ReasoningEffort string
	Workspace       string
	RunDir          string
	RunID           string
	ServerDir       string
	Environment     func([]string) []string
}

func ValidReasoningEffort(value string) bool {
	switch value {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		return true
	}
	return false
}
func ValidSandbox(value string) bool {
	return value == "read-only" || value == "workspace-write" || value == "danger-full-access"
}
func LocalWindowsSandboxDefault(goos, sandbox, mode string) string {
	if goos == "windows" && sandbox != "danger-full-access" && mode == "" {
		return "elevated"
	}
	return mode
}
func LocalReasoningEffortDefault(effort string) string {
	if effort == "" {
		return "high"
	}
	return effort
}
func (c Config) Validate(goos string) error {
	if !ValidSandbox(c.Sandbox) {
		return fmt.Errorf("unsupported Codex sandbox %q", c.Sandbox)
	}
	if c.ReasoningEffort != "" && !ValidReasoningEffort(c.ReasoningEffort) {
		return fmt.Errorf("invalid reasoning_effort %q", c.ReasoningEffort)
	}
	_, err := LocalSandboxArgs(goos, c.Sandbox, c.WindowsSandbox, c.Args)
	return err
}

func ValidWindowsSandbox(mode string) bool { return mode == "elevated" || mode == "unelevated" }
