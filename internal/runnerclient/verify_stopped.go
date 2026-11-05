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
	StopEvidenceScope string `json:"stop_evidence_scope,omitempty"`
	RunID             string `json:"run_id"`
	InputDigest       string `json:"input_digest"`
	ResourcesSettled  bool   `json:"resources_settled"`
	Diagnostic        string `json:"diagnostic,omitempty"`
}

// An unsettledRunsError carries an otherwise valid inventory.
type unsettledRunsError struct{ diagnostic string }

func (e *unsettledRunsError) Error() string { return e.diagnostic }

func (c Client) VerifyStopped(ctx context.Context) (StoppedVerification, error) {
	result, err := c.StoppedResourceInventory(ctx)
	if err != nil {
		return result, err
	}
	if !result.ResourcesSettled {
		diagnostics := []string{"Runner original native resource absence remains unverified"}
		for _, run := range result.Runs {
			if !run.ResourcesSettled {
				diagnostics = append(diagnostics, run.Diagnostic)
			}
		}
		return result, &unsettledRunsError{diagnostic: strings.Join(diagnostics, "; ")}
	}
	return result, nil
}

// StoppedResourceInventory validates a cold inventory, including unsettled Runs.
// Its success validates the inventory, not resource settlement or deletion.
func (c Client) StoppedResourceInventory(ctx context.Context) (StoppedVerification, error) {
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
	if !strings.EqualFold(filepath.Clean(result.StateDir), want) || result.Runs == nil {
		return result, errors.New("Runner original native resource absence remains unverified")
	}
	seen := map[string]bool{}
	diagnostics := []string{"Runner original native resource absence remains unverified"}
	for _, r := range result.Runs {
		if r.RunID == "" || r.InputDigest == "" || seen[r.RunID] || r.ResourcesSettled != (r.Diagnostic == "") {
			return result, errors.New("Runner stopped verification contains missing, duplicate or unsettled Run evidence")
		}
		seen[r.RunID] = true
		if !r.ResourcesSettled {
			diagnostics = append(diagnostics, r.Diagnostic)
		}
	}
	if result.ResourcesSettled != (len(diagnostics) == 1) {
		return result, errors.New("Runner stopped verification contains inconsistent resource evidence")
	}
	return result, nil
}
