package codexworker

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// NativeExecutable resolves a configured Codex command to its direct process.
// The detached process identity must name the actual app-server process.
// npm's codex shim launches Node, which launches another process; tracking
// either wrapper would leave the server alive after a confirmed wrapper stop.
func NativeExecutable(command string) (string, error) {
	if command == "" {
		return "", errors.New("Codex executable is missing")
	}
	path, err := exec.LookPath(command)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	name := strings.ToLower(filepath.Base(resolved))
	knownWrapper := name == "node.exe" || name == "node" || name == "cmd.exe" || name == "powershell.exe" || name == "pwsh.exe" || name == "sh" || name == "bash"
	if isNativeCodexExecutable(resolved) && !knownWrapper {
		return resolved, nil
	}
	var packageRoot string
	switch {
	case name == "codex.js" && filepath.Base(filepath.Dir(resolved)) == "bin":
		packageRoot = filepath.Dir(filepath.Dir(resolved))
	case runtime.GOOS == "windows" && (strings.HasSuffix(name, ".cmd") || strings.HasSuffix(name, ".bat")):
		shim, readErr := os.ReadFile(resolved)
		if readErr != nil || len(shim) > 16<<10 || !strings.Contains(strings.ToLower(string(shim)), `node_modules\@openai\codex\bin\codex.js`) {
			return "", errors.New("configured Codex wrapper cannot be bound to its native executable")
		}
		packageRoot = filepath.Join(filepath.Dir(resolved), "node_modules", "@openai", "codex")
	default:
		return "", fmt.Errorf("configured Codex command %q is not a native executable or known package shim", resolved)
	}
	platformPackage, target, nativeName := "", "", "codex"
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		platformPackage, target, nativeName = "codex-win32-x64", "x86_64-pc-windows-msvc", "codex.exe"
	case "windows/arm64":
		platformPackage, target, nativeName = "codex-win32-arm64", "aarch64-pc-windows-msvc", "codex.exe"
	case "linux/amd64":
		platformPackage, target = "codex-linux-x64", "x86_64-unknown-linux-musl"
	case "linux/arm64":
		platformPackage, target = "codex-linux-arm64", "aarch64-unknown-linux-musl"
	case "darwin/amd64":
		platformPackage, target = "codex-darwin-x64", "x86_64-apple-darwin"
	case "darwin/arm64":
		platformPackage, target = "codex-darwin-arm64", "aarch64-apple-darwin"
	default:
		return "", errors.New("configured Codex package has no supported native target")
	}
	candidates := []string{
		filepath.Join(packageRoot, "node_modules", "@openai", platformPackage, "vendor", target, "bin", nativeName),
		filepath.Join(packageRoot, "vendor", target, "bin", nativeName),
	}
	for _, candidate := range candidates {
		info, statErr := os.Stat(candidate)
		if statErr == nil && !info.IsDir() && isNativeCodexExecutable(candidate) {
			return filepath.EvalSymlinks(candidate)
		}
	}
	return "", fmt.Errorf("configured Codex package lacks its native %s app-server executable", target)
}

func isNativeCodexExecutable(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	var magic [4]byte
	if _, err := io.ReadFull(file, magic[:]); err != nil {
		return false
	}
	switch runtime.GOOS {
	case "windows":
		return magic[0] == 'M' && magic[1] == 'Z'
	case "linux":
		return magic == [4]byte{0x7f, 'E', 'L', 'F'}
	case "darwin":
		return magic == [4]byte{0xcf, 0xfa, 0xed, 0xfe} || magic == [4]byte{0xfe, 0xed, 0xfa, 0xcf} || magic == [4]byte{0xca, 0xfe, 0xba, 0xbe}
	}
	return false
}
