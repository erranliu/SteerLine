package orca

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

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
