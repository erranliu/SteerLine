package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

var advisoryProofFacts = []string{
	"automatic_before_browser", "immutable_restart_and_return", "unknown_only_observed",
	"failure_not_retried", "cancel_and_source_settled", "shared_slot_held",
	"no_business_side_effects", "public_wire_checked", "browser_ready", "browser_cancel",
	"cancel_after_progress", "unknown_stop_recovered",
}

func checkAdvisoryEvidence(dir string) error {
	read := func(path string, value any) error {
		raw, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, value)
	}
	var proof map[string]any
	if err := read("advisory-proof.json", &proof); err != nil {
		return err
	}
	if proof["jobs"] != float64(6) {
		return fmt.Errorf("advisory evidence must cover six jobs")
	}
	for _, fact := range advisoryProofFacts {
		if proof[fact] != true {
			return fmt.Errorf("missing advisory proof: %s", fact)
		}
	}
	var wire struct {
		StopAttempts map[string][]struct {
			RequestID string `json:"request_id"`
		} `json:"advisory_stop_attempts"`
		StopQueries map[string]int    `json:"advisory_stop_queries"`
		Questions   map[string]string `json:"advisory_questions"`
		Calls       map[string]int    `json:"submit_calls"`
		Bodies      map[string]string `json:"bodies"`
		Requests    map[string]struct {
			Command string
			RunID   *string `json:"run_id"`
		} `json:"requests"`
	}
	if err := read("advisory-public-requests.json", &wire); err != nil {
		return err
	}
	if len(wire.Questions) != 6 {
		return fmt.Errorf("missing six advisory public identities")
	}
	wanted := map[string]bool{"ready": true, "unknown": true, "failed": true, "cancel": true, "after": true, "source": true}
	for run, question := range wire.Questions {
		if run == "" || !wanted[question] {
			return fmt.Errorf("unexpected advisory public identity")
		}
		delete(wanted, question)
		if question == "cancel" {
			attempts := wire.StopAttempts[run]
			if len(attempts) != 2 || attempts[0].RequestID == "" || attempts[1].RequestID == "" || attempts[0].RequestID == attempts[1].RequestID || wire.StopQueries[attempts[0].RequestID] == 0 {
				return fmt.Errorf("missing original stale stop recovery proof")
			}
			if _, accepted := wire.Requests[attempts[0].RequestID]; accepted {
				return fmt.Errorf("replaced an accepted original stop")
			}
			replacement, ok := wire.Requests[attempts[1].RequestID]
			if !ok || replacement.Command != "stop" || replacement.RunID == nil || *replacement.RunID != run {
				return fmt.Errorf("replacement stop identity differs")
			}
		}
		count := 0
		for id, request := range wire.Requests {
			if request.Command == "submit" && request.RunID != nil && *request.RunID == run {
				var body map[string]any
				if json.Unmarshal([]byte(wire.Bodies[id]), &body) != nil || len(body) == 0 {
					return fmt.Errorf("missing advisory public body")
				}
				count += wire.Calls[id]
			}
		}
		if count != 1 {
			return fmt.Errorf("advisory public submit count differs: %s", question)
		}
	}
	for _, mode := range []string{"ready", "cancel"} {
		info, err := os.Stat(filepath.Join(dir, "browser-"+mode, "advisory-"+mode+".png"))
		if err != nil || info.IsDir() || info.Size() == 0 {
			return fmt.Errorf("missing advisory browser screenshot: %s", mode)
		}
		var browser struct {
			Passed, Navigation, Context, Copy, Cancel bool
			CancelAfterProgress                       bool `json:"cancel_after_progress"`
			Mode                                      string
			Commands                                  []string
		}
		if err := read(filepath.Join("browser-"+mode, "proof.json"), &browser); err != nil {
			return err
		}
		if !browser.Passed || browser.Mode != mode || !browser.Context || !browser.Navigation || browser.Copy != (mode == "ready") || browser.Cancel != (mode == "cancel") || browser.CancelAfterProgress != (mode == "cancel") {
			return fmt.Errorf("incomplete advisory browser evidence: %s", mode)
		}
		if mode == "ready" && len(browser.Commands) != 0 || mode == "cancel" && (len(browser.Commands) != 1 || browser.Commands[0] != "main_agent_cancel") {
			return fmt.Errorf("unexpected advisory browser command: %s", mode)
		}
	}
	return nil
}
