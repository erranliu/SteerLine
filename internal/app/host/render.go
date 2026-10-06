package host

import (
	"os"
)

// ConsoleDefault returns "pretty" when stdout is an interactive terminal, so a
// single `steerline run` shows live activity without a second process.
func ConsoleDefault() string {
	if isTerminal(os.Stdout) {
		return "pretty"
	}
	return ""
}

func stdoutColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isTerminal(os.Stdout)
}

func isTerminal(f *os.File) bool {
	info, e := f.Stat()
	if e != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
