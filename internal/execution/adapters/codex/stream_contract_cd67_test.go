package codexworker_test

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/adapters/protocol/codexapp"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestAppServerOutcomeUsesNativeTurnAndEarlierPRItem(t *testing.T) {
	thread, turn := "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
	path := filepath.Join(t.TempDir(), "appserver.jsonl")
	events := []codexapp.Envelope{
		{Method: "turn/started", Params: mustJSON(map[string]any{"threadId": thread, "turn": map[string]any{"id": turn}})},
		{Method: "item/completed", Params: mustJSON(map[string]any{"threadId": thread, "turnId": turn, "item": map[string]any{"type": "agentMessage", "text": "PR https://git.example/o/r/pulls/8"}})},
		{Method: "turn/completed", Params: mustJSON(map[string]any{"threadId": thread, "turn": map[string]any{"id": turn, "status": "completed", "items": []any{map[string]any{"type": "agentMessage", "phase": "final_answer", "text": "Done"}}}})},
	}
	var raw []byte
	for _, event := range events {
		line, _ := json.Marshal(event)
		raw = append(raw, append(line, '\n')...)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	outcome, err := codexworker.ReadAppServerOutcome(path, thread, turn)
	if err != nil || !outcome.Started || !outcome.Completed || outcome.NativeStatus != "completed" || outcome.LastMessage != "Done" || len(outcome.PRCandidates) != 1 || outcome.PRCandidates[0] != "https://git.example/o/r/pulls/8" {
		t.Fatalf("native outcome lost evidence: %+v %v", outcome, err)
	}
}

func TestMockAppServerThreadAndTurnContract(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "ws://" + listener.Addr().String()
	_ = listener.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestWorkerFlowMockCodexHelper$", "--", "app-server", "--listen", endpoint)
	cmd.Env = append(os.Environ(), "STEERLINE_WORKER_FLOW_MOCK=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	client, err := codexworker.ConnectAppServer(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var started struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	workspace := t.TempDir()
	if err := client.Call(ctx, "thread/start", map[string]any{"cwd": workspace, "dynamicTools": []any{codexapp.AskDynamicTool()}}, &started); err != nil {
		t.Fatal(err)
	}
	var turn struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := client.Call(ctx, "turn/start", map[string]any{"threadId": started.Thread.ID, "input": []map[string]string{{"type": "text", "text": "hello"}}}, &turn); err != nil {
		t.Fatal(err)
	}
	if started.Thread.ID != "00000000-0000-0000-0000-000000000008" || !codexworker.ValidNativeID(turn.Turn.ID) || turn.Turn.ID == started.Thread.ID {
		t.Fatalf("mock native identities are invalid: %s %s", started.Thread.ID, turn.Turn.ID)
	}
	for {
		select {
		case <-ctx.Done():
			t.Fatal("mock turn did not complete")
		case event := <-client.Notifications():
			if event.Method == "turn/completed" {
				return
			}
		}
	}
}
