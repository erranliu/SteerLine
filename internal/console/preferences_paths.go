package console

import (
	"path/filepath"
	"runtime"
	"strings"
)

func consolePathWithin(root, target string) bool {
	resolvedRoot, err := resolveConsolePath(root)
	if err != nil {
		return false
	}
	resolvedTarget, err := resolveConsolePath(target)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		resolvedRoot = strings.ToLower(resolvedRoot)
		resolvedTarget = strings.ToLower(resolvedTarget)
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedTarget)
	return err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}
