package driver

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const SupportedCodexVersion = "codex-cli 0.159.2"

// Tool discovery is a native test precondition, not a Runner configuration codec.
func CodexExecutable(command string) (string, error) {
	if configured := os.Getenv("STEERLINE_NATIVE_CODEX_EXE"); configured != "" {
		command = configured
	}
	if command == "" {
		command = "codex"
	}
	path, err := exec.LookPath(command)
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" {
		return path, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	var magic [2]byte
	_, readErr := file.Read(magic[:])
	file.Close()
	if readErr == nil && magic == [2]byte{'M', 'Z'} {
		return path, nil
	}
	// npm's documented Windows distribution carries the direct native executable.
	root := filepath.Join(filepath.Dir(path), "node_modules", "@openai", "codex")
	for _, candidate := range []string{filepath.Join(root, "node_modules", "@openai", "codex-win32-x64", "vendor", "x86_64-pc-windows-msvc", "bin", "codex.exe"), filepath.Join(root, "vendor", "x86_64-pc-windows-msvc", "bin", "codex.exe")} {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return filepath.EvalSymlinks(candidate)
		}
	}
	return "", errors.New("select the existing native Codex executable with STEERLINE_NATIVE_CODEX_EXE")
}
