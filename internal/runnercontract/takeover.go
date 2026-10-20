package runnercontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

type PendingStop struct {
	RunID   string  `json:"run_id"`
	Request Request `json:"request"`
}

type ServiceStartInput struct {
	PendingStops []PendingStop `json:"pending_stops"`
}

type TakeoverItem struct {
	RequestID     string `json:"request_id,omitempty"`
	RequestDigest string `json:"request_digest,omitempty"`
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Revision      uint64 `json:"revision"`
	InputDigest   string `json:"input_digest"`
	Status        string `json:"status"`
	Diagnostic    string `json:"diagnostic,omitempty"`
}

type TakeoverReport struct {
	Contract        string         `json:"contract"`
	StateDir        string         `json:"state_dir"`
	InstanceID      string         `json:"instance_id,omitempty"`
	CandidateSHA256 string         `json:"candidate_sha256"`
	Compatible      bool           `json:"compatible"`
	Items           []TakeoverItem `json:"items"`
}

// Validate binds the public report to the selected candidate transport. It
// grants no caller input authority and does not prove service/resource exit.
func (r TakeoverReport) Validate(stateDir, sha string) error {
	if r.Contract != "steerrunner-takeover/v1" || !r.Compatible || !strings.EqualFold(filepath.Clean(r.StateDir), filepath.Clean(stateDir)) || !strings.EqualFold(r.CandidateSHA256, sha) || r.Items == nil {
		return errors.New("Runner candidate takeover is unverified or incompatible")
	}
	seen := map[string]bool{}
	for _, item := range r.Items {
		key := item.Kind + ":" + item.ID
		if item.ID == "" || seen[key] || item.Status != "compatible" && item.Status != "read_only" {
			return errors.New("Runner takeover contains unsupported or duplicate state")
		}
		seen[key] = true
	}
	return nil
}

func (s ServiceStartInput) Validate() error {
	if s.PendingStops == nil || len(s.PendingStops) > 500 {
		return errors.New("original pending stops array is required")
	}
	seen := map[string]bool{}
	for _, stop := range s.PendingStops {
		var revision struct {
			ExpectedRevision uint64 `json:"expected_revision"`
		}
		if stop.RunID == "" || stop.Request.Protocol != "steerrunner-cli/v1" || stop.Request.RequestID == "" || seen[stop.Request.RequestID] || json.Unmarshal(stop.Request.Input, &revision) != nil || revision.ExpectedRevision == 0 {
			return errors.New("original stop identity/revision is required")
		}
		seen[stop.Request.RequestID] = true
	}
	return nil
}

// RequestDigest preserves JSON numbers and the original command/target. It
// compares an accepted envelope, never model input or a replacement request.
func RequestDigest(command, target string, request Request) string {
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
