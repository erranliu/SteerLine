package testprotocol

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	if command == "service" && len(args) > 1 {
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
	}
	if command == "service stop" {
		_ = json.NewDecoder(os.Stdin).Decode(&request)
	}
	switch command {
	case "service verify-stopped":
		var info map[string]any
		raw, err := os.ReadFile(filepath.Join(stateDir, "idle-fixture.json"))
		if err != nil || json.Unmarshal(raw, &info) != nil || info["status"] != "not_running" {
			panic("offline fixture writer has not stopped")
		}
		data = map[string]any{"state_dir": stateDir, "resources_settled": true, "runs": []any{}}
	case "version":
		data = map[string]any{"version": version, "protocols": []string{"steerrunner-cli/v1"}, "resource_version": 1, "resource_digest": digest}
	case "capabilities":
		data = map[string]any{"protocol": "steerrunner-cli/v1", "version": version, "resource_version": 1, "resource_digest": digest,
			"native_session_viewer": map[string]any{"adapters": []string{"codex-app-server"}},
			"workflow":              map[string]any{"id": "development", "version": 1},
			"configuration_schemas": map[string]any{"version": 1, "adapters": map[string]any{"codex-app-server": map[string]string{"type": "object"}}, "providers": map[string]any{"command": map[string]string{"type": "object"}}}}
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
		}
		if command == "service stop" {
			info["last_shutdown"] = map[string]any{"instance_id": info["instance_id"], "request_id": request.RequestID, "state_dir": stateDir, "executable_sha256": info["executable_sha256"], "resources_settled": true}
			info["status"] = "not_running"
			raw, _ := json.Marshal(info)
			if err := os.WriteFile(path, raw, 0600); err != nil {
				panic(err)
			}
		}
		data = info
	default:
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
