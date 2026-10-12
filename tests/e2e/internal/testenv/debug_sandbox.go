package testenv

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
	"github.com/google/uuid"
)

// DebugSandbox uses the canonical generated package, with its own live catalog,
// authentication and exact GitHub base. Only selected evidence leaves the package.
func DebugSandbox(s *Scenario) {
	s.t.Helper()
	base := filepath.Join(`C:\SteerlineDebug`, "l3-"+uuid.NewString())
	s.Must(os.Mkdir(base, 0700))
	s.debugSandboxRoot = base
	s.RecordResource("debug-package-parent", base, 0)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(s.Run.Root, "scripts", "new-debug-release.ps1"), "-DebugRoot", base)
	cmd.Dir = s.Run.Root
	// A Go child does not perform pwsh's Windows PowerShell module-path
	// conversion. Let PowerShell 5 establish its own native module defaults.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, "PSModulePath") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	raw, err := s.RunCommandFor(cmd, "debug-package-generation", 3*time.Minute)
	s.Must(err)
	var release struct{ Root string }
	s.Must(json.Unmarshal(raw, &release))
	rel, err := filepath.Rel(base, release.Root)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || !filepath.IsAbs(release.Root) {
		s.t.Fatal("generated debug package escaped its owned parent")
	}
	s.RecordResource("debug-package", release.Root, 0)
	manifest, err := os.ReadFile(filepath.Join(release.Root, "manifest.json"))
	s.Must(err)
	var identity struct {
		SourceCommit string `json:"source_commit"`
		SourceDirty  bool   `json:"source_dirty"`
	}
	s.Must(json.Unmarshal(manifest, &identity))
	if identity.SourceCommit != s.Run.SourceCommit || fmt.Sprint(identity.SourceDirty) != s.Run.SourceDirty {
		s.t.Fatal("generated package source differs from the selected E2E build")
	}
	s.Must(os.WriteFile(filepath.Join(s.Output, "debug-manifest.json"), manifest, 0600))
	result, runErr := driver.RunDebugSandbox(s.Context, release.Root, s.Run.AuthHome)
	s.debugSandboxResult = result
	s.usageVerified = result["usage_verified"] == true
	s.Must(controlled.Write(filepath.Join(s.Output, "debug-sandbox.json"), result))
	// evidence/ contains only test logs, proofs and filtered native usage, not
	// CODEX_HOME, GH_CONFIG_DIR, authentication, databases or complete prompts.
	copyErr := archiveDebugSandbox(release.Root, s.Output)
	s.Must(errors.Join(runErr, copyErr))
	if result["passed"] != true || !s.usageVerified || result["cleanup_complete"] != true {
		s.t.Fatal("debug sandbox lacks complete native usage, delivery or cleanup")
	}
}

func archiveDebugSandbox(root, output string) error {
	if manifest, err := os.ReadFile(filepath.Join(root, "manifest.json")); err == nil {
		if err := os.WriteFile(filepath.Join(output, "debug-manifest.json"), manifest, 0600); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "evidence")); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(filepath.Join(root, "evidence"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.Join(root, "evidence"), path)
		if err != nil {
			return err
		}
		target := filepath.Join(output, "debug-evidence", rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0600)
	})
}

func (s *Scenario) cleanupDebugSandbox() error {
	if s.debugSandboxRoot == "" {
		return nil
	}
	// Also archive partial generation diagnostics before removing any package.
	entries, err := os.ReadDir(s.debugSandboxRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		root := filepath.Join(s.debugSandboxRoot, entry.Name())
		if err := archiveDebugSandbox(root, s.Output); err != nil {
			return err
		}
	}
	if s.debugSandboxResult != nil && s.debugSandboxResult["cleanup_complete"] != true {
		return errors.New("debug sandbox cleanup incomplete; preserve its owned package for inspection")
	}
	// This is the exact nonce parent created by DebugSandbox, verified against
	// C:\SteerlineDebug before deletion. Installation paths cannot be selected.
	resolved, err := filepath.EvalSymlinks(s.debugSandboxRoot)
	if err != nil {
		return err
	}
	if !strings.EqualFold(resolved, s.debugSandboxRoot) {
		return errors.New("refuse linked debug package cleanup")
	}
	return RemoveOwned(`C:\SteerlineDebug`, s.debugSandboxRoot)
}
