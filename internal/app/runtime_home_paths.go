package app

import "fmt"

func validateRuntimeHomePaths(config Config) error {
	home, err := resolveProspectiveApplicationPath(HomeDir())
	if err != nil {
		return fmt.Errorf("resolve application home: %w", err)
	}
	for _, entry := range []struct{ name, path string }{
		{"run_dir", config.RunDir},
		{"worktree_root", config.WorktreeRoot},
	} {
		resolved, err := resolveProspectiveApplicationPath(entry.path)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", entry.name, err)
		}
		if !applicationPathInside(home, resolved) {
			return fmt.Errorf("%s must resolve inside STEERLINE_HOME (%s)", entry.name, home)
		}
	}
	return nil
}
