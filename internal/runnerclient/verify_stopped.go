package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

type StoppedVerification struct {
	StateDir         string         `json:"state_dir"`
	ResourcesSettled bool           `json:"resources_settled"`
	Checks           []StoppedCheck `json:"checks"`
	Runs             []StoppedRun   `json:"runs"`
}
type StoppedCheck struct {
	CheckID          string `json:"check_id"`
	InputDigest      string `json:"input_digest"`
	ResourcesSettled bool   `json:"resources_settled"`
}
type StoppedRun struct {
	StopEvidenceScope string `json:"stop_evidence_scope,omitempty"`
	RunID             string `json:"run_id"`
	InputDigest       string `json:"input_digest"`
	ResourcesSettled  bool   `json:"resources_settled"`
	Diagnostic        string `json:"diagnostic,omitempty"`
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
	if !strings.EqualFold(filepath.Clean(result.StateDir), want) || result.Runs == nil || result.Checks == nil {
		return result, errors.New("Runner original native resource absence remains unverified")
	}
	if !result.ResourcesSettled {
		diagnostics := []string{"Runner original native resource absence remains unverified"}
		for _, r := range result.Runs {
			if !r.ResourcesSettled && r.RunID != "" && r.Diagnostic != "" {
				diagnostics = append(diagnostics, r.Diagnostic)
			}
		}
		return result, errors.New(strings.Join(diagnostics, "; "))
	}
	seen := map[string]bool{}
	for _, r := range result.Runs {
		if r.RunID == "" || r.InputDigest == "" || seen[r.RunID] || !r.ResourcesSettled || r.Diagnostic != "" {
			return result, errors.New("Runner stopped verification contains missing, duplicate or unsettled Run evidence")
		}
		seen[r.RunID] = true
	}
	seenChecks := map[string]bool{}
	for _, check := range result.Checks {
		if check.CheckID == "" || check.InputDigest == "" || seenChecks[check.CheckID] || !check.ResourcesSettled {
			return result, errors.New("Runner check resource settlement is unverified")
		}
		seenChecks[check.CheckID] = true
	}
	return result, nil
}
