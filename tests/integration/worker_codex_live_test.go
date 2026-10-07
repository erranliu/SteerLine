package integration

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/adapters/protocol/codexapp"
)

// Run explicitly with STEERLINE_CODEX_APP_SERVER_LIVE=1. It uses a temporary
// workspace and tests the installed Codex app-server, not a protocol mock.
func TestCodexAppServerNativeQuestionLive(t *testing.T) {
	if os.Getenv("STEERLINE_CODEX_APP_SERVER_LIVE") != "1" {
		t.Skip("live Codex probe is opt-in")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "ws://" + listener.Addr().String()
	listener.Close()
	nativeExecutable, err := codexworker.NativeExecutable("codex")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(nativeExecutable, "app-server", "--listen", endpoint)
	cmd.Dir = os.TempDir()
	stderr, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	cmd.Stderr = stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	var client *codexapp.Client
	for i := 0; i < 100; i++ {
		client, err = codexapp.Open(ctx, endpoint)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var started struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	workspace, err := os.MkdirTemp("", "steerline-dynamic-tool-probe-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace)
	if err := client.Call(ctx, "thread/start", map[string]any{"cwd": workspace, "approvalPolicy": "never", "sandbox": "read-only", "model": "gpt-6-luna", "dynamicTools": []any{codexapp.AskDynamicTool()}}, &started); err != nil {
		t.Fatal(err)
	}
	var turn struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := client.Call(ctx, "turn/start", map[string]any{"threadId": started.Thread.ID, "input": []map[string]string{{"type": "text", "text": "Call steerline_ask with one question (id choice, header Choice, question Should I reply A or B?, options A and B, allow_free_text false). Wait for the answer. Then reply with the chosen letter only."}}, "clientUserMessageId": "steerline-live-question", "effort": "low"}, &turn); err != nil {
		t.Fatal(err)
	}
	t.Logf("native thread=%s turn=%s", started.Thread.ID, turn.Turn.ID)
	for {
		select {
		case <-ctx.Done():
			t.Fatal("native request did not arrive before timeout")
		case event, ok := <-client.Notifications():
			if !ok {
				t.Fatal("Codex connection ended before native request")
			}
			if event.Method == "item/tool/call" {
				var params struct {
					ThreadID  string          `json:"threadId"`
					TurnID    string          `json:"turnId"`
					Tool      string          `json:"tool"`
					Arguments json.RawMessage `json:"arguments"`
				}
				if err := json.Unmarshal(event.Params, &params); err != nil {
					t.Fatal(err)
				}
				if params.ThreadID != started.Thread.ID || params.TurnID != turn.Turn.ID || params.Tool != "steerline_ask" || len(params.Arguments) == 0 || len(event.ID) == 0 {
					t.Fatalf("native request identity invalid: %+v", event)
				}
				call, err := codexworker.ParseAskCall(event, started.Thread.ID, turn.Turn.ID)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("native request id=%s questions=%s", event.ID, event.Params)
				client.Close()
				secondListener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				secondEndpoint := "ws://" + secondListener.Addr().String()
				_ = secondListener.Close()
				secondCmd := exec.Command(nativeExecutable, "app-server", "--listen", secondEndpoint)
				secondCmd.Dir, secondCmd.Stderr = os.TempDir(), stderr
				if err := secondCmd.Start(); err != nil {
					t.Fatal(err)
				}
				defer func() { _ = secondCmd.Process.Kill(); _ = secondCmd.Wait() }()
				secondClient, err := codexworker.ConnectAppServer(ctx, secondEndpoint)
				if err != nil {
					t.Fatal(err)
				}
				defer secondClient.Close()
				secondWorkspace, err := os.MkdirTemp("", "steerline-parallel-probe-")
				if err != nil {
					t.Fatal(err)
				}
				defer os.RemoveAll(secondWorkspace)
				var secondThread struct {
					Thread struct {
						ID string `json:"id"`
					} `json:"thread"`
				}
				if err := secondClient.Call(ctx, "thread/start", map[string]any{"cwd": secondWorkspace, "approvalPolicy": "never", "sandbox": "read-only", "model": "gpt-6-luna"}, &secondThread); err != nil {
					t.Fatal(err)
				}
				var secondTurn struct {
					Turn struct {
						ID string `json:"id"`
					} `json:"turn"`
				}
				if err := secondClient.Call(ctx, "turn/start", map[string]any{"threadId": secondThread.Thread.ID, "input": []map[string]string{{"type": "text", "text": "Reply READY only."}}, "clientUserMessageId": "steerline-parallel-probe", "effort": "low"}, &secondTurn); err != nil {
					t.Fatal(err)
				}
				for waiting := true; waiting; {
					select {
					case <-ctx.Done():
						t.Fatal("independent Codex execution was blocked by the pending question")
					case next, ok := <-secondClient.Notifications():
						if !ok {
							t.Fatal("independent Codex execution disconnected")
						}
						if next.Method == "turn/completed" {
							var completed struct {
								Turn struct {
									ID     string `json:"id"`
									Status string `json:"status"`
								} `json:"turn"`
							}
							_ = json.Unmarshal(next.Params, &completed)
							if completed.Turn.ID != secondTurn.Turn.ID || completed.Turn.Status != "completed" {
								t.Fatalf("independent Codex execution did not finish: %s", next.Params)
							}
							waiting = false
						}
					}
				}
				t.Logf("parallel turn completed while first question remained pending: %s", secondTurn.Turn.ID)
				reconnected, err := codexapp.Open(ctx, endpoint)
				if err != nil {
					t.Fatal(err)
				}
				defer reconnected.Close()
				var resumed struct {
					Thread struct {
						ID string `json:"id"`
					} `json:"thread"`
				}
				if err := reconnected.Call(ctx, "thread/resume", map[string]any{"threadId": started.Thread.ID, "cwd": workspace, "excludeTurns": true}, &resumed); err != nil {
					t.Fatal(err)
				}
				if resumed.Thread.ID != started.Thread.ID {
					t.Fatal("resumed a different thread")
				}
				var pending struct {
					Thread struct {
						Turns []struct {
							ID     string `json:"id"`
							Status string `json:"status"`
						} `json:"turns"`
					} `json:"thread"`
				}
				if err := reconnected.Call(ctx, "thread/read", map[string]any{"threadId": started.Thread.ID, "includeTurns": true}, &pending); err != nil {
					t.Fatal(err)
				}
				foundPending := false
				for _, candidate := range pending.Thread.Turns {
					foundPending = foundPending || candidate.ID == turn.Turn.ID && candidate.Status == "inProgress"
				}
				if !foundPending {
					t.Fatal("first Codex turn did not remain pending during parallel execution")
				}
				replyClient, err := codexapp.Open(ctx, endpoint)
				if err != nil {
					t.Fatal(err)
				}
				defer replyClient.Close()
				if err := replyClient.Call(ctx, "thread/resume", map[string]any{"threadId": started.Thread.ID, "cwd": workspace, "excludeTurns": true}, &resumed); err != nil {
					t.Fatal(err)
				}
				if err := replyClient.ReplyRequest(event.ID, codexapp.AskToolResponse(map[string][]string{call.Questions[0].ID: {"A"}})); err != nil {
					t.Fatal(err)
				}
				for {
					select {
					case <-ctx.Done():
						t.Fatal("native tool response was not accepted after reconnect")
					case next, ok := <-reconnected.Notifications():
						if !ok {
							t.Fatal("reconnected Codex stream ended")
						}
						if next.Method == "item/completed" {
							t.Logf("native item completed: %s", next.Params)
						}
						if next.Method == "turn/completed" {
							t.Logf("completed after reconnect: %s", next.Params)
							var completed struct {
								Turn struct {
									ID     string `json:"id"`
									Status string `json:"status"`
								} `json:"turn"`
							}
							if err := json.Unmarshal(next.Params, &completed); err != nil || completed.Turn.ID != turn.Turn.ID || completed.Turn.Status != "completed" {
								t.Fatalf("first native turn did not complete: %s (%v)", next.Params, err)
							}
							var feedbackTurn struct {
								Turn struct {
									ID string `json:"id"`
								} `json:"turn"`
							}
							if err := reconnected.Call(ctx, "turn/start", map[string]any{"threadId": started.Thread.ID, "input": []map[string]string{{"type": "text", "text": "Feedback: reply ACK only."}}, "clientUserMessageId": "steerline-live-feedback", "effort": "low"}, &feedbackTurn); err != nil {
								t.Fatal(err)
							}
							if feedbackTurn.Turn.ID == "" || feedbackTurn.Turn.ID == turn.Turn.ID {
								t.Fatalf("feedback did not create a new turn on the original thread: %+v", feedbackTurn)
							}
							for {
								select {
								case <-ctx.Done():
									t.Fatal("feedback turn did not finish before timeout")
								case feedback, ok := <-reconnected.Notifications():
									if !ok {
										t.Fatal("connection ended during feedback turn")
									}
									if feedback.Method != "turn/completed" {
										continue
									}
									var done struct {
										ThreadID string `json:"threadId"`
										Turn     struct {
											ID     string `json:"id"`
											Status string `json:"status"`
										} `json:"turn"`
									}
									if err := json.Unmarshal(feedback.Params, &done); err != nil || done.ThreadID != started.Thread.ID || done.Turn.ID != feedbackTurn.Turn.ID || done.Turn.Status != "completed" {
										t.Fatalf("feedback turn did not complete on original thread: %s (%v)", feedback.Params, err)
									}
									t.Logf("feedback completed on original thread: %s", done.Turn.ID)
									return
								}
							}
						}
					}
				}
			}
			if event.Method == "turn/completed" {
				t.Fatalf("Codex completed without native request: %s", event.Params)
			}
		}
	}
}
