package testenv

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Only logs created in this isolated project are retained. Credentials/config
// and private SQLite files are not artifacts; known auth values are redacted.
func (p *Project) captureNativeDiagnostics() error {
	var secrets []string
	if token, err := os.ReadFile(filepath.Join(p.Home, "github-token")); err == nil {
		value := strings.TrimSpace(string(token))
		if value != "" {
			secrets = append(secrets, value, base64.StdEncoding.EncodeToString([]byte("x-access-token:"+value)))
		}
	}
	if auth, err := os.ReadFile(filepath.Join(p.Home, "codex", "auth.json")); err == nil {
		var data map[string]any
		if json.Unmarshal(auth, &data) == nil {
			var collect func(map[string]any)
			collect = func(m map[string]any) {
				for key, value := range m {
					if child, ok := value.(map[string]any); ok {
						collect(child)
					}
					if text, ok := value.(string); ok && (strings.Contains(strings.ToLower(key), "token") || key == "OPENAI_API_KEY") {
						secrets = append(secrets, text)
					}
				}
			}
			collect(data)
		}
	}
	roots := []string{filepath.Join(filepath.Dir(p.Database), "runs"), filepath.Join(p.Home, "codex", "log"), filepath.Join(p.Home, "codex", "sessions")}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || (filepath.Ext(path) != ".log" && filepath.Ext(path) != ".jsonl") {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Size() > 32<<20 {
				return errors.New("native diagnostic log exceeds bounded artifact size")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, secret := range secrets {
				if secret != "" {
					raw = []byte(strings.ReplaceAll(string(raw), secret, "[REDACTED]"))
				}
			}
			relative, err := filepath.Rel(p.Root, path)
			if err != nil {
				return err
			}
			// Orca logs are still beneath this project's own Codex home.
			target := filepath.Join(p.Scenario.Output, "native-logs", relative)
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			return os.WriteFile(target, raw, 0600)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
