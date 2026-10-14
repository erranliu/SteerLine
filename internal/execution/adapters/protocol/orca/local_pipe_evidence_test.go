package orca

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

// Opt-in decoding of real local pipe responses, never a fake state generator.
func TestOrcaLocalPipeNativeEvidence(t *testing.T) {
	root := os.Getenv("STEERLINE_ORCA_LOCAL_PIPE_EVIDENCE")
	if root == "" {
		t.Skip("requires explicit native local pipe evidence")
	}
	var report struct {
		Local  bool `json:"local_named_pipe"`
		Paired bool `json:"paired"`
		Fixed  struct{ Runtime, Worktree, Handle, Incarnation, Pane, Provider string }
		Rounds []struct {
			Label   string
			Stages  []string
			Working bool `json:"working_observed"`
			Done    bool `json:"done_observed"`
		}
	}
	raw, err := os.ReadFile(filepath.Join(root, "local-agent-roundtrip.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if !report.Local || report.Paired || len(report.Rounds) != 2 {
		t.Fatal("not a two-round unpaired local sample")
	}
	process := b.ProcessIdentity{Kind: "orca_terminal", RuntimeID: report.Fixed.Runtime, WorktreeID: report.Fixed.Worktree, Terminal: report.Fixed.Handle, Incarnation: report.Fixed.Incarnation}
	for _, round := range report.Rounds {
		t.Run(round.Label, func(t *testing.T) {
			accepted, started := false, false
			for _, stage := range round.Stages {
				accepted = accepted || stage == "input_accepted"
				started = started || stage == "turn_started"
			}
			if !accepted || !started || !round.Working || !round.Done {
				t.Fatal("native input/start/end evidence is incomplete")
			}
			read := func(suffix string) json.RawMessage {
				data, err := os.ReadFile(filepath.Join(root, round.Label+suffix))
				if err != nil {
					t.Fatal(err)
				}
				envelope, err := ParseEnvelope(data)
				if err != nil || envelope.Meta.RuntimeID != process.RuntimeID {
					t.Fatalf("native envelope identity changed: %v", err)
				}
				return envelope.Result
			}
			metadata, err := DecodeOriginalState(read("-metadata.json"), process)
			if err != nil {
				t.Fatal(err)
			}
			agent, err := DecodeOriginalAgent(read("-agents.json"), process.WorktreeID, report.Fixed.Pane)
			if err != nil {
				t.Fatal(err)
			}
			if metadata.PaneKey != agent.PaneKey || metadata.ProviderSession.ID != report.Fixed.Provider || agent.AgentType != "codex" || agent.MainAgent == nil || agent.MainAgent.State != "done" || agent.LastAssistantMessage != round.Label {
				t.Fatal("existing state decoders did not retain original identity and main-agent final")
			}
		})
	}
}
