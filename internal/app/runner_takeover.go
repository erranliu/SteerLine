package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	host "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

func validTakeover(proof runnerclient.TakeoverReport, pin runnercontract.BuildPin) bool {
	if proof.Contract != "steerrunner-takeover/v1" || !proof.Compatible || proof.CandidateSHA256 != pin.SHA256 || canonicalPath(proof.StateDir) != canonicalPath(pin.StateDir) || proof.Items == nil {
		return false
	}
	for _, item := range proof.Items {
		if item.Status != "compatible" && item.Status != "read_only" {
			return false
		}
	}
	return true
}

// Public request-digest v1: canonical JSON preserving numbers, with the original
// protocol/command/target. This compares the accepted envelope, not model input.
func runnerRequestDigest(command, target string, request runnercontract.Request) string {
	decoder := json.NewDecoder(bytes.NewReader(request.Input))
	decoder.UseNumber()
	var input any
	if decoder.Decode(&input) != nil {
		return ""
	}
	raw, _ := json.Marshal(struct {
		Protocol, Command, Target string
		Input                     any
	}{request.Protocol, command, target, input})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func takeoverCandidate(proof runnerclient.TakeoverReport, c runnerstate.Candidate) (runnerclient.TakeoverItem, bool) {
	for _, item := range proof.Items {
		if item.Kind != "run" || item.Status != "compatible" && item.Status != "read_only" {
			continue
		}
		if c.RunID != "" {
			if item.ID == c.RunID && c.InputDigest != "" && item.InputDigest == c.InputDigest {
				return item, true
			}
		} else if item.RequestID == c.Submit.RequestID && item.RequestDigest == runnerRequestDigest("submit", "", c.Submit) {
			return item, true
		}
	}
	return runnerclient.TakeoverItem{}, false
}

func takeoverCheck(proof runnerclient.TakeoverReport, c runnerstate.EnvironmentCheckRecord) bool {
	if canonicalPath(c.StateDir) != canonicalPath(proof.StateDir) {
		return false
	}
	for _, item := range proof.Items {
		if item.Kind != "check" || item.Status != "compatible" && item.Status != "read_only" {
			continue
		}
		if c.CheckID != "" && c.Report != nil && item.ID == c.CheckID && item.InputDigest == c.Report.InputDigest {
			return true
		}
		if c.CheckID == "" && len(c.Operations) > 0 && item.RequestID == c.Operations[0].Request.RequestID && item.RequestDigest == runnerRequestDigest("environment-check start", "", c.Operations[0].Request) {
			return true
		}
	}
	// An explicitly rejected, never-created check has no native state to adopt.
	return c.CheckID == "" && len(c.Operations) > 0 && c.Operations[0].Status == "rejected"
}

// The home is owned and no project actors have started. Take the usual project
// locks only while preserving the caller's existing durable stop outbox. Runner
// remains the only execution-state authority and admits stops before workers.
func (d *ApplicationDirectory) prepareRunnerStops(ctx context.Context, cli *runnerclient.Client) ([]runnerclient.PendingStop, error) {
	var pending []runnerclient.PendingStop
	for _, entry := range d.entries {
		if entry.ref.RunDir == "" {
			if entry.view.RuntimeState == "failed" {
				return nil, errors.New("Runner startup cannot inspect an unresolved project census")
			}
			continue
		}
		if _, err := os.Stat(filepath.Join(entry.ref.RunDir, "steerline.db")); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		c, err := loadConfigSnapshot(entry.ref.ConfigPath, entry.data, d.global, nil, false)
		if err != nil {
			return nil, err
		}
		stops, err := prepareProjectRunnerStops(ctx, &c, cli)
		if err != nil {
			return nil, fmt.Errorf("%s startup recovery: %w", entry.view.ConfigID, err)
		}
		pending = append(pending, stops...)
	}
	return pending, nil
}

func prepareProjectRunnerStops(ctx context.Context, c *Config, cli *runnerclient.Client) ([]runnerclient.PendingStop, error) {
	if err := c.ResolveIdentity(ctx); err != nil {
		return nil, err
	}
	repoLock, err := host.LockRepository(c.Identity())
	if err != nil {
		return nil, err
	}
	defer repoLock.Close()
	runLock, err := host.LockRepository(c.RunDir)
	if err != nil {
		return nil, err
	}
	defer runLock.Close()
	store, err := OpenNativeTaskCommandStore(ctx, c)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	snapshot, err := store.ReadRunnerSnapshot(ctx, c.Identity())
	if err != nil {
		return nil, err
	}
	need := false
	for _, candidate := range snapshot.Candidates {
		if !sameRunnerBuild(candidate.RunnerBuild, cli.BuildPin()) || candidate.StopRequested && !candidate.InputsRevoked {
			need = true
		}
	}
	checks, err := store.Checks(ctx, c.Identity())
	if err != nil {
		return nil, err
	}
	for _, check := range checks {
		if check.Selection.SHA256 != cli.SHA256 || check.ResourceDigest != cli.ResourceDigest {
			need = true
		}
	}
	if !need {
		return nil, nil
	}
	if cli.Takeover == nil {
		proof, err := cli.VerifyTakeover(ctx)
		if err != nil {
			return nil, err
		}
		cli.Takeover = &proof
	}
	if !validTakeover(*cli.Takeover, cli.BuildPin()) {
		return nil, errors.New("original Runner state is incompatible")
	}
	for _, check := range checks {
		if !takeoverCheck(*cli.Takeover, check) {
			return nil, errors.New("original environment check differs from takeover proof")
		}
	}
	var pending []runnerclient.PendingStop
	for _, candidate := range snapshot.Candidates {
		if canonicalPath(candidate.StateDir) != canonicalPath(cli.StateDir) {
			return nil, errors.New("candidate Runner directory changed")
		}
		item, bound := takeoverCandidate(*cli.Takeover, candidate)
		if candidate.RunID != "" && !bound {
			return nil, errors.New("original candidate is absent or input digest differs from takeover proof")
		}
		if !candidate.StopRequested || candidate.InputsRevoked {
			continue
		}
		if !bound {
			if candidate.SubmitState == "pending" || candidate.SubmitState == "rejected" || candidate.SubmitState == "cancelled" {
				continue
			}
			// A request not present in the complete offline command projection
			// stays observe-only. Starting workers cannot create its absent Run.
			if candidate.RunID == "" {
				continue
			}
			return nil, errors.New("original cancellation target is unverified")
		}
		if item.Status != "compatible" {
			return nil, errors.New("read-only original Run cannot accept its pending stop")
		}
		if candidate.Stop == nil || candidate.Stop.State == "rejected" {
			body, _ := json.Marshal(map[string]uint64{"expected_revision": item.Revision})
			candidate.Stop = &runnerstate.Action{Command: "stop", Request: runnercontract.Request{Protocol: runnerclient.Protocol, RequestID: "sl-startup-stop-" + candidate.ID + fmt.Sprintf("-%d", candidate.Revision), Input: body}, State: "unknown"}
			expected := candidate.Revision
			candidate.Revision++
			if err := store.SaveCandidate(ctx, candidate, expected, nil); err != nil {
				return nil, err
			}
		}
		if candidate.Stop.State == "unknown" || candidate.Stop.State == "pending" {
			pending = append(pending, runnerclient.PendingStop{RunID: item.ID, Request: candidate.Stop.Request})
		}
	}
	return pending, nil
}
