package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

type StoppedVerification struct {
	StateDir         string       `json:"state_dir"`
	ResourcesSettled bool         `json:"resources_settled"`
	Runs             []StoppedRun `json:"runs"`
}
type StoppedRun struct {
	RunID            string `json:"run_id"`
	InputDigest      string `json:"input_digest"`
	ResourcesSettled bool   `json:"resources_settled"`
	Diagnostic       string `json:"diagnostic,omitempty"`
}

func (c Client) VerifyStopped(ctx context.Context) (StoppedVerification, error) {
	response, err := c.Call(ctx, "service verify-stopped", nil, nil)
	if err != nil {
		return StoppedVerification{}, err
	}
	var result StoppedVerification
	if json.Unmarshal(response.Data, &result) != nil {
		return result, errors.New("Runner stopped verification is invalid")
	}
	want, err := filepath.EvalSymlinks(c.StateDir)
	if err != nil {
		want = filepath.Clean(c.StateDir)
	}
	if !strings.EqualFold(filepath.Clean(result.StateDir), want) || !result.ResourcesSettled || result.Runs == nil {
		return result, errors.New("Runner original native resource absence remains unverified")
	}
	seen := map[string]bool{}
	for _, r := range result.Runs {
		if r.RunID == "" || r.InputDigest == "" || seen[r.RunID] || !r.ResourcesSettled || r.Diagnostic != "" {
			return result, errors.New("Runner stopped verification contains missing, duplicate or unsettled Run evidence")
		}
		seen[r.RunID] = true
	}
	return result, nil
}
