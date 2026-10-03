package steerline

import (
	"os"

	"github.com/erranliu/SteerLine/internal/steerline/activityview"
)

type RenderOptions = activityview.RenderOptions

func RenderActivity(ev ActivityEvent, o RenderOptions) string {
	return activityview.RenderActivity(ev, o)
}

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

// ColorEnabled reports whether ANSI color is appropriate for stdout. Watch and
// the service console honor NO_COLOR and only color an interactive terminal.
func ColorEnabled() bool { return stdoutColor() }

func isTerminal(f *os.File) bool {
	info, e := f.Stat()
	if e != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
