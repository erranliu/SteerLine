package localservice

import (
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/pathguard"
	"os"
	"path/filepath"
)

// Install replaces exactly one named executable inside the explicitly selected
// home. A partial two-program upgrade is reported per executable by the CLIs.
func Install(home, source, name string) (string, error) {
	root, err := pathguard.ResolveProspective(home)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, "bin", name)
	resolved, err := pathguard.ResolveProspective(target)
	if err != nil {
		return "", err
	}
	if !pathguard.Within(root, resolved) || resolved != target {
		return "", errors.New("installation target must be an ordinary path inside home")
	}
	running, err := ExecutableRunning(target)
	if err != nil {
		return "", err
	}
	if running {
		return "", fmt.Errorf("stop running executable before installing %s", target)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(filepath.Dir(target), ".install-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(data)
	if err == nil {
		err = file.Chmod(0700)
	}
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return "", err
	}
	// Recheck immediately before replacing the target.
	if running, err = ExecutableRunning(target); err != nil {
		return "", err
	}
	if running {
		return "", errors.New("installation target started during copy")
	}
	if err = os.Rename(file.Name(), target); err != nil {
		return "", err
	}
	return target, nil
}
