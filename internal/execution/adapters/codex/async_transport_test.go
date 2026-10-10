package codexworker_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/adapters/protocol/codexapp"
)

func TestAsyncQuestionObserveSteerAndReconcile(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "ws://" + listener.Addr().String()
	_ = listener.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestWorkerFlowMockCodexHelper$", "--", "app-server", "--listen", endpoint)
	cmd.Env = append(os.Environ(), "STEERLINE_WORKER_FLOW_MOCK=1", "STEERLINE_WORKER_FLOW_ASYNC=1")
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
	workspace := t.TempDir()
	thread, err := codexworker.StartThread(ctx, client, map[string]any{"cwd": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := codexworker.StartTurn(ctx, client, map[string]any{"threadId": thread.Thread.ID, "input": []map[string]string{{"type": "text", "text": "ask asynchronously"}}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, "appserver.jsonl")
	journal, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	asked := make(chan codexworker.ObservationEvent, 1)
	done := make(chan error, 1)
	go func() {
		completed, err := codexworker.Observe(ctx, client, journal, thread.Thread.ID, turn.Turn.ID, nil, func(e codexapp.Envelope) error {
			_, err := codexworker.ParseAskCall(e, thread.Thread.ID, turn.Turn.ID)
			return err
		}, func(e codexworker.ObservationEvent) bool {
			if e.Kind == "question" {
				asked <- e
			}
			return true
		})
		if err == nil && !completed {
			err = context.Canceled
		}
		done <- err
	}()
	select {
	case e := <-asked:
		if e.Status != "pending" || !e.Forward {
			t.Fatal("question not forwarded")
		}
	case <-ctx.Done():
		t.Fatal("question not observed")
	}
	command, message := "saved-answer-command", "Answer to original async item: worker"
	if err := codexworker.SteerAsyncAnswer(ctx, client, thread.Thread.ID, workspace, turn.Turn.ID, command, message); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	confirmed, err := codexworker.AsyncAnswerEvidence(path, thread.Thread.ID, turn.Turn.ID, command, message)
	if err != nil || !confirmed {
		t.Fatalf("saved answer not reconciled: %v %v", confirmed, err)
	}
	wrong, err := codexworker.AsyncAnswerEvidence(path, thread.Thread.ID, turn.Turn.ID, command, "other answer")
	if err != nil || wrong {
		t.Fatal("different payload confirmed")
	}
	if err := codexworker.SteerAsyncAnswer(ctx, client, thread.Thread.ID, workspace, turn.Turn.ID, "later-command", message); err == nil {
		t.Fatal("stopped turn accepted steering")
	}
	outcome, err := codexworker.ReadAppServerOutcome(path, thread.Thread.ID, turn.Turn.ID)
	if err != nil || outcome.LastMessage != "done after answer" {
		t.Fatalf("wrong final: %+v %v", outcome, err)
	}
}
