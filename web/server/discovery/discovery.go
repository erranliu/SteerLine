// Package discovery owns Web query subscriptions and product projections.
package discovery

import (
	"context"
	"encoding/json"
	"errors"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	repository "github.com/erranliu/SteerLine/web/server/executionconfig"
	"regexp"
	"sync"
	"time"
)

var agentID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

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
	Provider      string   `json:"provider,omitempty"`
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
type Manager struct {
	Store    *repository.Store
	mu       sync.Mutex
	previous map[string]Result
	flights  map[string]*pending
	slots    chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	closed   bool
}

func New(store *repository.Store) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{Store: store, previous: map[string]Result{}, flights: map[string]*pending{}, slots: make(chan struct{}, 2), ctx: ctx, cancel: cancel}
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
		if q.Host == "orca_environment" && q.Environment == "" {
			return Result{}, errors.New("remote discovery requires an explicit environment")
		}
		if !agentID.MatchString(q.Agent) {
			return Result{}, errors.New("invalid discovery agent")
		}
	}
	config, err := m.Store.Current()
	if err != nil {
		return Result{}, err
	}
	input := runnercontract.NativeCatalogRequest{Kind: q.Kind}
	if q.Kind != "environments" {
		input.Adapter = q.Adapter
		input.Target = &runnercontract.ExecutionTarget{Kind: q.Host, Environment: q.Environment}
		if q.Kind == "models" {
			input.Agent = q.Agent
		}
	}
	if q.ProfileID != "" {
		found := false
		for _, profile := range config.Profiles {
			if profile.ID != q.ProfileID {
				continue
			}
			var settings struct {
				Executable string `json:"executable"`
				Agent      string `json:"agent"`
			}
			if err = json.Unmarshal(profile.Execution.Settings, &settings); err != nil {
				return Result{}, err
			}
			if settings.Agent == "" || profile.Execution.Adapter == "codex-app-server" {
				settings.Agent = "codex"
			}
			if q.Kind == "environments" || profile.Execution.Adapter != q.Adapter || profile.Execution.Target.Kind != q.Host || profile.Execution.Target.Environment != q.Environment || settings.Agent != q.Agent {
				return Result{}, errors.New("discovery profile and target/agent differ")
			}
			found = true
			if q.Kind == "models" && q.Adapter == "codex-app-server" {
				input.Executable = settings.Executable
			}
			break
		}
		if !found {
			return Result{}, errors.New("discovery profile does not exist")
		}
	}
	if q.Kind != "environments" && q.Adapter == "codex-app-server" && (q.Host != "local" || q.Agent != "codex") {
		return Result{State: "unsupported", Origin: "unknown", ContextUnverified: true}, nil
	}
	snapshot, _ := json.Marshal(config)
	identity, _ := json.Marshal([]any{m.Store.Home, string(snapshot), input})
	key := cfg.Digest(identity)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Result{}, context.Canceled
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
	previous, hasPrevious := m.previous[key]
	m.flights[key] = flight
	m.wg.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wg.Done()
		defer func() { <-m.slots }()
		defer cancel()
		out := Result{State: "unavailable", Origin: "unknown", ContextKey: key, ContextUnverified: true, Freshness: "fresh", Scope: "原生目录查询，未验证方案的账号、权限或执行资格"}
		var queryErr error
		if q.Adapter == "codex-app-server" && q.Kind == "agents" {
			out.Agents = []Agent{{ID: "codex", Directory: "product"}}
			out.State = "ready"
			out.Origin = "product"
			out.FetchedAt = time.Now().UTC()
		} else {
			var result runnercontract.NativeCatalogResult
			result, queryErr = m.Store.QueryNativeCatalog(queryCtx, config.Runner, input)
			if queryErr == nil {
				out.State = result.State
				if out.State == "loading" {
					out.State = "in_progress"
				}
				out.Origin = result.Origin
				out.FetchedAt = result.ObservedAt
				if result.NativeFetchedAt != nil {
					out.FetchedAt = *result.NativeFetchedAt
				}
				out.Version = result.Source.NativeVersion
				out.Protocol = result.Source.Protocol
				if result.Source.RuntimeID != "" {
					out.RuntimeDigest = cfg.Digest([]byte(result.Source.RuntimeID))
				}
				if q.Adapter == "orca-terminal" {
					out.Channel = "paired-websocket"
					if q.Host == "local" {
						out.Channel = "named-pipe"
					}
				}
				if result.Environments != nil {
					out.Environments = []Environment{}
					for _, e := range *result.Environments {
						out.Environments = append(out.Environments, Environment{ID: e.ID, Name: e.Name})
					}
				}
				if result.Agents != nil {
					out.Agents = []Agent{}
					for _, a := range *result.Agents {
						out.Agents = append(out.Agents, Agent{ID: a.ID, Directory: a.Directory})
					}
				}
				if result.Models != nil {
					out.Models = []Model{}
					for _, model := range *result.Models {
						efforts := []Effort{}
						for _, e := range model.Efforts {
							efforts = append(efforts, Effort{Value: e.Value, Label: e.Label})
						}
						out.Models = append(out.Models, Model{ID: model.ID, Label: model.Label, Provider: model.Provider, Default: model.Default, DefaultEffort: model.DefaultEffort, Efforts: efforts})
					}
				}
			}
		}
		latest, e := m.Store.Current()
		latestRaw, _ := json.Marshal(latest)
		if e != nil || string(latestRaw) != string(snapshot) {
			queryErr = errors.New("配置已变化，请重新探测")
			hasPrevious = false
			out = Result{State: "unavailable", Freshness: "expired", Origin: "unknown", ContextUnverified: true, ContextKey: key}
		}
		if queryErr != nil {
			out.Diagnostic = queryErr.Error()
			var unsupported *repository.Error
			if errors.As(queryErr, &unsupported) && unsupported.Code == "unsupported_capability" {
				out.State = "unsupported"
			}
			if hasPrevious {
				out = previous
				out.Freshness = "expired"
				out.Diagnostic = queryErr.Error()
			}
		}
		m.mu.Lock()
		if queryErr == nil && out.State == "ready" {
			if len(m.previous) >= 128 {
				for k := range m.previous {
					delete(m.previous, k)
					break
				}
			}
			m.previous[key] = out
		}
		flight.result = out
		delete(m.flights, key)
		close(flight.done)
		m.mu.Unlock()
	}()
	return m.await(ctx, flight)
}
