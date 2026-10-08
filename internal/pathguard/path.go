package pathguard

import (
	"path/filepath"
	"runtime"
	"strings"
)

// Within reports whether target is contained by root after both paths have
// been resolved with ResolveProspective.
func Within(root, target string) bool {
	resolvedRoot, err := ResolveProspective(root)
	if err != nil {
		return false
	}
	resolvedTarget, err := ResolveProspective(target)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		resolvedRoot = strings.ToLower(resolvedRoot)
		resolvedTarget = strings.ToLower(resolvedTarget)
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedTarget)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
