//go:build !windows

package httpfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func CreatePrivateTemp(dir, prefix string) (*os.File, error) {
	return os.CreateTemp(dir, prefix+"*.tmp")
}

func EnsurePrivateDirectory(path string) error {
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

func VerifyPrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("local API directory permissions are not private")
	}
	return nil
}

func SecurePrivateFile(path string) error {
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
	return VerifyPrivateFile(path)
}

func VerifyPrivateFile(path string) error {
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

func PrivatePath(root, target string) bool {
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
