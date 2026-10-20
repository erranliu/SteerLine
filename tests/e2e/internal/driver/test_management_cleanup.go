package driver

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

// Called only after formal native settlement. A publish attempt may have
// pushed its original head even when no PR was created and tracked code stayed clean.
func (d *debugRelease) tmCleanupOwnedRefs(out, commit, head, run string) error {
	branches := []string{d.base}
	if head != "" {
		if run == "" || head != "codex/runner-"+run || head == d.base {
			return errors.New("original Run head identity is ambiguous; refs preserved")
		}
		raw, err := d.tmCommand(out, "owned-head-prs", "gh", "pr", "list", "--repo", debugRepository, "--head", head, "--state", "all", "--json", "number,url")
		if err != nil {
			return err
		}
		var prs []json.RawMessage
		if err = json.Unmarshal(raw, &prs); err != nil || prs == nil || len(prs) != 0 {
			return errors.New("owned head has an unverified PR history; refs preserved")
		}
		branches = append(branches, head)
	}
	present := []string{}
	for _, branch := range branches {
		remote, err := d.git("ls-remote", "origin", "refs/heads/"+branch)
		if err != nil {
			return err
		}
		if remote == "" {
			continue
		}
		fields := strings.Fields(remote)
		if len(fields) != 2 || fields[0] != commit || fields[1] != "refs/heads/"+branch {
			return errors.New("owned ref changed; refs preserved")
		}
		present = append(present, branch)
	}
	for _, branch := range present {
		if _, err := d.git("push", "--force-with-lease=refs/heads/"+branch+":"+commit, "origin", ":refs/heads/"+branch); err != nil {
			return err
		}
	}
	for _, branch := range branches {
		remote, err := d.git("ls-remote", "origin", "refs/heads/"+branch)
		if err != nil || remote != "" {
			return errors.New("owned ref cleanup remains unverified")
		}
	}
	return controlled.Write(filepath.Join(out, "owned-refs-cleanup.json"), map[string]any{"passed": true, "commit": commit, "branches": branches, "run_id": run})
}
