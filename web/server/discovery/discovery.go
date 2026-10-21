// Package discovery is owned exclusively by the Web process. It reads native
// capability catalogs and never creates business sessions or execution input.
package discovery

import (
	"context"
	"encoding/json"
	"errors"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	repository "github.com/erranliu/SteerLine/web/server/executionconfig"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type Request struct {
	Kind        string `json:"kind"`
	Adapter     string `json:"adapter,omitempty"`
	ProfileID   string `json:"profile_id,omitempty"`
	Host        string `json:"host,omitempty"`
	Environment string `json:"environment,omitempty"`
	Agent       string `json:"agent,omitempty"`
	Refresh     bool   `json:"refresh,omitempty"`
}
type Effort struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
type Model struct {
	ID            string   `json:"id"`
	Label         string   `json:"label"`
	Default       bool     `json:"is_default"`
	DefaultEffort string   `json:"default_effort,omitempty"`
	Efforts       []Effort `json:"efforts"`
}
type Environment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Agent struct {
	ID        string `json:"id"`
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
	Directory string `json:"directory,omitempty"`
}
type Result struct {
	State             string        `json:"state"`
	Freshness         string        `json:"freshness"`
	Origin            string        `json:"origin"`
	FetchedAt         time.Time     `json:"fetched_at"`
	ContextKey        string        `json:"context_key"`
	ContextUnverified bool          `json:"context_unverified"`
	Scope             string        `json:"scope"`
	Diagnostic        string        `json:"diagnostic,omitempty"`
	Environments      []Environment `json:"environments,omitempty"`
	Agents            []Agent       `json:"agents,omitempty"`
	Models            []Model       `json:"models,omitempty"`
	Version           string        `json:"version,omitempty"`
	Channel           string        `json:"channel,omitempty"`
	RuntimeDigest     string        `json:"runtime_digest,omitempty"`
	Protocol          int           `json:"protocol,omitempty"`
}
type pending struct {
	subscribers int
	cancel      context.CancelFunc
	done        chan struct{}
	result      Result
	err         error
}
type cacheEntry struct {
	result   Result
	storedAt time.Time
}
type Manager struct {
	Store    *repository.Store
	OrcaData string
	mu       sync.Mutex
	cache    map[string]cacheEntry
	flights  map[string]*pending
	slots    chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	closed   bool
}

func New(store *repository.Store) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{Store: store, cache: map[string]cacheEntry{}, flights: map[string]*pending{}, slots: make(chan struct{}, 2), ctx: ctx, cancel: cancel}
}
func (m *Manager) Close() { m.mu.Lock(); m.closed = true; m.cancel(); m.mu.Unlock(); m.wg.Wait() }
func (m *Manager) await(ctx context.Context, flight *pending) (Result, error) {
	defer func() {
		m.mu.Lock()
		flight.subscribers--
		if flight.subscribers == 0 {
			flight.cancel()
		}
		m.mu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-flight.done:
		return flight.result, flight.err
	}
}
func (m *Manager) Query(ctx context.Context, q Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if q.Kind != "environments" && q.Kind != "agents" && q.Kind != "models" {
		return Result{}, errors.New("unsupported discovery query")
	}
	if q.Host != "local" && q.Host != "orca_environment" {
		return Result{}, errors.New("invalid discovery host")
	}
	if q.Kind != "environments" {
		if q.Adapter != "codex-app-server" && q.Adapter != "orca-terminal" {
			return Result{}, errors.New("explicit discovery adapter is required")
		}
		if q.Agent == "" {
			q.Agent = "codex"
		}
		if q.Host == "local" && q.Environment != "" {
			return Result{}, errors.New("local discovery cannot select an environment")
		}
		if q.Adapter == "codex-app-server" && (q.Host != "local" || q.Agent != "codex") || q.Adapter == "orca-terminal" && (q.Agent != "codex" && (q.Host != "local" || q.Agent != "pi" && q.Agent != "dsh")) {
			return Result{State: "unsupported", Diagnostic: "当前 Web 未映射此执行组合", ContextUnverified: true}, nil
		}
	}
	config, err := m.Store.Current()
	if err != nil {
		return Result{}, err
	}
	program := "codex"
	if runtime.GOOS == "windows" {
		program = "codex.exe"
	}
	orcaProgram := "orca"
	for _, p := range config.Profiles {
		if p.ID == q.ProfileID {
			var s struct {
				Executable string `json:"executable"`
				Transport  struct {
					Executable string `json:"executable"`
				} `json:"transport"`
			}
			json.Unmarshal(p.Execution.Settings, &s)
			if p.Execution.Adapter == "codex-app-server" && s.Executable != "" {
				program = s.Executable
			}
			if p.Execution.Adapter == "orca-terminal" && s.Transport.Executable != "" {
				orcaProgram = s.Transport.Executable
			}
		}
	}
	if q.Kind == "environments" || q.Host == "orca_environment" {
		program = orcaProgram
	}
	resolved := program
	var executableIdentity any = program
	if q.Host == "local" && q.Adapter == "codex-app-server" && q.Kind != "environments" {
		resolved, err = exec.LookPath(program)
		if err != nil {
			return Result{State: "unavailable", Diagnostic: "找不到已配置的原生程序", ContextUnverified: true}, nil
		}
		info, e := os.Stat(resolved)
		if e != nil {
			return Result{}, errors.New("native executable is unavailable")
		}
		executableIdentity = []any{resolved, info.Size(), info.ModTime()}
	}
	var env nativeEnvironment
	if q.Kind != "environments" && q.Host == "orca_environment" {
		env, err = m.environment(q.Environment)
		if err != nil {
			return Result{State: "unavailable", Diagnostic: err.Error(), ContextUnverified: true}, nil
		}
	}
	var local localRuntime
	if q.Kind != "environments" && q.Host == "local" && q.Adapter == "orca-terminal" {
		local, err = m.localRuntime()
		if err != nil {
			return Result{State: "unavailable", Diagnostic: err.Error(), ContextUnverified: true}, nil
		}
	}
	identity, _ := json.Marshal([]any{m.Store.Home, q.Kind, q.Host, q.Adapter, q.Environment, q.Agent, executableIdentity, env.pairingKey(), m.dataPath(), local.identity(), os.Getenv("CODEX_HOME")})
	key := cfg.Digest(identity)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Result{}, context.Canceled
	}
	entry, hasCache := m.cache[key]
	cached := entry.result
	if !q.Refresh && hasCache && time.Since(entry.storedAt) < 5*time.Minute {
		cached.Freshness = "cached"
		m.mu.Unlock()
		return cached, nil
	}
	if flight := m.flights[key]; flight != nil {
		flight.subscribers++
		m.mu.Unlock()
		return m.await(ctx, flight)
	}
	select {
	case m.slots <- struct{}{}:
	default:
		m.mu.Unlock()
		return Result{State: "unavailable", Diagnostic: "能力探测正忙，请稍后重试", ContextUnverified: true}, nil
	}
	queryCtx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	flight := &pending{done: make(chan struct{}), subscribers: 1, cancel: cancel}
	m.flights[key] = flight
	m.wg.Add(1)
	m.mu.Unlock()
	// A shared query belongs to Web, rather than the first browser subscriber.
	go func() {
		defer m.wg.Done()
		defer func() { <-m.slots }()
		defer cancel()
		result := Result{ContextKey: key, ContextUnverified: true, Freshness: "fresh", Scope: "原生默认账号与配置目录；未验证具体项目执行上下文"}
		var e error
		if q.Kind == "environments" {
			result.Environments, e = m.environments(queryCtx, resolved)
			result.State = "ready"
			result.Origin = "registered"
		} else if q.Host == "local" && q.Adapter == "orca-terminal" {
			result, e = localOrcaCatalog(queryCtx, local, q.Kind, q.Agent, result)
		} else if q.Host == "local" {
			if q.Kind == "agents" {
				result.Agents = []Agent{{ID: "codex", Supported: true}}
				result.State = "ready"
				result.Origin = "product"
			} else {
				result.Models, result.Version, e = codexModels(queryCtx, resolved)
				result.State = "ready"
				result.Origin = "probe"
			}
		} else {
			result, e = orcaCatalog(queryCtx, env, q.Kind, result)
		}
		if e != nil {
			result.State = "unavailable"
			result.Diagnostic = e.Error()
			if hasCache {
				result = cached
				result.Freshness = "expired"
				result.Diagnostic = e.Error()
			}
		}
		if result.FetchedAt.IsZero() {
			result.FetchedAt = time.Now().UTC()
		}
		m.mu.Lock()
		if e == nil && result.State == "ready" {
			if len(m.cache) >= 128 {
				for k := range m.cache {
					delete(m.cache, k)
					break
				}
			}
			m.cache[key] = cacheEntry{result: result, storedAt: time.Now()}
		}
		flight.result = result
		delete(m.flights, key)
		close(flight.done)
		m.mu.Unlock()
	}()
	return m.await(ctx, flight)
}
func (m *Manager) dataPath() string {
	if m.OrcaData != "" {
		return m.OrcaData
	}
	if p := os.Getenv("ORCA_USER_DATA_PATH"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "orca")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "orca")
	default:
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, "orca")
	}
}
