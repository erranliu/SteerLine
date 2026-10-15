package runnerclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Selection is the caller's release pin, never a native adapter configuration.
type Selection struct {
	Executable string `json:"executable"`
	Version    string `json:"version"`
	SHA256     string `json:"sha256"`
}

func VerifyExecutable(path, expected string) error {
	want, err := hex.DecodeString(expected)
	if !filepath.IsAbs(path) || err != nil || len(want) != sha256.Size {
		return errors.New("Runner requires an absolute executable and release SHA-256")
	}
	file, err := os.Open(path)
	if err != nil {
		return errors.New("selected Runner executable is unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("selected Runner executable is not a regular file")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return errors.New("cannot verify selected Runner executable")
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expected) {
		return errors.New("selected Runner executable differs from its release SHA-256")
	}
	return nil
}

// Open verifies the offline version command. It never starts a service or reads
// capabilities, so opening a project cannot launch an execution process.
func (s Selection) Open(ctx context.Context, stateDir string, timeout time.Duration) (Client, Version, error) {
	if s.Version == "" || !filepath.IsAbs(stateDir) {
		return Client{}, Version{}, errors.New("Runner version and absolute project state directory are required")
	}
	if err := VerifyExecutable(s.Executable, s.SHA256); err != nil {
		return Client{}, Version{}, err
	}
	client := Client{Executable: s.Executable, StateDir: stateDir, Timeout: timeout, SHA256: s.SHA256}
	version, err := client.Version(ctx)
	if err != nil {
		return Client{}, Version{}, err
	}
	if version.Version != s.Version || version.ResourceVersion <= 0 || len(version.ResourceDigest) != 64 {
		return Client{}, Version{}, errors.New("selected Runner version/resource identity differs from the release contract")
	}
	if err := VerifyExecutable(s.Executable, s.SHA256); err != nil {
		return Client{}, Version{}, err
	}
	return client, version, nil
}
