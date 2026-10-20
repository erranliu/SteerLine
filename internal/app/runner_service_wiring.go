package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/hostport"
	"github.com/erranliu/SteerLine/internal/hostcontract"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

func (c Config) runnerHome() string {
	if c.executionPath != "" {
		return filepath.Dir(c.executionPath)
	}
	return HomeDir()
}

func (c Config) runnerPublication() runnerclient.Publication {
	settings := map[string]any{"base_url": c.PR.BaseURL}
	if c.PR.Kind == "command" {
		settings["query"] = c.PR.Query
	}
	if c.PR.TokenEnv != "" {
		settings["authentication_refs"] = []string{c.PR.TokenEnv}
	}
	body, _ := json.Marshal(settings)
	return runnerclient.Publication{Provider: c.PR.Kind, Repository: c.PR.Repository, BaseBranch: c.Branch, Settings: body}
}

func runnerTaskService(config Config, connection taskrunner.Store, cli runnerclient.Client) *taskrunner.Service {
	service := &taskrunner.Service{Project: config.Identity(), Claimant: config.Claimant, StateDir: cli.StateDir, DefaultProfile: config.RunnerExecution.DefaultProfile, Profiles: config.RunnerExecution.taskProfiles(config.Repository, config.runnerPublication()), Tasks: NewTaskService(connection, config), Store: connection}
	service.RunnerBuild = cli.BuildPin()
	service.PrepareTestPlan = config.prepareTestPlan
	service.Bridge = &runnerbridge.Bridge{Store: connection, Client: func(string) runnerbridge.CLI { return cli }}
	git := commandRunner{Dir: config.Repository, Timeout: positiveDuration(config.CommandTimeout, 30*time.Second), ProcessHost: config.ProcessHost}
	base := taskrunner.BaseResolver{Project: config.Identity(), Store: connection, Git: func(ctx context.Context, args ...string) ([]byte, error) { return git.Output(ctx, config.Git, args...) }}
	service.ResolveBase = base.Resolve
	return service
}

func (c Config) runnerClient(ctx context.Context) (runnerclient.Client, error) {
	if err := runnerstatepath.Validate(c.runnerHome()); err != nil {
		return runnerclient.Client{}, err
	}
	if c.runtimeRunner != nil {
		return *c.runtimeRunner, nil
	}
	cli, _, err := c.RunnerExecution.Runner.Open(ctx, runnerstatepath.Directory(c.runnerHome()), positiveDuration(c.CommandTimeout, 30*time.Second))
	return cli, err
}

// newRunnerService borrows project resources; it never opens or closes them.
func newRunnerService(config Config, host hostport.HostPort, connection *sqlite.SQLiteStore, cli runnerclient.Client, info runnerclient.ServiceInfo) (*taskrunner.Actor, error) {
	service := runnerTaskService(config, connection, cli)
	service.Admit = func(ctx context.Context) error {
		if err := (appConfigVerifier{project: config.projectPath, global: config.executionPath, digest: config.configDigest}).VerifyCurrentConfig(); err != nil {
			return err
		}
		status, err := cli.ServiceStatus(ctx)
		if err != nil {
			return err
		}
		if status.Status != "ready" || status.InstanceID != info.InstanceID || status.ExecutableSHA256 != info.ExecutableSHA256 || status.ResourceDigest != info.ResourceDigest || status.Version != info.Version {
			return errors.New("original selected Runner service is unavailable; restart SteerLine explicitly")
		}
		return nil
	}
	controls := taskrunner.Controls{Tasks: service, Store: connection, ResolveProfile: func(id, digest string) (string, error) {
		p, err := config.RunnerExecution.Profile(id, digest)
		return p.ID, err
	}}
	var actor *taskrunner.Actor
	var err error
	pin := cli.BuildPin()
	actor, err = taskrunner.NewActor(taskrunner.ActorOptions{RunnerBuild: &pin, NativeSessionAdapters: cli.NativeSessionAdapters, Tasks: service, Store: connection, Host: host, Repository: hostcontract.Repository{Path: config.Repository, Remote: config.Remote, Branch: config.Branch, Identity: config.Identity()}, Mode: "manual", ConfigDigest: config.configDigest, Heartbeat: positiveDuration(config.Heartbeat, 5*time.Second), Control: func(ctx context.Context, req api.ControlRequest) (api.ControlReceipt, error) {
		return controls.Handle(ctx, req, actor.InstanceID(), actor.State.Control.Revision)
	}})
	return actor, err
}
