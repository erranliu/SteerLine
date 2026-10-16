package runnerclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv("STEERLINE_RUNNER_CLI_FIXTURE"); mode != "" {
		body, _ := io.ReadAll(os.Stdin)
		_ = os.WriteFile(os.Getenv("STEERLINE_RUNNER_CLI_CALL"), append([]byte(strings.Join(os.Args[1:], "\n")+"\n--stdin--\n"), body...), 0600)
		var request Request
		_ = json.Unmarshal(body, &request)
		if mode == "check-response" {
			fmt.Print(strings.ReplaceAll(os.Getenv("STEERLINE_CHECK_RESPONSE"), "__REQUEST_ID__", request.RequestID))
			code := 0
			if os.Getenv("STEERLINE_CHECK_ERROR") != "" {
				code = 5
			}
			os.Exit(code)
		}
		if mode == "timeout" {
			time.Sleep(3 * time.Second)
			os.Exit(0)
		}
		if mode == "broken" {
			fmt.Println("diagnostic before JSON")
			os.Exit(0)
		}
		if mode == "wrong-id" {
			request.RequestID = "another"
		}
		if strings.HasPrefix(mode, "native") {
			if mode == "native-broken" {
				fmt.Println("broken response")
				os.Exit(0)
			}
			session := "session"
			if mode == "native-wrong" {
				session = "another"
			}
			fmt.Printf(`{"protocol":"steerrunner-cli/v1","ok":true,"request_id":null,"data":{"run_id":"run","attempt_id":"attempt","session_id":%q,"generation":"generation","opened":true},"error":null}`, session)
			os.Exit(0)
		}
		if len(os.Args) > 1 && os.Args[1] == "version" {
			fmt.Printf(`{"protocol":"steerrunner-cli/v1","ok":true,"request_id":null,"data":{"version":"fixture","protocols":["steerrunner-cli/v1"]},"error":null}`)
			os.Exit(0)
		}
		if mode == "rejected" {
			fmt.Printf(`{"protocol":"steerrunner-cli/v1","ok":false,"request_id":%q,"data":null,"error":{"code":"stale_revision","message":"Run changed"}}`, request.RequestID)
			os.Exit(5)
		}
		fmt.Printf(`{"protocol":"steerrunner-cli/v1","ok":true,"request_id":%q,"data":{"command_id":"command","run_id":"run","receipt_status":"accepted","action_status":"pending"},"error":null}`, request.RequestID)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestSteerRunnerCLIContract(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	callPath := filepath.Join(t.TempDir(), "call.json")
	t.Setenv("STEERLINE_RUNNER_CLI_CALL", callPath)
	request := Request{Protocol: Protocol, RequestID: "fixed-id", Input: json.RawMessage(`{"text":"中文\nwith spaces"}`)}
	for _, mode := range []string{"accepted", "rejected", "broken", "wrong-id", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("STEERLINE_RUNNER_CLI_FIXTURE", mode)
			client := Client{Executable: exe, StateDir: filepath.Join(t.TempDir(), "runner with spaces"), Timeout: 200 * time.Millisecond}
			if mode != "timeout" {
				client.Timeout = 5 * time.Second
			}
			receipt, err := client.Submit(context.Background(), request)
			if mode == "accepted" {
				if err != nil || receipt.RunID != "run" || receipt.ActionStatus != "pending" {
					t.Fatal(receipt, err)
				}
				body, _ := os.ReadFile(callPath)
				if !strings.Contains(string(body), "--input\n-\n") || !strings.Contains(string(body), "中文") || !strings.Contains(string(body), client.StateDir) {
					t.Fatal("argv/stdin contract changed")
				}
				return
			}
			var failure *Failure
			if !errors.As(err, &failure) {
				t.Fatalf("missing classified failure: %v", err)
			}
			if mode == "rejected" {
				if failure.Unknown || failure.Exit != 5 {
					t.Fatal(failure)
				}
			} else if !failure.Unknown || failure.Exit != 6 {
				t.Fatal("mutation outcome was guessed", failure)
			}
		})
	}
	t.Setenv("STEERLINE_RUNNER_CLI_FIXTURE", "accepted")
	if _, err := (Client{Executable: exe}).Version(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err = (Client{Executable: filepath.Join(t.TempDir(), "absent.exe")}).Submit(context.Background(), request)
	var failure *Failure
	if !errors.As(err, &failure) || failure.Exit != 4 || failure.Unknown {
		t.Fatal("pre-start failure became unknown", err)
	}
}

func TestSteerRunnerAmbiguousEnvelope(t *testing.T) {
	valid := `{"protocol":"steerrunner-cli/v1","ok":true,"request_id":null,"data":{},"error":null}`
	for _, body := range []string{strings.Replace(valid, `"ok":true`, `"ok":null`, 1), strings.Replace(valid, `"ok":true`, `"ok":true,"ok":false`, 1), valid + "\n" + valid, strings.Replace(valid, `"protocol":"steerrunner-cli/v1"`, `"protocol":null`, 1), strings.Replace(valid, `"error":null`, `"extra":1,"error":null`, 1)} {
		var response Response
		if decodeEnvelope([]byte(body), &response) == nil {
			t.Fatal("ambiguous envelope accepted", body)
		}
	}
}
