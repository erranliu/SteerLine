// Package executionconfig is the Web-owned execution configuration repository.
// It has no task, runtime, or resident Runner client dependency.
package executionconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/erranliu/SteerLine/internal/developerconfig"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/pathguard"
	privatefs "github.com/erranliu/SteerLine/internal/product/httpfs"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type Config = cfg.File[cfg.Pin]
type Error struct {
	Status        int
	Code, Message string
}

func (e *Error) Error() string                       { return e.Message }
func failure(status int, code, message string) error { return &Error{status, code, message} }

type Snapshot struct {
	DeveloperAvailable bool `json:"developer_available"`

	Config       Config                          `json:"config"`
	Environments []cfg.Environment               `json:"environments"`
	SavedDigest  string                          `json:"saved_digest"`
	Writable     bool                            `json:"writable"`
	Reason       string                          `json:"reason,omitempty"`
	Workflows    []runnercontract.WorkflowChoice `json:"workflows"`
	Efforts      []string                        `json:"efforts"`
	Combinations []Combination                   `json:"combinations"`
	Profiles     map[string]ProfileView          `json:"profile_views"`
}
type Store struct {
	Home       string
	mu         sync.Mutex
	catalog    *catalog
	catalogPin cfg.Pin
	// Tests replace the offline metadata reader, never the consumer or a task.
	ReadCatalog        func(context.Context, cfg.Pin) (*catalog, error)
	Configure          func(context.Context, cfg.Pin, runnercontract.ExecutionConfigurationRequest) (runnercontract.ExecutionConfigurationResult, error)
	configurationCache map[string]configurationValue
}

func New(home string) *Store { return &Store{Home: home} }

