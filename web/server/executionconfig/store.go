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
	Config       Config                          `json:"config"`
	SavedDigest  string                          `json:"saved_digest"`
	Writable     bool                            `json:"writable"`
	Reason       string                          `json:"reason,omitempty"`
	Workflows    []runnercontract.WorkflowChoice `json:"workflows"`
	Efforts      []string                        `json:"efforts"`
	Combinations []Combination                   `json:"combinations"`
	LocalHelper  HelperStatus                    `json:"local_helper"`
}
type Store struct {
	Home       string
	mu         sync.Mutex
	catalog    *catalog
	catalogPin cfg.Pin
	// Tests replace the offline metadata reader, never the consumer or a task.
	ReadCatalog func(context.Context, cfg.Pin) (*catalog, error)
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
	c, raw, err := s.read()
	if err != nil {
		return Snapshot{}, err
	}
	out := Snapshot{Config: c, SavedDigest: cfg.Digest(raw), Writable: s.writeReason() == "", Reason: s.writeReason(), Efforts: []string{}, Workflows: []runnercontract.WorkflowChoice{}}
	cat, err := s.contract(ctx, c.Runner)
	if err != nil {
		out.Writable = false
		out.Reason = err.Error()
		return out, nil
	}
	out.Workflows = cat.Workflows
	out.Efforts = cat.Efforts
	out.Combinations = cat.Combinations
	out.LocalHelper = helperStatus(c)
	return out, nil
}
func (s *Store) Validate(ctx context.Context, raw []byte) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.validate(ctx, raw)
}
func (s *Store) validate(ctx context.Context, raw []byte) (Config, error) {
	var proposed Config
	if len(raw) > 1<<20 {
		return proposed, failure(413, "configuration_too_large", "configuration exceeds 1 MiB")
	}
	if err := cfg.Decode(raw, &proposed); err != nil {
		return proposed, failure(400, "invalid_configuration", err.Error())
	}
	current, _, err := s.read()
	if err != nil {
		return proposed, err
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
	cat, err := s.contract(ctx, current.Runner)
	if err != nil {
		return proposed, failure(503, "unsupported_contract", err.Error())
	}
	for _, p := range proposed.Profiles {
		if err = Supported(p); err != nil {
			// Existing unsupported profiles can be preserved exactly or removed, but
			// neither created nor rewritten by an editor without a parameter mapping.
			preserved := false
			for _, old := range current.Profiles {
				if old.ID == p.ID && sameValue(old, p) {
					preserved = true
				}
			}
			if !preserved {
				return proposed, failure(400, "unsupported_profile", fmt.Sprintf("%s: %s", p.Name, err))
			}
		}
		if err = cat.Validate(p); err != nil {
			return proposed, failure(400, "invalid_profile", fmt.Sprintf("%s: %s", p.Name, err))
		}
		if err = programsUnchanged(current, p); err != nil {
			return proposed, err
		}
	}
	return proposed, nil
}

type nativeCommand struct {
	Executable        string   `json:"executable"`
	Args              []string `json:"args,omitempty"`
	RuntimeExecutable string   `json:"runtime_executable,omitempty"`
	RuntimeSHA        string   `json:"runtime_sha256,omitempty"`
}
type nativePrograms struct {
	Executable  string        `json:"executable"`
	Agent       string        `json:"agent"`
	Transport   nativeCommand `json:"transport"`
	AgentLaunch nativeCommand `json:"agent_launch"`
}

func programs(p cfg.Profile) nativePrograms {
	var s nativePrograms
	json.Unmarshal(p.Execution.Settings, &s)
	if s.Agent == "" {
		s.Agent = "codex"
	}
	return s
}
func sameExecution(a, b cfg.Profile) bool {
	return sameValue(a.Execution, b.Execution) && sameValue(a.Workflow, b.Workflow)
}
func programsUnchanged(current Config, p cfg.Profile) error {
	for _, old := range current.Profiles {
		if old.ID == p.ID && sameExecution(old, p) {
			return nil
		}
	}
	next := programs(p)
	if p.Execution.Adapter == "codex-app-server" {
		if next.Executable == "" || next.Executable == "codex" || next.Executable == "codex.exe" {
			return nil
		}
		for _, old := range current.Profiles {
			if old.Execution.Adapter == p.Execution.Adapter && programs(old).Executable == next.Executable {
				return nil
			}
		}
	} else if p.Execution.Adapter == "orca-terminal" {
		transportOK := (next.Transport.Executable == "" || next.Transport.Executable == "orca" || next.Transport.Executable == "orca.exe") && len(next.Transport.Args) == 0
		executable := map[string]string{"codex": "codex", "pi": "pi.cmd", "dsh": "dsh.cmd"}[next.Agent]
		launch := next.AgentLaunch
		launch.RuntimeExecutable = ""
		launch.RuntimeSHA = ""
		launchOK := (launch.Executable == "" || launch.Executable == executable) && len(launch.Args) == 0
		for _, old := range current.Profiles {
			if old.Execution.Adapter != p.Execution.Adapter || old.Execution.Target.Kind != p.Execution.Target.Kind {
				continue
			}
			previous := programs(old)
			transportOK = transportOK || sameValue(previous.Transport, next.Transport)
			previousLaunch := previous.AgentLaunch
			previousLaunch.RuntimeExecutable = ""
			previousLaunch.RuntimeSHA = ""
			launchOK = launchOK || (previous.Agent == next.Agent && sameValue(previousLaunch, launch))
		}
		if !transportOK || !launchOK {
			return failure(400, "program_read_only", "native executable and arguments must come from a matching configuration")
		}
		if p.Execution.Target.Kind == "local" {
			inherited := false
			for _, old := range current.Profiles {
				a := programs(old).AgentLaunch
				if old.Execution.Adapter == "orca-terminal" && old.Execution.Target.Kind == "local" && a.RuntimeExecutable == next.AgentLaunch.RuntimeExecutable && a.RuntimeSHA == next.AgentLaunch.RuntimeSHA {
					inherited = true
				}
			}
			if !inherited || !validHelper(next.AgentLaunch) {
				return failure(400, "local_helper_unavailable", "本机 Orca 需要部署方提供的受信 helper 路径及 SHA256；页面不能创建或更换程序")
			}
		}
		return nil
	}
	return failure(400, "program_read_only", "native executable and arguments must come from an existing configuration")
}

type HelperStatus struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

func validHelper(c nativeCommand) bool {
	if !filepath.IsAbs(c.RuntimeExecutable) || len(c.RuntimeSHA) != 64 {
		return false
	}
	absolute, _ := filepath.Abs(c.RuntimeExecutable)
	resolved, err := pathguard.ResolveProspective(absolute)
	if err != nil {
		return false
	}
	same := filepath.Clean(absolute) == filepath.Clean(resolved)
	if runtime.GOOS == "windows" {
		same = strings.EqualFold(filepath.Clean(absolute), filepath.Clean(resolved))
	}
	if !same {
		return false
	}
	f, err := privatefs.OpenRegularSnapshot(c.RuntimeExecutable)
	if err != nil {
		return false
	}
	defer f.Close()
	hash, err := hashReader(f)
	return err == nil && hash == c.RuntimeSHA
}
func helperStatus(c Config) HelperStatus {
	for _, p := range c.Profiles {
		if p.Execution.Adapter == "orca-terminal" && p.Execution.Target.Kind == "local" && validHelper(programs(p).AgentLaunch) {
			return HelperStatus{Available: true}
		}
	}
	return HelperStatus{Reason: "本机 Orca helper 未部署或摘要不符；仍可查询目录。请由部署方提供路径及 SHA256 后生成新配置环境。"}
}
func Supported(p cfg.Profile) error {
	e := p.Execution
	if e.Adapter == "codex-app-server" && e.Target.Kind == "local" && e.Target.Environment == "" {
		return nil
	}
	if e.Adapter == "orca-terminal" && e.Target.Kind == "local" && e.Target.Environment == "" && runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		agent := programs(p).Agent
		if agent == "codex" || agent == "pi" || agent == "dsh" {
			return nil
		}
	}
	if e.Adapter == "orca-terminal" && e.Target.Kind == "orca_environment" && strings.TrimSpace(e.Target.Environment) != "" {
		var settings struct {
			Agent       string         `json:"agent"`
			AgentLaunch map[string]any `json:"agent_launch"`
		}
		if json.Unmarshal(e.Settings, &settings) == nil && (settings.Agent == "" || settings.Agent == "codex") {
			if _, ok := settings.AgentLaunch["runtime_executable"]; ok {
				return errors.New("remote helper runtime is unsupported")
			}
			if _, ok := settings.AgentLaunch["runtime_sha256"]; ok {
				return errors.New("remote helper runtime is unsupported")
			}
			return nil
		}
	}
	return errors.New("当前 Web 仅支持本机 Codex、本机 Orca / Codex、Pi、DSH 或远端 Orca / Codex")
}
func sameValue(a, b any) bool {
	ra, _ := json.Marshal(a)
	rb, _ := json.Marshal(b)
	ca, ea := runnercontract.CanonicalJSON(ra)
	cb, eb := runnercontract.CanonicalJSON(rb)
	return ea == nil && eb == nil && bytes.Equal(ca, cb)
}
func (s *Store) Save(ctx context.Context, expected string, raw []byte) (Snapshot, error) {
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
		_, latest, err := s.read()
		if err != nil || !bytes.Equal(latest, old) {
			return Snapshot{}, failure(412, "configuration_changed", "configuration changed before publication")
		}
		if err = ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		if err = privatefs.ReplacePrivateFile(f.Name(), target); err != nil {
			return Snapshot{}, failure(500, "save_unverified", "保存结果未核实，请重新读取配置")
		}
		_, published, err := s.read()
		if err != nil || !bytes.Equal(published, body) {
			return Snapshot{}, failure(500, "save_unverified", "保存结果未核实，请重新读取配置")
		}
		digest = cfg.Digest(published)
	}
	return Snapshot{Config: c, SavedDigest: digest, Writable: true, Workflows: s.catalog.Workflows, Efforts: s.catalog.Efforts, Combinations: s.catalog.Combinations, LocalHelper: helperStatus(c)}, nil
}
