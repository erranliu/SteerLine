package app

import (
	"fmt"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/pathguard"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
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
		{"local API state", filepath.Join(config.RunDir, "local-api")},
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
	if err := runnerstatepath.Validate(config.RunDir); err != nil {
		return err
	}
	return pathguard.RejectSharedSQLiteFiles(filepath.Join(config.RunDir, "steerline.db"))
}
