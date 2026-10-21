package server

import (
	"bufio"
	"encoding/json"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		case "app-server":
			sc := bufio.NewScanner(os.Stdin)
			for sc.Scan() {
				var q struct {
					ID     *int   `json:"id"`
					Method string `json:"method"`
				}
				json.Unmarshal(sc.Bytes(), &q)
				if q.ID == nil {
					continue
				}
				var result any
				switch q.Method {
				case "initialize":
					result = map[string]string{"userAgent": "fixture"}
				case "model/list":
					result = map[string]any{"data": []any{map[string]any{"id": "fixture-model", "model": "fixture-model", "supportedReasoningEfforts": []any{map[string]string{"reasoningEffort": "high"}}}}, "nextCursor": nil}
				default:
					os.Exit(5)
				}
				json.NewEncoder(os.Stdout).Encode(map[string]any{"id": *q.ID, "result": result})
			}
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
	config := cfg.File[cfg.Pin]{Version: 3, Runner: cfg.Pin{Executable: exe, Version: "web-fixture", SHA256: cfg.Digest(raw)}, DefaultProfile: "local", Profiles: []cfg.Profile{{ID: "local", Name: "本机", Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"model":"fixture-model","reasoning_effort":"high"}`)}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}}}}
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
	for _, body := range []string{`{"kind":"models","host":"local"}`, `{"kind":"models","host":"local","adapter":"orca-terminal","environment":"remote"}`, `{"kind":"models","host":"local","adapter":"orca-terminal","endpoint":"arbitrary"}`} {
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
	for _, route := range []string{path + "/../projects", path + "/rpc", path + "?file=elsewhere"} {
		w = request(s, "GET", route, "", cookie, "")
		if w.Code == 200 {
			t.Fatal("unsupported route accepted", route)
		}
	}
	if w = request(s, "GET", "/settings/execution-configs", "", nil, ""); w.Code != 200 {
		t.Fatal("global SPA route unavailable", w.Code)
	}
}
