package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type profileSnapshot struct {
	config RunnerExecutionConfig
	digest string
}

var errExecutionRestartRequired = errors.New("全局启动设置已变化，请重启 SteerLine")

// The application home publishes one immutable profile revision for all
// projects. The Runner build and the other startup settings stay pinned.
type executionProfiles struct {
	mu      sync.Mutex
	path    string
	startup []byte
	active  *profileSnapshot
	enabled bool
	frozen  bool
	native  *runnerclient.ConfigurationSnapshot
}

func newExecutionProfiles(home string, raw []byte) *executionProfiles {
	p := &executionProfiles{path: filepath.Join(home, "execution.json")}
	for _, path := range []string{filepath.Join(home, "manifest.json"), filepath.Join(filepath.Dir(home), "manifest.json")} {
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			p.frozen = true
			break
		}
	}
	if len(raw) != 0 {
		p.active = &profileSnapshot{digest: executionconfig.Digest(raw)}
		if decodeStrictJSON(raw, &p.active.config) == nil {
			p.startup = p.active.config.startupSettings()
		}
	}
	return p
}

func (p *executionProfiles) enable(native *runnerclient.ConfigurationSnapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.native, p.enabled = native, true
}

func (p *executionProfiles) current(ctx context.Context) (*profileSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.enabled || p.startup == nil {
		return p.active, nil
	}
	if err := ctx.Err(); err != nil {
		return p.active, err
	}
	raw, err := readConfiguration(p.path)
	if err != nil {
		return p.active, err
	}
	digest := executionconfig.Digest(raw)
	if digest == p.active.digest {
		return p.active, nil
	}
	if p.frozen {
		return p.active, errors.New("调试包执行方案已冻结，请生成新包以更换配置")
	}
	var next RunnerExecutionConfig
	if err := decodeStrictJSON(raw, &next); err != nil {
		return p.active, err
	}
	if !bytes.Equal(p.startup, next.startupSettings()) {
		return p.active, errExecutionRestartRequired
	}
	if err := executionconfig.Validate(next.Version, next.DefaultProfile, next.Profiles); err != nil {
		return p.active, err
	}
	if p.native != nil {
		for _, profile := range next.Profiles {
			if err := p.native.Catalog.ValidateConfiguration(profile.Execution, profile.Workflow); err != nil {
				return p.active, err
			}
		}
		prepareProfileMetadata(ctx, p.native, next.Profiles)
	}
	if err := ctx.Err(); err != nil {
		return p.active, err
	}
	latest, err := readConfiguration(p.path)
	if err != nil {
		return p.active, err
	}
	if !bytes.Equal(raw, latest) {
		return p.active, errors.New("执行方案在加载期间已变化，正在等待重新读取")
	}
	p.active = &profileSnapshot{config: next, digest: digest}
	return p.active, nil
}

type profileContextKey struct{}
type profileContextValue struct {
	source   *executionProfiles
	snapshot *profileSnapshot
	err      error
}

func (c Config) withProfiles(ctx context.Context) context.Context {
	if c.runtimeProfiles == nil {
		return ctx
	}
	if value, _ := ctx.Value(profileContextKey{}).(profileContextValue); value.source == c.runtimeProfiles {
		return ctx
	}
	snapshot, err := c.runtimeProfiles.current(ctx)
	return context.WithValue(ctx, profileContextKey{}, profileContextValue{c.runtimeProfiles, snapshot, err})
}

func (c Config) executionConfig(ctx context.Context) RunnerExecutionConfig {
	if c.runtimeProfiles != nil {
		value, _ := c.withProfiles(ctx).Value(profileContextKey{}).(profileContextValue)
		if value.snapshot != nil {
			return value.snapshot.config
		}
	}
	return c.RunnerExecution
}

func prepareProfileMetadata(ctx context.Context, native *runnerclient.ConfigurationSnapshot, profiles []RunnerProfile) {
	for _, profile := range profiles {
		if ctx.Err() != nil {
			return
		}
		_, _ = native.Resolve(ctx, runnercontract.ExecutionConfigurationRequest{Operation: "describe", Execution: &profile.Execution, Workflow: runnercontract.Workflow{ID: profile.Workflow.ID, Version: profile.Workflow.Version}})
		for _, target := range []struct{ workflow, mode string }{{"execute", "full_access"}, {"generate", "read_only"}} {
			_, _ = native.Resolve(ctx, runnercontract.ExecutionConfigurationRequest{Operation: "derive", Execution: &profile.Execution, Workflow: runnercontract.Workflow{ID: target.workflow, Version: 1}, AccessMode: target.mode})
		}
	}
}
