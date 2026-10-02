package native

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The live environment pins codex-cli 0.159.2, whose native setup version is 5.
// Read existing state only: test execution must not provision accounts via UAC.
func sandboxState(home string) (map[string][]byte, error) {
	files := make(map[string][]byte)
	for _, path := range []string{filepath.Join(".sandbox", "setup_marker.json"), filepath.Join(".sandbox-secrets", "sandbox_users.json")} {
		raw, err := os.ReadFile(filepath.Join(home, path))
		if err != nil {
			return nil, fmt.Errorf("read existing Windows sandbox state: %w", err)
		}
		files[path] = raw
	}
	var marker struct {
		Version int    `json:"version"`
		Offline string `json:"offline_username"`
		Online  string `json:"online_username"`
	}
	var users sandboxUsers
	if json.Unmarshal(files[filepath.Join(".sandbox", "setup_marker.json")], &marker) != nil || json.Unmarshal(files[filepath.Join(".sandbox-secrets", "sandbox_users.json")], &users) != nil {
		return nil, errors.New("invalid Windows sandbox state")
	}
	if marker.Version != 5 || users.Version != 5 || marker.Offline != "CodexSandboxOffline" || marker.Online != "CodexSandboxOnline" || users.Offline.Username != marker.Offline || users.Online.Username != marker.Online || users.Offline.Password == "" || users.Online.Password == "" {
		return nil, errors.New("Windows sandbox state does not match pinned Codex setup")
	}
	if err := validateSandboxAccounts(users); err != nil {
		return nil, err
	}
	return files, nil
}

type sandboxAccount struct {
	Username string `json:"username"`
	Password string `json:"password"` // Native DPAPI blob; never log this record.
}

type sandboxUsers struct {
	Version int            `json:"version"`
	Offline sandboxAccount `json:"offline"`
	Online  sandboxAccount `json:"online"`
}

// CheckSandboxReady verifies existing credentials without starting Codex,
// requesting elevation, changing accounts or making model requests.
func CheckSandboxReady(home string) error {
	_, err := sandboxState(home)
	return err
}
