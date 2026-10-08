package app

import (
	"fmt"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/pathguard"
)

func validateRuntimeHomePaths(config Config) error {
	home, err := resolveProspectiveApplicationPath(HomeDir())
	if err != nil {
		return fmt.Errorf("resolve application home: %w", err)
	}
	for _, entry := range []struct{ name, path string }{
		{"application bin", BinDir()},
		{"application etc", EtcDir()},
		{"execution profile", GlobalExecutionConfigPath()},
		{"state root", StateRoot()},
		{"run_dir", config.RunDir},
		{"database", filepath.Join(config.RunDir, "steerline.db")},
		{"database WAL", filepath.Join(config.RunDir, "steerline.db-wal")},
		{"database shared memory", filepath.Join(config.RunDir, "steerline.db-shm")},
		{"database journal", filepath.Join(config.RunDir, "steerline.db-journal")},
		{"control state", filepath.Join(config.RunDir, "control")},
		{"control requests", filepath.Join(config.RunDir, "control", "requests")},
		{"control receipts", filepath.Join(config.RunDir, "control", "receipts")},
		{"service state", filepath.Join(config.RunDir, "service")},
		{"service launch lock", filepath.Join(config.RunDir, "service", "launch.lock")},
		{"service startups", filepath.Join(config.RunDir, "service", "startups")},
		{"run directories", filepath.Join(config.RunDir, "runs")},
		{"managed worktrees", filepath.Join(config.RunDir, "worktrees")},
		{"worktree_root", config.WorktreeRoot},
	} {
		if entry.path == "" {
			continue
		}
		resolved, err := resolveProspectiveApplicationPath(entry.path)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", entry.name, err)
		}
		if !pathguard.Within(home, resolved) {
			return fmt.Errorf("%s must resolve inside STEERLINE_HOME (%s)", entry.name, home)
		}
	}
	return pathguard.RejectSharedSQLiteFiles(filepath.Join(config.RunDir, "steerline.db"))
}
