package codexworker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkerTurnFindsPRBeforeFinalMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	events := `{"method":"turn/started","params":{"threadId":"00000000-0000-0000-0000-000000000008","turn":{"id":"turn-current"}}}
{"method":"item/completed","params":{"threadId":"00000000-0000-0000-0000-000000000008","turnId":"turn-current","item":{"type":"agentMessage","text":"Created https://git.example/o/r/pulls/8.","phase":"commentary"}}}
{"method":"item/completed","params":{"threadId":"00000000-0000-0000-0000-000000000008","turnId":"turn-current","item":{"type":"agentMessage","text":"Work is ready.","phase":"final_answer"}}}
{"method":"turn/completed","params":{"threadId":"00000000-0000-0000-0000-000000000008","turn":{"id":"turn-current","status":"completed"}}}
`
	if err := os.WriteFile(path, []byte(events), 0600); err != nil {
		t.Fatal(err)
	}
	outcome, err := ReadAppServerOutcome(path, "00000000-0000-0000-0000-000000000008", "turn-current")
	if err != nil || !outcome.Completed || outcome.LastMessage != "Work is ready." || len(outcome.PRCandidates) != 1 || outcome.PRCandidates[0] != "https://git.example/o/r/pulls/8" {
		t.Fatalf("lost an earlier PR clue: outcome=%+v err=%v", outcome, err)
	}
}
