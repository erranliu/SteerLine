package orca

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// Bootstrap identity fences local pairing without returning its authentication.
func LocalRuntimeID() (string, error) {
	dir, err := stateUserDataDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, "orca-runtime.json"))
	var metadata struct {
		RuntimeID string `json:"runtimeId"`
	}
	if err != nil || json.Unmarshal(data, &metadata) != nil || metadata.RuntimeID == "" {
		return "", errors.New("local Orca runtime identity is unavailable")
	}
	return metadata.RuntimeID, nil
}

type StateRPCError struct {
	Code    string
	Message string
}

func (e *StateRPCError) Error() string { return e.Code + ": " + e.Message }

func stateUserDataDir() (string, error) {
	dir := os.Getenv("ORCA_USER_DATA_PATH")
	if dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "windows":
		if os.Getenv("APPDATA") == "" {
			return "", errors.New("Orca environment authentication requires APPDATA")
		}
		return filepath.Join(os.Getenv("APPDATA"), "orca"), nil
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "orca"), nil
	default:
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, "orca"), nil
	}
}
