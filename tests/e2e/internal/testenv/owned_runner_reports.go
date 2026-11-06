//go:build e2e

package testenv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Called only after exact workspace/branch ownership, original Runner settlement
// and empty terminal inventories. Keep Git's dirty-workspace refusal: archive
// only this stopped Run's two protocol files, never force-remove other changes.
func archiveOwnedRunnerReports(ctx context.Context, workspace, runID, evidence string) error {
	if !filepath.IsAbs(workspace) || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(runID) {
		return errors.New("report artifact ownership is incomplete")
	}
	resolved, err := filepath.EvalSymlinks(workspace)
	if err != nil || !sameOrcaPath(resolved, workspace) {
		return errors.New("report workspace path changed")
	}
	allowed := map[string]bool{
		".steerrunner/progress/" + runID + "/progress.bin": true,
		".steerrunner/result/" + runID + "/result.bin":     true,
	}
	status, err := exec.CommandContext(ctx, "git", "-C", workspace, "status", "--porcelain=v1", "--untracked-files=all", "-z").Output()
	if err != nil {
		return err
	}
	files := map[string][]byte{}
	for _, entry := range strings.Split(string(status), "\x00") {
		if entry == "" {
			continue
		}
		if !strings.HasPrefix(entry, "?? ") || !allowed[strings.TrimPrefix(entry, "?? ")] {
			return errors.New("owned workspace has changes other than original Runner reports")
		}
		relative := strings.TrimPrefix(entry, "?? ")
		path := filepath.Join(workspace, filepath.FromSlash(relative))
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || !sameOrcaPath(resolved, path) {
			return errors.New("report file path changed")
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("report file is not regular")
		}
		files[path], err = os.ReadFile(path)
		if err != nil {
			return err
		}
	}
	if len(files) == 0 {
		return nil
	}
	if err := os.MkdirAll(evidence, 0700); err != nil {
		return err
	}
	for path, body := range files {
		archive := filepath.Join(evidence, runID+"-"+filepath.Base(path))
		f, err := os.OpenFile(archive, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, writeErr := f.Write(body)
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			return errors.Join(writeErr, closeErr)
		}
		archived, err := os.ReadFile(archive)
		if err != nil || !bytes.Equal(body, archived) {
			return errors.New("report archive did not preserve original bytes")
		}
	}
	for path, body := range files {
		current, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(current, body) {
			return errors.New("report changed after settlement")
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}
