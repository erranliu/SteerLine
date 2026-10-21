package discovery

import (
	"bufio"
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
	if os.Getenv("SL_DISCOVERY_CHILD") == "1" && len(os.Args) > 1 && os.Args[1] == "app-server" {
		nativeFixture()
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func nativeFixture() {
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var q struct {
			ID     *int           `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		json.Unmarshal(scanner.Bytes(), &q)
		if log := os.Getenv("SL_DISCOVERY_LOG"); log != "" {
			f, e := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if e == nil {
				f.WriteString(q.Method + "\n")
				f.Close()
			}
		}
		if q.ID == nil {
			continue
		}
		var result any
		switch q.Method {
		case "initialize":
			result = map[string]string{"userAgent": "fixture/1"}
		case "model/list":
			if os.Getenv("SL_DISCOVERY_MODE") == "slow" {
				time.Sleep(100 * time.Millisecond)
			}
			switch os.Getenv("SL_DISCOVERY_MODE") {
			case "hang":
				time.Sleep(time.Minute)
			case "oversize":
				os.Stdout.WriteString(strings.Repeat("x", (1<<20)+1) + "\n")
				continue
			case "disconnect":
				os.Exit(0)
			case "invalid":
				os.Stdout.WriteString("not-json\n")
				continue
			}
			cursor := ""
			if os.Getenv("SL_DISCOVERY_MODE") == "cycle" || q.Params["cursor"] == nil {
				cursor = "page2"
			}
			id := "first"
			if q.Params["cursor"] != nil {
				id = "second"
			}
			result = map[string]any{"data": []any{map[string]any{"id": id, "model": id, "displayName": id, "supportedReasoningEfforts": []any{map[string]string{"reasoningEffort": "high", "description": "High"}}, "defaultReasoningEffort": "high"}}, "nextCursor": cursor}
		default:
			os.Exit(3)
		}
		encoder.Encode(map[string]any{"id": *q.ID, "result": result})
	}
}

func TestDiscoverySharingCancellationAndCache(t *testing.T) {
	t.Setenv("SL_DISCOVERY_CHILD", "1")
	t.Setenv("SL_DISCOVERY_MODE", "slow")
	home := t.TempDir()
	log := filepath.Join(home, "queries")
	t.Setenv("SL_DISCOVERY_LOG", log)
	exe, _ := os.Executable()
	settings, _ := json.Marshal(map[string]string{"executable": exe})
	c := cfg.File[cfg.Pin]{Version: 3, DefaultProfile: "local", Profiles: []cfg.Profile{{ID: "local", Name: "Local", Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: settings}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}}}}
	if err := localservice.WritePrivateFile(home, "execution.json", c); err != nil {
		t.Fatal(err)
	}
	manager := New(repository.New(home))
	defer manager.Close()
	q := Request{Kind: "models", Adapter: "codex-app-server", Host: "local", ProfileID: "local"}
	var wg sync.WaitGroup
	results := make(chan Result, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := manager.Query(context.Background(), q)
			if e != nil {
				t.Error(e)
			}
			results <- r
		}()
	}
	wg.Wait()
	close(results)
	for r := range results {
		if r.State != "ready" || len(r.Models) != 2 {
			t.Fatal(r)
		}
	}
	calls, _ := os.ReadFile(log)
	if strings.Count(string(calls), "initialize\n") != 1 {
		t.Fatal("duplicate native probe", string(calls))
	}
	cached, err := manager.Query(context.Background(), q)
	if err != nil || cached.Freshness != "cached" {
		t.Fatal(cached, err)
	}
	// Cache lifetime belongs to Web's monotonic insertion clock, not a remote
	// host's possibly skewed source timestamp.
	manager.mu.Lock()
	entry := manager.cache[cached.ContextKey]
	entry.storedAt = time.Now().Add(-6 * time.Minute)
	entry.result.FetchedAt = time.Now().Add(time.Hour)
	manager.cache[cached.ContextKey] = entry
	manager.mu.Unlock()
	fresh, e := manager.Query(context.Background(), q)
	if e != nil || fresh.Freshness != "fresh" {
		t.Fatal("source clock extended cache lifetime", fresh, e)
	}
	t.Setenv("SL_DISCOVERY_MODE", "hang")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	q.Refresh = true
	_, err = manager.Query(ctx, q)
	if err == nil {
		t.Fatal("canceled subscriber succeeded")
	}
	start := time.Now()
	manager.Close()
	if time.Since(start) > 6*time.Second {
		t.Fatal("unsubscribed native process remained active")
	}
}
func TestCodexCatalog(t *testing.T) {
	t.Setenv("SL_DISCOVERY_CHILD", "1")
	exe, _ := os.Executable()
	log := filepath.Join(t.TempDir(), "methods")
	t.Setenv("SL_DISCOVERY_LOG", log)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	models, version, err := codexModels(ctx, exe)
	if err != nil || len(models) != 2 || version != "fixture/1" || models[1].ID != "second" || models[0].Efforts[0].Value != "high" {
		t.Fatal(models, version, err)
	}
	methods, _ := os.ReadFile(log)
	if string(methods) != "initialize\ninitialized\nmodel/list\nmodel/list\n" {
		t.Fatal("unexpected native methods", string(methods))
	}
	for _, mode := range []string{"cycle", "invalid", "hang", "oversize", "disconnect"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SL_DISCOVERY_MODE", mode)
			ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
			defer cancel()
			started := time.Now()
			_, _, err := codexModels(ctx, exe)
			if err == nil || time.Since(started) > 6*time.Second {
				t.Fatal("invalid query or cleanup succeeded", err)
			}
		})
	}
}
func TestEnvironmentDirectory(t *testing.T) {
	m := New(nil)
	defer m.Close()
	m.OrcaData = t.TempDir()
	content := `{"version":1,"environments":[{"id":"stable","name":"机器","pairedDeviceId":"device","preferredEndpointId":"ep","endpoints":[{"id":"ep","endpoint":"ws://127.0.0.1:1","deviceToken":"secret","publicKeyB64":"private-pair-key"}]}]}`
	if err := os.WriteFile(filepath.Join(m.OrcaData, "orca-environments.json"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	rows, err := m.environments(context.Background(), "")
	if err != nil || len(rows) != 1 || rows[0].ID != "stable" {
		t.Fatal(rows, err)
	}
	public, _ := json.Marshal(rows)
	if strings.Contains(string(public), "secret") || strings.Contains(string(public), "127.0.0.1") {
		t.Fatal("pairing material leaked")
	}
	if _, err = m.environment("machine-not-registered"); err == nil {
		t.Fatal("foreign environment accepted")
	}
	byName, err := m.environment("机器")
	if err != nil || byName.ID != "stable" {
		t.Fatal(byName, err)
	}
}
