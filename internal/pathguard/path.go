package pathguard

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// RejectHardLinkedFile prevents a caller-owned evidence directory from reading
// another name for an external file. Missing files are handled by the caller.
func RejectHardLinkedFile(path string) error {
	links, err := fileLinkCount(path)
	if err != nil {
		return err
	}
	if links > 1 {
		return fmt.Errorf("file must not be a hard link: %s", path)
	}
	return nil
}

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
