package testprotocol

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunRunnerServiceFixture implements only the public, idle service contract for
// caller topology tests. It cannot accept a Run or start a native agent.
func RunRunnerServiceFixture() bool {
	version := os.Getenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION")
	if version == "" || len(os.Args) < 2 {
		return false
	}
	args := os.Args[1:]
	command := args[0]
	if (command == "service" || command == "environment-check") && len(args) > 1 {
		command += " " + args[1]
	}
	var stateDir string
	for i, arg := range args {
		if arg == "--state-dir" && i+1 < len(args) {
			stateDir = args[i+1]
		}
	}
	digest := strings.Repeat("a", 64)
	var data any
	var request struct {
		RequestID string `json:"request_id"`
		Input     struct {
			ExpectedInstanceID string `json:"expected_instance_id"`
		} `json:"input"`
	}
	if command == "service stop" {
		_ = json.NewDecoder(os.Stdin).Decode(&request)
	}
	switch command {
	case "environment-check inspect":
		raw, err := os.ReadFile(filepath.Join(stateDir, "idle-check-fixture.json"))
		if err != nil || json.Unmarshal(raw, &data) != nil {
			panic("invalid public check fixture")
		}
	case "service verify-stopped":
		var info map[string]any
		raw, err := os.ReadFile(filepath.Join(stateDir, "idle-fixture.json"))
		if err != nil || json.Unmarshal(raw, &info) != nil || info["status"] != "not_running" {
			panic("offline fixture writer has not stopped")
		}
		runs := []any{}
		if body, err := os.ReadFile(filepath.Join(stateDir, "caller-fixture.json")); err == nil {
			var fixture struct {
				Runs map[string]map[string]any `json:"runs"`
			}
			if json.Unmarshal(body, &fixture) != nil {
				panic("invalid caller fixture")
			}
			for id, run := range fixture.Runs {
				if run["stop_status"] != "stopped" {
					panic("fixture Run is not stopped")
				}
				runs = append(runs, map[string]any{"run_id": id, "input_digest": run["input_digest"], "resources_settled": true})
			}
		}
		data = map[string]any{"state_dir": stateDir, "resources_settled": true, "runs": runs, "checks": []any{}}
	case "version":
		data = map[string]any{"version": version, "protocols": []string{"steerrunner-cli/v1"}, "resource_version": 1, "resource_digest": digest}
	case "capabilities":
		if raw, err := os.ReadFile(filepath.Join(stateDir, "idle-capabilities-fixture.json")); err == nil {
			if json.Unmarshal(raw, &data) != nil {
				panic("invalid public capabilities fixture")
			}
			break
		}
		data = map[string]any{"protocol": "steerrunner-cli/v1", "version": version, "resource_version": 1, "resource_digest": digest,
			"native_session_viewer": map[string]any{"adapters": []string{"codex-app-server"}},
			"instance_binding":      map[string]bool{"expected_instance_id": true},
			"workflows":             []any{map[string]any{"id": "development", "version": 1, "supported_adapters": []string{"codex-app-server"}, "prompt_steps": []string{"implement.execute", "implement.report", "publish", "publish.report"}}},
			"configuration_schemas": map[string]any{"version": 1, "workflow": map[string]any{"type": "object"}, "run_configuration": map[string]any{"type": "object"}, "adapters": map[string]any{"codex-app-server": map[string]string{"type": "object"}}, "providers": map[string]any{"command": map[string]string{"type": "object"}}}}
	case "inspect", "events":
		// A static public observation, supplied only by the caller test. This
		// fixture never submits a Run or executes a workflow/native operation.
		raw, err := os.ReadFile(filepath.Join(stateDir, "idle-run-fixture.json"))
		if err != nil {
			panic("idle fixture has no public Run observation")
		}
		var inspection map[string]any
		if json.Unmarshal(raw, &inspection) != nil {
			panic("invalid public Run fixture")
		}
		data = inspection
		if command == "events" {
			data = map[string]any{"events": []any{}, "next_cursor": "idle-end"}
		}
	case "service start", "service status", "service stop":
		if !filepath.IsAbs(stateDir) {
			panic("fixture requires absolute owned state-dir")
		}
		path := filepath.Join(stateDir, "idle-fixture.json")
		info := map[string]any{"state_dir": stateDir, "status": "not_running", "protocol": "steerrunner-cli/v1"}
		if raw, err := os.ReadFile(path); err == nil {
			if err := json.Unmarshal(raw, &info); err != nil {
				panic(err)
			}
		}
		if command == "service start" && info["status"] != "ready" {
			delete(info, "last_shutdown")
			exe, err := os.Executable()
			if err != nil {
				panic(err)
			}
			raw, err := os.ReadFile(exe)
			if err != nil {
				panic(err)
			}
			sum := sha256.Sum256(raw)
			var id [16]byte
			if _, err := rand.Read(id[:]); err != nil {
				panic(err)
			}
			info["status"], info["instance_id"] = "ready", hex.EncodeToString(id[:])
			info["version"], info["resource_digest"], info["executable_sha256"] = version, digest, hex.EncodeToString(sum[:])
			if err := os.MkdirAll(stateDir, 0700); err != nil {
				panic(err)
			}
			raw, _ = json.Marshal(info)
			if err := os.WriteFile(path, raw, 0600); err != nil {
				panic(err)
			}
			if marker := os.Getenv("STEERLINE_TEST_RUNNER_START_ACCEPTED"); marker != "" {
				if err := os.WriteFile(marker, []byte("accepted before response"), 0600); err != nil {
					panic(err)
				}
				time.Sleep(10 * time.Second) // Parent shutdown cancels this CLI, losing its response.
			}
		}
		if command == "service stop" {
			if request.Input.ExpectedInstanceID != "" && request.Input.ExpectedInstanceID != info["instance_id"] {
				_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"protocol": "steerrunner-cli/v1", "ok": false, "request_id": request.RequestID, "data": nil, "error": map[string]string{"code": "stale_instance", "message": "original fixture instance changed"}})
				os.Exit(5)
			}
			info["last_shutdown"] = map[string]any{"instance_id": info["instance_id"], "request_id": request.RequestID, "state_dir": stateDir, "executable_sha256": info["executable_sha256"], "resources_settled": true}
			if raw, err := os.ReadFile(filepath.Join(stateDir, "caller-fixture.json")); err == nil {
				var fixture map[string]json.RawMessage
				if json.Unmarshal(raw, &fixture) != nil {
					panic("invalid caller fixture")
				}
				var runs map[string]map[string]any
				if json.Unmarshal(fixture["runs"], &runs) != nil {
					panic("invalid public Runs")
				}
				for _, run := range runs {
					run["stop_status"] = "stopped"
					run["new_inputs_allowed"] = false
					run["resume_required"] = true
				}
				fixture["runs"], _ = json.Marshal(runs)
				raw, _ = json.Marshal(fixture)
				if err := os.WriteFile(filepath.Join(stateDir, "caller-fixture.json"), raw, 0600); err != nil {
					panic(err)
				}
			}
			info["status"] = "not_running"
			raw, _ := json.Marshal(info)
			if err := os.WriteFile(path, raw, 0600); err != nil {
				panic(err)
			}
		}
		data = info
	default:
		if stateDir != "" {
			f, err := os.OpenFile(filepath.Join(stateDir, "business-dispatch.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err == nil {
				_, _ = f.WriteString(command + "\n")
				_ = f.Close()
			}
		}
		panic("idle Runner fixture cannot perform business operations: " + command)
	}
	var id any
	if request.RequestID != "" {
		id = request.RequestID
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"protocol": "steerrunner-cli/v1", "ok": true, "request_id": id, "data": data, "error": nil}); err != nil {
		panic(err)
	}
	return true
}
