// Package runnerstatepath owns the caller's short, home-scoped Runner directory.
package runnerstatepath

import (
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/erranliu/SteerLine/internal/pathguard"
	httpfs "github.com/erranliu/SteerLine/internal/product/httpfs"
)

// Directory binds the application home
// to a 128-bit key. Home depth and repository names cannot lengthen this path.
// It performs no I/O and never adopts or moves an old Runner directory.
func Directory(home string) string {
	identity := "steerline/shared-runner/v1\x00" + filepath.Clean(home)
	root := filepath.Join(os.TempDir(), "slr")
	if runtime.GOOS == "windows" {
		identity = strings.ToLower(identity)
		root = filepath.Join(strings.ToUpper(filepath.VolumeName(home))+string(filepath.Separator), "slr")
	}
	sum := sha256.Sum256([]byte(identity))
	key := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:16]))
	return filepath.Join(root, key)
}

// Validate is read-only. Reject aliases rather than shortening paths through
// junctions/subst or redirecting another application's execution state.
func Validate(home string) error {
	if !filepath.IsAbs(home) {
		return errors.New("Runner state requires an absolute application home")
	}
	if runtime.GOOS == "windows" && len(filepath.VolumeName(home)) != 2 {
		return errors.New("Runner state requires a local drive application home")
	}
	dir := Directory(home)
	// Pinned Runner's Git/authentication suffix needs 64 bytes; AF_UNIX permits
	// 106. Keep that budget here, without relaxing Runner's own validation.
	if len(dir)+64 > 106 {
		return fmt.Errorf("Runner state directory exceeds authentication path budget: %s", dir)
	}
	resolved, err := pathguard.ResolveProspective(dir)
	if err != nil {
		return fmt.Errorf("resolve Runner state: %w", err)
	}
	if runtime.GOOS == "windows" {
		resolved, dir = strings.ToLower(resolved), strings.ToLower(dir)
	}
	if filepath.Clean(resolved) != filepath.Clean(dir) {
		return errors.New("Runner state must use a plain directory without redirected ancestors")
	}
	if info, err := os.Lstat(dir); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Runner state must be a plain directory")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Prepare is used only by explicit service startup. Read-only clients call
// Validate and do not create directories or change permissions.
func Prepare(home string) error {
	if err := Validate(home); err != nil {
		return err
	}
	return httpfs.EnsurePrivateDirectory(Directory(home))
}

// RemoveStopped removes only this home's derived Runner namespace.
// The caller must first prove the original service and Runs have stopped.
func RemoveStopped(home string) error {
	if err := Validate(home); err != nil {
		return err
	}
	target := Directory(home)
	if err := filepath.WalkDir(target, func(path string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		resolved, err := pathguard.ResolveProspective(path)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 || !pathguard.Within(target, resolved) {
			return fmt.Errorf("linked Runner cleanup path: %s", path)
		}
		return nil
	}); err != nil {
		return err
	}
	return os.RemoveAll(target)
}
