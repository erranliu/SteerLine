package app

import (
	"os"
	"runtime/debug"

	"github.com/erranliu/SteerLine/internal/pathguard"
)

var buildSourceCommit = "unknown"
var buildSourceDirty = "unknown"

// RuntimeIdentity describes the executable and source build that is running.
// Build values stay unknown when Go did not embed them.
type RuntimeIdentity struct {
	Executable   string
	Home         string
	Config       string
	SourceCommit string
	SourceDirty  string
}

func CurrentRuntimeIdentity(configPath string) RuntimeIdentity {
	identity := RuntimeIdentity{
		Executable:   resolvedRuntimePath(executablePath()),
		Home:         resolvedRuntimePath(HomeDir()),
		Config:       resolvedRuntimePath(configPath),
		SourceCommit: buildSourceCommit,
		SourceDirty:  buildSourceDirty,
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				if identity.SourceCommit == "unknown" && setting.Value != "" {
					identity.SourceCommit = setting.Value
				}
			case "vcs.modified":
				if identity.SourceDirty == "unknown" && (setting.Value == "true" || setting.Value == "false") {
					identity.SourceDirty = setting.Value
				}
			}
		}
	}
	return identity
}

func executablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return path
}

func resolvedRuntimePath(path string) string {
	if path == "" {
		return "unknown"
	}
	resolved, err := resolveAbsolutePath(path)
	if err != nil {
		return "unknown"
	}
	return resolved
}

func resolveAbsolutePath(path string) (string, error) {
	return pathguard.ResolveProspective(path)
}
