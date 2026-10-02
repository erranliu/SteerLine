package native

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// SeedOrcaHooks keeps Orca's real transcript reporting in an isolated home.
// Only the installed Orca hook payload and its existing trust hashes are used.
func SeedOrcaHooks(sourceHookPath, home string) error {
	raw, err := os.ReadFile(sourceHookPath)
	if err != nil {
		return err
	}
	var config struct {
		Hooks map[string][]struct{ Hooks []struct{ Command string } }
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return err
	}
	if len(config.Hooks) == 0 {
		return errors.New("Orca Codex hooks missing")
	}
	for _, groups := range config.Hooks {
		for _, group := range groups {
			for _, hook := range group.Hooks {
				if !strings.Contains(strings.ToLower(filepath.ToSlash(hook.Command)), "/.orca/agent-hooks/codex-hook.") {
					return errors.New("refuse non-Orca hook in native test environment")
				}
			}
		}
	}
	target := filepath.Join(home, "hooks.json")
	if err := os.WriteFile(target, raw, 0600); err != nil {
		return err
	}
	sourceConfig, err := os.ReadFile(filepath.Join(filepath.Dir(sourceHookPath), "config.toml"))
	if err != nil {
		return err
	}
	lines := strings.Split(string(sourceConfig), "\n")
	var sections strings.Builder
	selected := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			selected = strings.HasPrefix(line, "[hooks.state.") && strings.Contains(line, sourceHookPath+":")
		}
		if selected {
			sections.WriteString(strings.ReplaceAll(line, sourceHookPath, target))
			sections.WriteByte('\n')
		}
	}
	if sections.Len() == 0 {
		return errors.New("Orca native hook trust state missing")
	}
	f, err := os.OpenFile(filepath.Join(home, "config.toml"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.WriteString("\n" + sections.String())
	return errors.Join(err, f.Close())
}
