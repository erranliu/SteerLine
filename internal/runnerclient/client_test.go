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

		if mode == "instance-race" {
			if len(os.Args) > 2 && os.Args[1] == "service" && os.Args[2] == "status" {
				_ = os.WriteFile(os.Getenv("STEERLINE_RACE_MARKER"), []byte("B"), 0600)
				info := ServiceInfo{InstanceID: "A", Status: "ready", StateDir: os.Getenv("STEERLINE_RACE_DIR"), Protocol: Protocol, ResourceDigest: "resources"}
				data, _ := json.Marshal(info)
				fmt.Printf(`{"protocol":"steerrunner-cli/v1","ok":true,"request_id":null,"data":%s,"error":null}`, data)
				os.Exit(0)
			}
			raw, _ := os.ReadFile(os.Getenv("STEERLINE_RACE_MARKER"))
			if string(raw) != "B" {
				os.Exit(1)
			}
			for i, arg := range os.Args {
				if arg == "--expected-instance-id" && i+1 < len(os.Args) && os.Args[i+1] == "A" {
					var id any
					if request.RequestID != "" {
						id = request.RequestID
					}
					_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"protocol": Protocol, "ok": false, "request_id": id, "data": nil, "error": map[string]string{"code": "stale_instance", "message": "restart SteerLine explicitly"}})
					os.Exit(5)
				}
			}
			_ = os.WriteFile(os.Getenv("STEERLINE_RACE_BUSINESS"), []byte("B received business"), 0600)
		}
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

func TestRunnerPresetFreeze(t *testing.T) {
	t.Run("reject_replacement_between_status_and_dispatch", func(t *testing.T) {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		business := filepath.Join(dir, "business")
		t.Setenv("STEERLINE_RUNNER_CLI_FIXTURE", "instance-race")
		t.Setenv("STEERLINE_RUNNER_CLI_CALL", filepath.Join(dir, "call"))
		t.Setenv("STEERLINE_RACE_DIR", dir)
		t.Setenv("STEERLINE_RACE_MARKER", filepath.Join(dir, "replacement"))
		t.Setenv("STEERLINE_RACE_BUSINESS", business)
		c := Client{Executable: exe, StateDir: dir, InstanceID: "A", ResourceDigest: "resources", Timeout: 5 * time.Second}
		for _, command := range []string{"submit", "reply", "resume", "stop", "inspect"} {
			var request *Request
			if command != "inspect" {
				request = &Request{Protocol: Protocol, RequestID: "original-" + strings.ReplaceAll(command, " ", "-"), Input: json.RawMessage(`{}`)}
			}
			_, err := c.Call(context.Background(), command, nil, request)
			var failure *Failure
			if !errors.As(err, &failure) || failure.Code != "stale_instance" || failure.Unknown {
				t.Fatalf("%s not fenced: %v", command, err)
			}
			if _, err := os.Stat(business); !os.IsNotExist(err) {
				t.Fatal("replacement received business", err)
			}
			if c.InstanceID != "A" {
				t.Fatal("old binding changed")
			}
		}
	})
}
