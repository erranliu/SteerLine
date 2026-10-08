//go:build !windows

package httpapi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("local API directory is not a real directory")
	}
	if err = os.Chmod(path, 0700); err != nil {
		return err
	}
	info, err = os.Stat(path)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("local API directory permissions are not private")
	}
	return nil
}

func verifyPrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("local API directory permissions are not private")
	}
	return nil
}

func securePrivateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("local API credential is not a regular file")
	}
	if err = os.Chmod(path, 0600); err != nil {
		return err
	}
	return verifyPrivateFile(path)
}

func verifyPrivateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("local API credential permissions are not private")
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Nlink != 1 {
		return fmt.Errorf("local API credential has multiple hard links")
	}
	return nil
}

func privatePath(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !hasParentTraversal(rel)
}

func hasParentTraversal(path string) bool {
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == ".." {
			return true
		}
	}
	return false
}
