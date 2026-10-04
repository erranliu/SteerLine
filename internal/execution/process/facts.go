// Package process owns bounded subprocess effects and read-only OS process facts.
package process

import (
	"path/filepath"
	"runtime"
	"strings"
)

type ProbeResult struct{ State, Reason string }

func canonical(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}