// Status reads the saved file identity without invoking native executables.
func (s *Store) Status() (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, raw, err := s.read()
	if err != nil {
		return nil, err
	}
	return map[string]string{"saved_digest": cfg.Digest(raw)}, nil
}
func (s *Store) read() (Config, []byte, error) {
	var c Config
	path := filepath.Join(s.Home, "execution.json")
	if !privatefs.PrivatePath(s.Home, path) {
		return c, nil, failure(400, "invalid_path", "execution configuration path is not private")
	}
	resolved, resolveErr := pathguard.ResolveProspective(path)
	absolute, _ := filepath.Abs(path)
	samePath := filepath.Clean(resolved) == filepath.Clean(absolute)
	if runtime.GOOS == "windows" {
		samePath = strings.EqualFold(filepath.Clean(resolved), filepath.Clean(absolute))
	}
	if resolveErr != nil || !samePath {
		return c, nil, failure(400, "invalid_path", "linked configuration paths are unsupported")
	}
	f, err := privatefs.OpenRegularSnapshot(path)
	if err != nil {
		return c, nil, failure(503, "configuration_unavailable", "cannot read execution.json")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return c, nil, failure(400, "invalid_configuration", "execution.json is unreadable or exceeds 1 MiB")
	}
	if err = cfg.Decode(raw, &c); err != nil {
		return c, nil, failure(400, "invalid_configuration", err.Error())
	}
	if _, err = cfg.GenerateConcurrency(c.MaxGenerateConcurrency); err != nil {
		return c, nil, failure(400, "invalid_configuration", err.Error())
	}
	if err = cfg.Validate(c.Version, c.DefaultProfile, c.Profiles); err != nil {
		return c, nil, failure(400, "invalid_configuration", err.Error())
	}
	if _, err = cfg.DecodeUsagePolicy(c.UsagePolicy); err != nil {
		return c, nil, failure(400, "invalid_usage_policy", err.Error())
	}
	return c, raw, nil
}
func (s *Store) Current() (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, _, err := s.read()
	return c, err
}
func (s *Store) writeReason() string {
	// Generator packages freeze their scheme on first start. Conservatively keep
	// every generated package read-only, including a stopped package with history.
	for _, p := range []string{filepath.Join(s.Home, "..", "manifest.json"), filepath.Join(s.Home, "manifest.json")} {
		if info, err := os.Lstat(p); err == nil && info.Mode().IsRegular() {
			return "调试包执行方案已冻结，请生成新包以更换配置"
		}
	}
	return ""
}
func (s *Store) Snapshot(ctx context.Context) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshConfigurationFailures()
	c, raw, err := s.read()
	if err != nil {
		return Snapshot{}, err
	}
	out := Snapshot{DeveloperAvailable: developerconfig.Available, Config: c, Environments: cfg.Environments(c.Profiles), SavedDigest: cfg.Digest(raw), Writable: s.writeReason() == "", Reason: s.writeReason(), Efforts: []string{}, Workflows: []runnercontract.WorkflowChoice{}}
	cat, err := s.contract(ctx, c.Runner)
	if err != nil {
		out.Writable = false
		out.Reason = err.Error()
		return out, nil
	}
	out.Workflows = cat.Workflows
	out.Efforts = cat.Efforts
	out.Combinations = cat.Combinations
	out.Profiles = s.profileViews(ctx, c)
	return out, nil
}
func (s *Store) Validate(ctx context.Context, raw []byte) (Config, error) {
	if err := s.validateSteerAgentEdit(ctx, raw); err != nil {
		return Config{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.validate(ctx, raw)
}
func (s *Store) validate(ctx context.Context, raw []byte) (Config, error) {
	s.refreshConfigurationFailures()
	var proposed Config
	var edit EditRequest
	if len(raw) > 1<<20 {
		return proposed, failure(413, "configuration_too_large", "configuration exceeds 1 MiB")
	}
	if err := cfg.Decode(raw, &edit); err != nil {
		return proposed, failure(400, "invalid_configuration", err.Error())
	}
	proposed = edit.Config
	if _, err := cfg.SteerAgentSettings(proposed.SteerAgent); err != nil {
		return proposed, failure(400, "invalid_steer_agent", err.Error())
	}
	if err := proposed.Developer.Validate(); err != nil {
		return proposed, failure(400, "developer_disabled", err.Error())
	}
	current, savedRaw, err := s.read()
	if err != nil {
		return proposed, err
	}
	if edit.SavedDigest == "" || edit.SavedDigest != cfg.Digest(savedRaw) {
		return proposed, failure(412, "configuration_changed", "配置已变化，请重新读取后合并")
	}
	if proposed.Version != current.Version || proposed.Runner != current.Runner {
		return proposed, failure(400, "deployment_read_only", "Runner deployment pin cannot be changed here")
	}
	if _, err = cfg.GenerateConcurrency(proposed.MaxGenerateConcurrency); err != nil {
		return proposed, failure(400, "invalid_configuration", err.Error())
	}
	if err = cfg.Validate(proposed.Version, proposed.DefaultProfile, proposed.Profiles); err != nil {
		return proposed, failure(400, "invalid_configuration", err.Error())
	}
	_, policyErr := cfg.DecodeUsagePolicy(proposed.UsagePolicy)
	if policyErr != nil {
		return proposed, failure(400, "invalid_usage_policy", policyErr.Error())
	}
	cat, err := s.contract(ctx, current.Runner)
	if err != nil {
		return proposed, failure(503, "unsupported_contract", err.Error())
	}
	if err = s.verifyEdits(ctx, current, proposed, edit.ProfileEdits); err != nil {
		return proposed, err
	}
	for _, p := range proposed.Profiles {
		if err = cat.Validate(p); err != nil {
			return proposed, failure(400, "invalid_profile", fmt.Sprintf("%s: %s", p.Name, err))
		}
	}

	return proposed, nil
}

func sameExecution(a, b cfg.Profile) bool {
	return sameValue(a.Execution, b.Execution) && sameValue(a.Workflow, b.Workflow)
}

func sameValue(a, b any) bool {
	ra, _ := json.Marshal(a)
	rb, _ := json.Marshal(b)
	ca, ea := runnercontract.CanonicalJSON(ra)
	cb, eb := runnercontract.CanonicalJSON(rb)
	return ea == nil && eb == nil && bytes.Equal(ca, cb)
}
func (s *Store) Save(ctx context.Context, expected string, raw []byte) (Snapshot, error) {
	if err := s.validateSteerAgentEdit(ctx, raw); err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if expected == "" {
		return Snapshot{}, failure(428, "precondition_required", "If-Match is required")
	}
	current, old, err := s.read()
	if err != nil {
		return Snapshot{}, err
	}
	if cfg.Digest(old) != strings.Trim(expected, "\"") {
		return Snapshot{}, failure(412, "configuration_changed", "配置已被其他页面修改，请重新读取后合并")
	}
	if reason := s.writeReason(); reason != "" {
		return Snapshot{}, failure(409, "configuration_frozen", reason)
	}
	var edit EditRequest
	if err = cfg.Decode(raw, &edit); err != nil {
		return Snapshot{}, failure(400, "invalid_configuration", err.Error())
	}
	if edit.SavedDigest != strings.Trim(expected, "\"") {
		return Snapshot{}, failure(412, "configuration_changed", "If-Match 与 saved_digest 不同")
	}
	c, err := s.validate(ctx, raw)
	if err != nil {
		return Snapshot{}, err
	}
	digest := cfg.Digest(old)
	if !sameValue(current, c) {
		target := filepath.Join(s.Home, "execution.json")
		resolved, err := pathguard.ResolveProspective(target)
		if err != nil || !pathguard.Within(s.Home, resolved) {
			return Snapshot{}, failure(400, "invalid_path", "configuration path changed")
		}
		f, err := privatefs.CreatePrivateTemp(s.Home, ".execution-")
		if err != nil {
			return Snapshot{}, failure(500, "save_failed", "cannot create private configuration file")
		}
		defer os.Remove(f.Name())
		body, err := json.MarshalIndent(c, "", "  ")
		if err == nil {
			body = append(body, '\n')
			_, err = f.Write(body)
		}
		if err == nil {
			err = f.Sync()
		}
		err = errors.Join(err, f.Close())
		if err != nil {
			return Snapshot{}, failure(500, "save_failed", "configuration was not published")
		}
		var published []byte
		err = privatefs.WithConfigurationLock(s.Home, func() error {
			_, latest, err := s.read()
			if err != nil || !bytes.Equal(latest, old) {
				return failure(412, "configuration_changed", "configuration changed before publication")
			}
			if err = ctx.Err(); err != nil {
				return err
			}
			if err = privatefs.ReplacePrivateFile(f.Name(), target); err != nil {
				return failure(500, "save_unverified", "保存结果未核实，请重新读取配置")
			}
			_, published, err = s.read()
			if err != nil || !bytes.Equal(published, body) {
				return failure(500, "save_unverified", "保存结果未核实，请重新读取配置")
			}
			return nil
		})
		if err != nil {
			return Snapshot{}, err
		}

		digest = cfg.Digest(published)
	}
	return Snapshot{DeveloperAvailable: developerconfig.Available, Config: c, Environments: cfg.Environments(c.Profiles), SavedDigest: digest, Writable: true, Workflows: s.catalog.Workflows, Efforts: s.catalog.Efforts, Combinations: s.catalog.Combinations, Profiles: s.profileViews(ctx, c)}, nil
}
