package driver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/app"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

// Only the selected Runner transport pin changes in this dedicated acceptance
// variant. The live scheme, model, permissions, materials and original Run stay fixed.
func (d *debugRelease) tmGenerationTakeover(ctx context.Context, out, candidate string, generation api.TestGeneration, original runnerclient.Run, before runnerclient.Client) (runnerclient.Client, error) {
	fail := func(err error) (runnerclient.Client, error) { return runnerclient.Client{}, err }
	if !filepath.IsAbs(candidate) {
		return fail(errors.New("generation takeover requires an absolute candidate executable"))
	}
	data, err := os.ReadFile(candidate)
	if err != nil {
		return fail(err)
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	if sha == before.SHA256 {
		return fail(errors.New("generation takeover requires two different Runner builds"))
	}
	selected := runnerclient.Client{Executable: candidate, SHA256: sha, StateDir: before.StateDir, Timeout: 30 * time.Second}
	version, err := selected.Version(ctx)
	if err != nil {
		return fail(err)
	}
	raw, err := d.tmCommand(out, "generation-takeover-online", d.binary, "verify-runner-takeover", "--runner-exe", candidate, "--runner-sha256", sha, "--state-dir", before.StateDir, "--json")
	if err != nil {
		return fail(err)
	}
	var takeover runnercontract.TakeoverReport
	if err = json.Unmarshal(raw, &takeover); err != nil {
		return fail(err)
	}
	bound := false
	for _, item := range takeover.Items {
		if item.ID == original.RunID && item.InputDigest == original.InputDigest && item.Kind == "run" && item.Status == "compatible" {
			bound = true
		}
	}
	if !takeover.Compatible || !bound {
		return fail(errors.New("candidate cannot take over the original generation"))
	}
	// A failed stop remains a failed resource-settlement result. Only the
	// production verify-restart proof can authorize this compatible restart.
	_, _ = d.tmCommand(out, "generation-stop-a", d.binary, "stop")
	raw, err = d.tmCommand(out, "generation-restart-proof", d.binary, "verify-restart", "--runner-exe", candidate, "--json")
	if err != nil {
		return fail(err)
	}
	var restart app.ApplicationRestartVerification
	if err = json.Unmarshal(raw, &restart); err != nil {
		return fail(err)
	}
	if !restart.Restartable || restart.RunnerBuild.SHA256 != before.SHA256 || restart.CandidateBuild == nil || restart.CandidateBuild.SHA256 != sha || restart.Takeover == nil || !restart.Takeover.Compatible {
		return fail(errors.New("generation restart proof differs from the original and candidate builds"))
	}
	if err = os.WriteFile(before.Executable, data, 0700); err != nil {
		return fail(err)
	}
	selection := runnerclient.Selection{Executable: before.Executable, Version: version.Version, SHA256: sha}
	path := filepath.Join(d.home, "execution.json")
	raw, err = os.ReadFile(path)
	if err != nil {
		return fail(err)
	}
	var configuration map[string]json.RawMessage
	if err = json.Unmarshal(raw, &configuration); err != nil {
		return fail(err)
	}
	configuration["runner"], err = json.Marshal(selection)
	if err != nil {
		return fail(err)
	}
	if err = controlled.Write(path, configuration); err != nil {
		return fail(err)
	}
	if _, err = d.tmCommand(out, "generation-start-b", d.binary, "start", "--no-open"); err != nil {
		return fail(err)
	}
	after, _, err := selection.Open(ctx, before.StateDir, 30*time.Second)
	if err != nil {
		return fail(err)
	}
	startup, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var status runnerclient.ServiceInfo
	err = tmPoll(startup, func() (bool, error) {
		var statusErr error
		status, statusErr = after.ServiceStatus(startup)
		if statusErr != nil {
			return false, statusErr
		}
		return status.Status == "ready", nil
	})
	if err != nil {
		return fail(err)
	}
	if status.InstanceID == "" || status.InstanceID == before.InstanceID || status.ExecutableSHA256 != sha {
		return fail(errors.New("replacement Runner instance/build not verified"))
	}
	after.InstanceID = status.InstanceID
	inspection, err := after.Inspect(ctx, "run-id", original.RunID, false)
	if err != nil {
		return fail(err)
	}
	if inspection.Run == nil || inspection.Run.InputDigest != original.InputDigest || !bytes.Equal(mustTMJSON(inspection.Run.Final), mustTMJSON(original.Final)) || !bytes.Equal(mustTMJSON(inspection.Run.ContentResult), mustTMJSON(original.ContentResult)) {
		return fail(errors.New("generation original input/final/result changed during takeover"))
	}
	if err = controlled.Write(filepath.Join(out, "generation-takeover.json"), map[string]any{
		"passed": true, "generation_id": generation.ID, "run_id": original.RunID, "input_digest": original.InputDigest,
		"original_sha256": before.SHA256, "candidate_sha256": sha, "original_instance": before.InstanceID,
		"candidate_instance": after.InstanceID, "before": original, "after": inspection.Run,
	}); err != nil {
		return fail(err)
	}
	return after, nil
}

func mustTMJSON(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}
