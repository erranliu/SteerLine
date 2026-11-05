package discovery

import (
	"context"
	"encoding/json"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/localservice"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	repository "github.com/erranliu/SteerLine/web/server/executionconfig"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if file := os.Getenv("SL_CATALOG_FIXTURE"); file != "" && len(os.Args) > 1 {
		raw, _ := os.ReadFile(file)
		var caps map[string]any
		_ = json.Unmarshal(raw, &caps)
		response := map[string]any{"protocol": "steerrunner-cli/v1", "request_id": nil, "ok": true, "error": nil, "data": nil}
		switch os.Args[1] {
		case "version":
			response["data"] = map[string]any{"version": "fixture", "resource_digest": caps["resource_digest"]}
		case "capabilities":
			response["data"] = caps
		case "native-catalog":
			if log := os.Getenv("SL_CATALOG_LOG"); log != "" {
				f, _ := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
				if f != nil {
					f.WriteString("query\n")
					f.Close()
				}
			}
			var in runnercontract.NativeCatalogRequest
			_ = json.NewDecoder(os.Stdin).Decode(&in)
			switch os.Getenv("SL_CATALOG_MODE") {
			case "slow":
				time.Sleep(250 * time.Millisecond)
			case "hang":
				time.Sleep(time.Minute)
			case "invalid":
				os.Stdout.WriteString("not-json")
				os.Exit(0)
			case "disconnect":
				os.Exit(0)
			case "unsupported":
				response["ok"] = false
				response["error"] = map[string]string{"code": "unsupported_capability"}
				json.NewEncoder(os.Stdout).Encode(response)
				os.Exit(3)
			}
			exe, _ := os.Executable()
			body, _ := os.ReadFile(exe)
			result := runnercontract.NativeCatalogResult{Contract: runnercontract.NativeCatalogContract, Kind: in.Kind, Adapter: in.Adapter, Target: in.Target, Agent: in.Agent, State: "ready", Complete: true, ContextUnverified: true, ObservedAt: time.Now().UTC(), Origin: "probe", Source: runnercontract.NativeCatalogSource{Version: "fixture", ExecutableSHA256: cfg.Digest(body), ResourceDigest: caps["resource_digest"].(string), NativeVersion: "native-fixture"}}
			switch in.Kind {
			case "environments":
				rows := []runnercontract.NativeEnvironment{{ID: "one", Name: "Machine"}}
				result.Environments = &rows
			case "agents":
				rows := []runnercontract.NativeAgent{{ID: "pi", Directory: "registered"}, {ID: "dsh", Directory: "registered"}}
				result.Agents = &rows
			case "models":
				rows := []runnercontract.NativeModel{{ID: "provider/model", Label: "Model", Provider: "provider", Efforts: []runnercontract.NativeEffort{{Value: "high", Label: "High"}}}}
				result.Models = &rows
			}
			if os.Getenv("SL_CATALOG_MODE") == "foreign-source" {
				result.Source.ExecutableSHA256 = strings.Repeat("f", 64)
			}
			response["data"] = result
		default:
			os.Exit(m.Run())
		}
		json.NewEncoder(os.Stdout).Encode(response)
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func fixture(t *testing.T) (*Manager, Request) {
	t.Helper()
	home := t.TempDir()
	raw, err := os.ReadFile("../../../internal/runnerclient/testdata/workflow-capabilities.json")
	if err != nil {
		t.Fatal(err)
	}
	var caps map[string]any
	_ = json.Unmarshal(raw, &caps)
	caps["version"] = "fixture"
	caps["resource_digest"] = strings.Repeat("a", 64)

	raw, _ = json.Marshal(caps)
	file := filepath.Join(home, "catalog.json")
	if err = os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SL_CATALOG_FIXTURE", file)
	t.Setenv("SL_CATALOG_MODE", "")
	exe, _ := os.Executable()
	body, _ := os.ReadFile(exe)
	config := cfg.File[cfg.Pin]{Version: 3, Runner: cfg.Pin{Executable: exe, Version: "fixture", SHA256: cfg.Digest(body)}, DefaultProfile: "local", Profiles: []cfg.Profile{{ID: "local", Name: "Local", Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"executable":"codex.exe"}`)}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}}}}
	if err = localservice.WritePrivateFile(home, "execution.json", config); err != nil {
		t.Fatal(err)
	}
	m := New(repository.New(home))
	t.Cleanup(m.Close)
	return m, Request{Kind: "models", Adapter: "codex-app-server", Host: "local", ProfileID: "local"}
}
func TestDiscoverySharingCancellationAndFreshQueries(t *testing.T) {
	m, q := fixture(t)
	log := filepath.Join(m.Store.Home, "queries")
	t.Setenv("SL_CATALOG_LOG", log)
	t.Setenv("SL_CATALOG_MODE", "slow")
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			out, err := m.Query(context.Background(), q)
			if err != nil || out.State != "ready" || len(out.Models) != 1 {
				t.Errorf("query %v %v", out, err)
			}
		}()
	}
	close(start)
	wg.Wait()
	raw, _ := os.ReadFile(log)
	if strings.Count(string(raw), "query\n") != 1 {
		t.Fatal("in-flight query not shared", string(raw))
	}
	out, err := m.Query(context.Background(), q)
	raw, _ = os.ReadFile(log)
	if err != nil || out.Freshness != "fresh" || strings.Count(string(raw), "query\n") != 2 {
		t.Fatal("cross-request native cache reused", out, err)
	}
	t.Setenv("SL_CATALOG_MODE", "hang")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err = m.Query(ctx, q); err == nil {
		t.Fatal("subscriber cancellation ignored")
	}
	began := time.Now()
	m.Close()
	if time.Since(began) > 6*time.Second {
		t.Fatal("query did not drain")
	}
}
func TestDiscoveryRejectsForeignProfileTargetAndAgent(t *testing.T) {
	m, _ := fixture(t)
	for _, q := range []Request{{Kind: "models", Adapter: "orca-terminal", Host: "orca_environment"}, {Kind: "models", Adapter: "orca-terminal", Host: "local", Environment: "one"}, {Kind: "models", Host: "local"}, {Kind: "models", Adapter: "codex-app-server", Host: "local", ProfileID: "missing"}, {Kind: "models", Adapter: "orca-terminal", Host: "orca_environment", Environment: "one", ProfileID: "local"}} {
		if _, err := m.Query(context.Background(), q); err == nil {
			t.Fatal("foreign query accepted", q)
		}
	}
	out, err := m.Query(context.Background(), Request{Kind: "environments", Host: "orca_environment"})
	if err != nil || out.State != "ready" || len(out.Environments) != 1 {
		t.Fatal(out, err)
	}
}
func TestDiscoveryFailureKeepsOnlyExpiredPriorResult(t *testing.T) {
	m, q := fixture(t)
	if out, err := m.Query(context.Background(), q); err != nil || out.State != "ready" {
		t.Fatal(out, err)
	}
	for _, mode := range []string{"invalid", "disconnect", "foreign-source", "unsupported"} {
		t.Setenv("SL_CATALOG_MODE", mode)
		out, err := m.Query(context.Background(), q)
		if err != nil || out.Freshness != "expired" || out.Diagnostic == "" {
			t.Fatal(mode, out, err)
		}
	}
	q.ProfileID = ""
	q.Adapter = "orca-terminal"
	q.Host = "orca_environment"
	q.Environment = "new-target"
	q.Agent = "pi"
	out, err := m.Query(context.Background(), q)
	if err != nil || out.State != "unsupported" || len(out.Models) != 0 {
		t.Fatal("prior result crossed target", out, err)
	}
}
func TestDiscoveryPreservesRegisteredUnsupportedAgents(t *testing.T) {
	m, _ := fixture(t)
	out, err := m.Query(context.Background(), Request{Kind: "agents", Adapter: "orca-terminal", Host: "orca_environment", Environment: "one"})
	if err != nil || len(out.Agents) != 2 || out.Agents[1].ID != "dsh" {
		t.Fatal(out, err)
	}
}

func TestDiscoveryDiscardsResponseAfterConfigurationChanges(t *testing.T) {
	m, q := fixture(t)
	log := filepath.Join(m.Store.Home, "queries")
	t.Setenv("SL_CATALOG_LOG", log)
	t.Setenv("SL_CATALOG_MODE", "slow")
	done := make(chan Result, 1)
	go func() { out, _ := m.Query(context.Background(), q); done <- out }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if raw, _ := os.ReadFile(log); strings.Contains(string(raw), "query") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	config, err := m.Store.Current()
	if err != nil {
		t.Fatal(err)
	}
	config.Profiles[0].Name = "Changed during query"
	if err = localservice.WritePrivateFile(m.Store.Home, "execution.json", config); err != nil {
		t.Fatal(err)
	}
	out := <-done
	if out.State != "unavailable" || out.Freshness != "expired" || len(out.Models) != 0 || out.Diagnostic == "" {
		t.Fatal("late native result became current", out)
	}
}
