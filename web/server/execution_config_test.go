package server

import (
	"encoding/json"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("SL_WEB_CONFIG_META") == "1" && len(os.Args) > 1 {
		command := os.Args[1]
		log := os.Getenv("SL_WEB_CONFIG_LOG")
		if log != "" {
			f, _ := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if f != nil {
				f.WriteString(strings.Join(os.Args[1:], " ") + "\n")
				f.Close()
			}
		}
		switch command {
		case "version":
			json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "protocol": "steerrunner-cli/v1", "data": map[string]any{"version": "web-fixture", "resource_digest": strings.Repeat("a", 64)}})
			os.Exit(0)
		case "capabilities":
			if len(os.Args) != 4 || os.Args[2] != "--offline" || os.Args[3] != "--json" {
				os.Exit(3)
			}
			raw, err := os.ReadFile(os.Getenv("SL_WEB_CONFIG_SCHEMA"))
			if err != nil {
				os.Exit(4)
			}
			var caps map[string]any
			json.Unmarshal(raw, &caps)
			caps["version"] = "web-fixture"
			caps["resource_digest"] = strings.Repeat("a", 64)
			json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "protocol": "steerrunner-cli/v1", "data": caps})
			os.Exit(0)
		case "native-catalog":
			var input runnercontract.NativeCatalogRequest
			if json.NewDecoder(os.Stdin).Decode(&input) != nil {
				os.Exit(2)
			}
			exe, _ := os.Executable()
			body, _ := os.ReadFile(exe)
			models := []runnercontract.NativeModel{{ID: "fixture-model", Label: "Fixture", Efforts: []runnercontract.NativeEffort{{Value: "high", Label: "High"}}}}
			result := runnercontract.NativeCatalogResult{Contract: runnercontract.NativeCatalogContract, Kind: input.Kind, Adapter: input.Adapter, Target: input.Target, Agent: input.Agent, State: "ready", Complete: true, ContextUnverified: true, ObservedAt: time.Now().UTC(), Origin: "probe", Models: &models, Source: runnercontract.NativeCatalogSource{Version: "web-fixture", ExecutableSHA256: cfg.Digest(body), ResourceDigest: strings.Repeat("a", 64)}}
			json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "protocol": "steerrunner-cli/v1", "request_id": nil, "error": nil, "data": result})
			os.Exit(0)
		}

	}
	os.Exit(m.Run())
}
func TestWebExecutionConfigRoutes(t *testing.T) {
	t.Setenv("SL_WEB_CONFIG_META", "1")
	schema, _ := filepath.Abs("../../internal/runnerclient/testdata/workflow-capabilities.json")
	t.Setenv("SL_WEB_CONFIG_SCHEMA", schema)
	s, backend := fixture(t)
	cookie, csrf := login(t, s)
	exe, _ := os.Executable()
	raw, _ := os.ReadFile(exe)
	config := cfg.File[cfg.Pin]{Version: 3, Runner: cfg.Pin{Executable: exe, Version: "web-fixture", SHA256: cfg.Digest(raw)}, DefaultProfile: "local", Profiles: []cfg.Profile{{ID: "local", Name: "本机", Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"model":"fixture-model","reasoning_effort":"high","access_mode":"full_access"}`)}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}}}}
	if err := localservice.WritePrivateFile(s.options.Home, "execution.json", config); err != nil {
		t.Fatal(err)
	}
	const path = "/api/web/execution-config"
	if w := request(s, "GET", path, "", nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request(s, "POST", path+"/discover", `{"kind":"models","adapter":"codex-app-server","host":"local"}`, cookie, "wrong"); w.Code != 403 {
		t.Fatal(w.Code)
	}
	w := request(s, "GET", path, "", cookie, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"writable":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	var snap struct {
		Digest string `json:"saved_digest"`
	}
	json.Unmarshal(w.Body.Bytes(), &snap)
	if !strings.Contains(w.Body.String(), `"local_helper"`) || !strings.Contains(w.Body.String(), `"combinations"`) {
		t.Fatal("missing structured support contract")
	}
	for _, body := range []string{`{"kind":"models","host":"local"}`, `{"kind":"models","host":"local","adapter":"orca-terminal","api_key":"fixture-secret"}`, `{"kind":"models","host":"local","adapter":"orca-terminal","environment":"remote"}`, `{"kind":"models","host":"local","adapter":"orca-terminal","endpoint":"arbitrary"}`} {
		if got := request(s, "POST", path+"/discover", body, cookie, csrf); got.Code != 400 {
			t.Fatal("ambiguous or arbitrary native route accepted", got.Code)
		}
	}
	originalDigest := snap.Digest
	config.Profiles[0].Name = "新名称"
	body, _ := json.Marshal(config)
	send := func(etag string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PUT", "http://"+s.options.Address+path, strings.NewReader(string(body)))
		r.RemoteAddr = "127.0.0.1:11111"
		r.AddCookie(cookie)
		r.Header.Set("Origin", "http://"+s.options.Address)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		if etag != "" {
			r.Header.Set("If-Match", etag)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w = send(""); w.Code != 428 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = send(originalDigest); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = send(originalDigest); w.Code != 412 {
		t.Fatal(w.Code, w.Body.String())
	}
	backend.route = ""
	w = request(s, "GET", path+"/status", "", cookie, "")
	if w.Code != 200 || backend.route != "" {
		t.Fatal("configuration request reached execution service")
	}
	settings, _ := json.Marshal(map[string]string{"executable": exe, "model": "fixture-model", "access_mode": "full_access"})
	config.Profiles[0].Execution.Settings = settings
	if err := localservice.WritePrivateFile(s.options.Home, "execution.json", config); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"no-projects", "failed-project", "no-project-materials"} {
		if phase == "failed-project" {
			if err := os.MkdirAll(filepath.Join(s.options.Home, "etc"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(s.options.Home, "etc", "failed.json"), []byte("invalid project configuration"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		backend.route = ""
		w = request(s, "GET", path, "", cookie, "")
		var snapshot struct {
			Environments []cfg.Environment `json:"environments"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &snapshot); err != nil || w.Code != 200 || len(snapshot.Environments) != 1 || snapshot.Environments[0].ProfileIDs[0] != "local" {
			t.Fatal(phase, w.Code, w.Body.String(), err)
		}
		w = request(s, "POST", path+"/discover", `{"kind":"models","host":"local","adapter":"codex-app-server","profile_id":"local"}`, cookie, csrf)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "fixture-model") || backend.route != "" {
			t.Fatal(phase, "project-independent discovery failed", w.Code, w.Body.String(), backend.route)
		}
	}
	for _, body := range []string{
		`{"kind":"models","host":"local","adapter":"codex-app-server","profile_id":"missing"}`,
		`{"kind":"models","host":"orca_environment","environment":"other","adapter":"orca-terminal","profile_id":"local"}`,
		`{"kind":"models","host":"local","adapter":"codex-app-server","agent":"pi","profile_id":"local"}`,
	} {
		if got := request(s, "POST", path+"/discover", body, cookie, csrf); got.Code != 400 {
			t.Fatal("wrong profile binding accepted", got.Code, got.Body.String())
		}
	}
	for _, route := range []string{path + "/../projects", path + "/rpc", path + "?file=elsewhere"} {
		w = request(s, "GET", route, "", cookie, "")
		if w.Code == 200 {
			t.Fatal("unsupported route accepted", route)
		}
	}
	if w = request(s, "GET", "/settings/execution-configs", "", nil, ""); w.Code != 200 {
		t.Fatal("global SPA route unavailable", w.Code)
	}
	if w = request(s, "GET", "/settings", "", nil, ""); w.Code != 200 {
		t.Fatal("configuration SPA route unavailable", w.Code)
	}
}
