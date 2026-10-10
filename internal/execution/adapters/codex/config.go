package codexworker

import (
	"fmt"
)

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
	WorkerRuntime   bool
}

func ValidReasoningEffort(value string) bool {
	switch value {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		return true
	}
	return false
}

func LocalReasoningEffortDefault(effort string) string {
	if effort == "" {
		return "high"
	}
	return effort
}

func (c Config) Validate(goos string) error {
	if c.Profile != "" {
		return fmt.Errorf("native profile overrides are unsupported")
	}
	permissions, err := NativePermissions(goos, "local", "codex")
	if err != nil {
		return err
	}
	if c.Sandbox != permissions.Sandbox || c.WindowsSandbox != permissions.WindowsSandbox {
		return fmt.Errorf("native Codex settings differ from the fixed native launch")
	}
	if c.ReasoningEffort != "" && !ValidReasoningEffort(c.ReasoningEffort) {
		return fmt.Errorf("invalid reasoning_effort %q", c.ReasoningEffort)
	}
	return ValidateManagedArgs(c.Args)
}
