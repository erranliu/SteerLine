package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/testkit/remotetest"
)

// Three concrete result checks, alongside the existing live checks. No proof
// registry or generic provider is needed for these different test purposes.
func checkRemoteEvidence(id, out string) error {
	read := func(name string, value any) error {
		raw, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, value)
	}
	var frozen remotetest.Input
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(out)), "remote-input.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &frozen); err != nil {
		return err
	}
	switch id {
	case remotetest.Discovery:
		var proof struct {
			ExecutionDigest string `json:"execution_digest"`
			ProfileID       string `json:"profile_id"`
			Environment     string `json:"environment"`
			Candidates      int    `json:"candidates"`
			Browser         struct {
				Execution runnercontract.ExecutionInput `json:"effective_execution"`
				Queries   []struct {
					Request struct {
						Kind, Host  string
						Profile     string `json:"profile_id"`
						Environment string `json:"environment"`
					}
					Result struct {
						State             string
						ContextUnverified bool `json:"context_unverified"`
						Models            []json.RawMessage
					}
				}
			} `json:"browser"`
		}
		if err := read("remote-discovery.json", &proof); err != nil {
			return err
		}
		if proof.ExecutionDigest != frozen.ExecutionDigest || remotetest.Digest(proof.Browser.Execution) != frozen.ExecutionDigest || proof.ProfileID != frozen.Profile.ID || proof.Environment != frozen.Profile.Execution.Target.Environment || proof.Candidates != 0 {
			return errors.New("remote directory configuration/zero-work proof differs")
		}
		found := false
		for _, q := range proof.Browser.Queries {
			if q.Request.Host == "orca_environment" && q.Request.Kind == "models" && q.Request.Environment == proof.Environment && q.Request.Profile == proof.ProfileID && q.Result.State == "ready" && q.Result.ContextUnverified && len(q.Result.Models) > 0 {
				found = true
			}
		}
		if !found {
			return errors.New("missing real remote model catalog with frozen profile binding")
		}
		return nil
	case remotetest.Progress:
		var proof struct {
			Passed    bool
			RunID     string    `json:"run_id"`
			Digest    string    `json:"input_digest"`
			Observed  [][3]bool `json:"observed_states"`
			Candidate string    `json:"candidate_id"`
		}
		if err := read("proof.json", &proof); err != nil {
			return err
		}
		want := [][3]bool{{false, false, false}, {true, false, false}, {true, true, false}, {true, false, false}, {true, true, false}, {true, true, true}}
		if !proof.Passed || proof.RunID == "" || proof.Digest == "" || proof.Candidate == "" || !reflect.DeepEqual(proof.Observed, want) {
			return errors.New("missing original remote progress six-state proof")
		}
		var source struct {
			Source  string `json:"source_commit"`
			Fixture string `json:"fixture_commit"`
			Profile struct {
				Execution runnercontract.ExecutionInput `json:"execution"`
			}
		}
		if err := read("source.json", &source); err != nil {
			return err
		}
		if remotetest.Digest(source.Profile.Execution) != frozen.ExecutionDigest || source.Source != frozen.Commit || source.Fixture == "" {
			return errors.New("remote progress effective execution differs")
		}
		// The same project must have passed its own formal adaptation first.
		return checkSealedEnvironment(read, frozen.ExecutionDigest, source.Fixture)
	case remotetest.Environment, remotetest.LocalEnvironment:
		var admission struct {
			Before        bool   `json:"before_rejected"`
			AfterRejected bool   `json:"after_rejected"`
			Run           string `json:"after_run_id"`
			Stopped       bool   `json:"stopped"`
			Digest        string `json:"execution_digest"`
			Source        string `json:"source_commit"`
			Fixture       string `json:"fixture_commit"`
		}
		if err := read("admission-proof.json", &admission); err != nil {
			return err
		}
		if !admission.Before || !admission.Stopped || admission.Digest != frozen.ExecutionDigest || admission.Source != frozen.Commit || admission.Fixture == "" {
			return errors.New("308 binary admission/original source proof incomplete")
		}
		if admission.AfterRejected && admission.Run == "" {
			if err := checkUnreadyEnvironment(read, frozen.ExecutionDigest, admission.Fixture); err != nil {
				return err
			}
			return errors.New("native environment positive readiness not passed; negative audit is not a positive L4 pass")
		}
		if admission.AfterRejected || admission.Run == "" {
			return errors.New("308 admission outcome is contradictory")
		}
		return checkSealedEnvironment(read, frozen.ExecutionDigest, admission.Fixture)
	}
	return errors.New("unsupported remote L4 scenario")
}

