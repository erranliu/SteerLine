package main

import (
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

// A correct mixed failure report is a valid negative L4 outcome, never ready.
func checkUnreadyEnvironment(read func(string, any) error, digest, fixture string) error {
	var proof struct {
		Digest   string      `json:"execution_digest"`
		Source   string      `json:"source_commit"`
		Activity string      `json:"activity_id"`
		State    string      `json:"state"`
		Rounds   int         `json:"rounds"`
		Details  []env.Round `json:"round_details"`
		Report   env.Report  `json:"report"`
		L2       []string    `json:"l2_ids"`
	}
	if err := read("environment-proof.json", &proof); err != nil {
		return err
	}
	if proof.Digest != digest || fixture == "" || proof.Source != fixture || proof.State != "failed" || proof.Rounds != 3 || len(proof.Details) != 3 || proof.Activity == "" || len(proof.Report.ScriptDigest) != 64 {
		return errors.New("unready environment source or original budget differs")
	}
	seenRuns := map[string]bool{}
	for i, round := range proof.Details {
		if round.Number != i+1 || !round.Settled || round.Run == nil || round.RunID == "" || round.RunID != round.Run.RunID || round.Run.StopStatus != "stopped" || round.Result == nil || round.Result.Version != 1 || round.Result.CollectionStatus != "closed" {
			return errors.New("unready round lacks original stopped/sealed evidence")
		}
		if seenRuns[round.RunID] {
			return errors.New("unready verification reused a Run")
		}
		seenRuns[round.RunID] = true
		var input runnercontract.RunInput
		if json.Unmarshal(round.Request.Input, &input) != nil || remotetest.Digest(input.Execution) != digest {
			return errors.New("unready round execution differs")
		}
		raw, err := runnerclient.ValidateResultFile(*round.Result, round.RunID)
		if err != nil {
			return err
		}
		record := env.Record{ID: proof.Activity, Prepared: env.Prepared{Commit: fixture, ScriptDigest: proof.Report.ScriptDigest, L2: proof.L2}}
		report, err := env.ParseReport(raw, record, round)
		if err != nil {
			return err
		}
		if !report.Completed || report.PR.URL != "" || report.PR.Merged || report.PR.ChecksPassed || report.PR.ReviewsPassed || report.PR.OutcomeUnknown || !report.PR.CleanupCompleted || report.PR.Base == "" || report.PR.Head != round.Run.Execution.Branch {
			return errors.New("failed requirements produced unexpected PR effects")
		}
		passed, unmet := 0, 0
		for _, check := range report.Checks {
			if check.Status == "passed" {
				passed++
			} else {
				unmet++
			}
		}
		if passed == 0 || unmet == 0 {
			return errors.New("mixed environment requirement results missing")
		}
		for _, item := range report.L2 {
			if item.Status != "incomplete" {
				return errors.New("unmet requirements unexpectedly executed L2")
			}
		}
		if i == 2 && !reflect.DeepEqual(*report, proof.Report) {
			return errors.New("last mixed report differs from sealed bytes")
		}
	}
	var provider struct {
		NoPR   bool   `json:"no_pr"`
		Base   string `json:"validation_base"`
		Digest string `json:"execution_digest"`
	}
	if err := read("provider-pr-proof.json", &provider); err != nil {
		return err
	}
	if !provider.NoPR || provider.Digest != digest || provider.Base != proof.Report.PR.Base {
		return errors.New("independent no-PR evidence differs")
	}
	return nil
}

func checkSealedEnvironment(read func(string, any) error, digest, fixture string) error {
	var proof struct {
		ExecutionDigest string                    `json:"execution_digest"`
		Source          string                    `json:"source_commit"`
		Activity        string                    `json:"activity_id"`
		Rounds          int                       `json:"rounds"`
		RunID           string                    `json:"run_id"`
		Run             runnercontract.Run        `json:"run"`
		Result          runnercontract.ResultFile `json:"result"`
		Report          env.Report                `json:"report"`
		L2              []string                  `json:"l2_ids"`
	}
	if err := read("environment-proof.json", &proof); err != nil {
		return err
	}
	if proof.ExecutionDigest != digest || proof.Activity == "" || proof.Source != fixture || fixture == "" || proof.Rounds < 1 || proof.Rounds > 3 || proof.RunID != proof.Run.RunID || proof.Run.StopStatus != "stopped" || proof.Result.CollectionStatus != "closed" || proof.Result.Version != 1 {
		return errors.New("environment source/budget/stopped seal proof incomplete")
	}
	raw, err := runnerclient.ValidateResultFile(proof.Result, proof.RunID)
	if err != nil {
		return err
	}
	var report env.Report
	if err = json.Unmarshal(raw, &report); err != nil {
		return err
	}
	if !reflect.DeepEqual(report, proof.Report) || report.Version != 1 || len(report.ScriptDigest) != 64 || report.ActivityID != proof.Activity || report.Commit != proof.Source || report.Round != proof.Rounds || !report.Passed(report.PR.Base, proof.Run.Execution.Branch) {
		return errors.New("sealed environment report differs or did not pass")
	}
	want := map[string]bool{}
	for _, id := range proof.L2 {
		if want[id] || id == "" {
			return errors.New("duplicate/empty L2 definition")
		}
		want[id] = true
	}
	if len(want) == 0 || len(want) != len(report.L2) {
		return errors.New("incomplete L2 set")
	}
	for _, item := range report.L2 {
		if !want[item.ID] {
			return errors.New("L2 result differs")
		}
		delete(want, item.ID)
	}
	var provider struct {
		PR struct {
			Merged bool
			Merge  string `json:"merge_commit_sha"`
			Head   struct{ Ref string }
			Base   struct{ Ref string }
		}
		Checks  bool   `json:"required_checks_verified"`
		Reviews bool   `json:"required_reviews_verified"`
		Digest  string `json:"execution_digest"`
	}
	if err := read("provider-pr-proof.json", &provider); err != nil {
		return err
	}
	if !provider.PR.Merged || provider.PR.Merge != report.PR.MergeCommit || provider.PR.Head.Ref != report.PR.Head || provider.PR.Base.Ref != report.PR.Base || !provider.Checks || !provider.Reviews || provider.Digest != digest {
		return errors.New("independent provider PR proof differs")
	}
	var summary struct {
		Passed       bool     `json:"passed"`
		Cleanup      bool     `json:"cleanup_complete"`
		Level        string   `json:"level"`
		IDs          []string `json:"selected_ids"`
		Verification []struct {
			ID     string `json:"test_id"`
			Status string `json:"status"`
		} `json:"project_verification"`
	}
	if err := read("original-l2-summary.json", &summary); err != nil {
		return err
	}
	if !summary.Passed || !summary.Cleanup || summary.Level != "L2" || len(summary.IDs) != len(proof.L2) || len(summary.Verification) != len(proof.L2) {
		return errors.New("original complete L2 evidence differs")
	}
	verified := map[string]bool{}
	selected := map[string]bool{}
	for _, id := range summary.IDs {
		if selected[id] || id == "" {
			return errors.New("invalid original L2 selection")
		}
		selected[id] = true
	}
	for _, item := range summary.Verification {
		if item.Status != "passed" || item.ID == "" || verified[item.ID] {
			return errors.New("original L2 item did not pass uniquely")
		}
		verified[item.ID] = true
	}
	for _, id := range proof.L2 {
		if !verified[id] || !selected[id] {
			return errors.New("original L2 item missing")
		}
	}
	return nil
}