// Environment evidence validates the transport/identity envelope, never project business details.
func checkUnreadyEnvironment(read func(string, any) error, digest, fixture string) error {
	return checkEnvironmentResult(read, digest, fixture, false)
}
func checkSealedEnvironment(read func(string, any) error, digest, fixture string) error {
	return checkEnvironmentResult(read, digest, fixture, true)
}
func checkEnvironmentResult(read func(string, any) error, digest, fixture string, want bool) error {
	var proof struct {
		ExecutionDigest string                           `json:"execution_digest"`
		Identity        runnercontract.ExecutionIdentity `json:"execution_identity"`
		Source          string                           `json:"source_commit"`
		Activity        string                           `json:"activity_id"`
		Rounds          int                              `json:"rounds"`
		Details         []env.Round                      `json:"round_details"`
		RunID           string                           `json:"run_id"`
		Run             runnercontract.Run               `json:"run"`
		Result          runnercontract.ResultFile        `json:"result"`
		Report          env.Report                       `json:"report"`
	}
	if err := read("environment-proof.json", &proof); err != nil {
		return err
	}
	if proof.ExecutionDigest != digest || fixture == "" || proof.Source != fixture || proof.Activity == "" || proof.Rounds != 1 || len(proof.Details) != 1 {
		return errors.New("environment source or single invocation proof differs")
	}
	a := proof.Details[0]
	if !a.Settled || a.Number != 1 || a.Run == nil || a.RunID == "" || a.RunID != proof.RunID || a.RunID != a.Run.RunID || a.RunID != proof.Run.RunID || a.Run.StopStatus != "stopped" || a.Result == nil || a.Result.Version != 1 || a.Result.CollectionStatus != "closed" || !reflect.DeepEqual(*a.Result, proof.Result) || !reflect.DeepEqual(*a.Run, proof.Run) || a.InputDigest == "" || a.InputDigest != a.Run.InputDigest {
		return errors.New("original detection was not stopped and sealed")
	}
	var input runnercontract.RunInput
	if json.Unmarshal(a.Request.Input, &input) != nil || input.Workflow.ID != "execute" || input.Workflow.Version != 1 || input.ResultFile == nil || input.Publication.Provider != "" || remotetest.Digest(input.Execution) != digest {
		return errors.New("detection invocation differs")
	}
	initial, err := base64.StdEncoding.Strict().DecodeString(input.ResultFile.InitialContentBase64)
	if err != nil {
		return err
	}
	var seed env.Report
	if json.Unmarshal(initial, &seed) != nil {
		return errors.New("original result identity missing")
	}
	if seed.ActivityID != proof.Activity || seed.Commit != fixture || seed.Inputs == "" || a.Run.Correlation["environment_activity"] != proof.Activity || a.Run.Correlation["steerline_project"] != seed.Project || a.Run.Correlation["environment_round"] != "1" {
		return errors.New("original environment result binding differs")
	}
	if err := runnercontract.ValidateExecutionIdentity(input.Execution, proof.Identity); err != nil {
		return err
	}
	record := env.Record{ID: seed.ActivityID, Project: seed.Project, Execution: input.Execution, Prepared: env.Prepared{Commit: seed.Commit, Inputs: seed.Inputs, Identity: proof.Identity}}
	raw, err := runnerclient.ValidateResultFile(proof.Result, proof.RunID)
	if err != nil {
		return err
	}
	report, err := env.ParseReport(raw, record, a)
	if err != nil {
		return err
	}
	// Evidence is formatted JSON; compare the original report semantically,
	// while ValidateResultFile above still verifies its sealed raw bytes.
	actual, _ := json.Marshal(report)
	retained, _ := json.Marshal(proof.Report)
	actual, err = runnercontract.CanonicalJSON(actual)
	if err != nil {
		return err
	}
	retained, err = runnercontract.CanonicalJSON(retained)
	if err != nil {
		return err
	}
	if !report.Completed || report.Ready != want || !bytes.Equal(actual, retained) || want && !env.TrustedFinal(a) {
		return errors.New("final project result or native completion differs")
	}
	return nil
}
