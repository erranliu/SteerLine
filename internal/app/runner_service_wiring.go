package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	steerline "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution/hostport"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	ports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

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
	service.Bridge = &runnerbridge.Bridge{Store: connection, Client: func(string) runnerbridge.CLI { return cli }}
	git := commandRunner{Dir: config.Repository, Timeout: positiveDuration(config.CommandTimeout, 30*time.Second), ProcessHost: config.ProcessHost}
	base := taskrunner.BaseResolver{Project: config.Identity(), Store: connection, Git: func(ctx context.Context, args ...string) ([]byte, error) { return git.Output(ctx, config.Git, args...) }}
	service.ResolveBase = base.Resolve
	return service
}

func newRunnerServiceWithHost(config Config, host hostport.HostPort, mode string) (*taskrunner.Actor, error) {
	ctx := context.Background()
	if config.ProcessHost == nil {
		return nil, errors.New("service process host is required")
	}
	if err := config.ValidateTaskCommandConfiguration(); err != nil {
		return nil, err
	}
	if err := config.RequireNativeTaskAuthority(); err != nil {
		return nil, err
	}
	if err := config.ResolveIdentity(ctx); err != nil {
		return nil, err
	}
	if err := PreflightNativeStatePath(config); err != nil {
		return nil, err
	}
	if host == nil {
		hostConfig, err := hostRuntimeConfig(ctx, config)
		if err != nil {
			return nil, err
		}
		host = steerline.NewServiceHost(hostConfig, "", "", nil, serviceAdmission{config: &config}, consoleActivityFormatter{})
	}
	connection, err := OpenNativeTaskCommandStore(ctx, &config)
	if err != nil {
		return nil, err
	}
	cli, info, err := config.RunnerExecution.startRunner(ctx, config.RunDir, positiveDuration(config.CommandTimeout, 30*time.Second), config.runnerPublication())
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	shutdownID := "sl-service-stop-" + info.InstanceID
	shutdown := func(ctx context.Context) error { return cli.StopService(ctx, info.InstanceID, shutdownID) }
	service := runnerTaskService(config, connection, cli)
	service.Admit = func(ctx context.Context) error {
		if err := (appConfigVerifier{project: config.projectPath, global: config.executionPath, digest: config.configDigest}).VerifyCurrentConfig(); err != nil {
			return err
		}
		status, err := cli.ServiceStatus(ctx)
		if err != nil {
			return err
		}
		if status.Status != "ready" || status.InstanceID != info.InstanceID || status.ExecutableSHA256 != info.ExecutableSHA256 {
			return errors.New("original selected Runner service is unavailable; restart SteerLine explicitly")
		}
		return nil
	}
	controls := taskrunner.Controls{Tasks: service, Store: connection, ResolveProfile: func(id, digest string) (string, error) {
		p, err := config.RunnerExecution.Profile(id, digest)
		return p.ID, err
	}}
	var actor *taskrunner.Actor
	actor, err = taskrunner.NewActor(taskrunner.ActorOptions{Tasks: service, Store: connection, Host: host, Repository: ports.CheckpointRepository{Path: config.Repository, Remote: config.Remote, Branch: config.Branch, Identity: config.Identity()}, Mode: mode, ConfigDigest: config.configDigest, Heartbeat: positiveDuration(config.Heartbeat, 5*time.Second), ShutdownRunner: shutdown, Control: func(ctx context.Context, req api.ControlRequest) (api.ControlReceipt, error) {
		return controls.Handle(ctx, req, actor.InstanceID(), actor.State.Control.Revision)
	}})
	if err != nil {
		closeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return nil, errors.Join(err, shutdown(closeCtx), connection.Close())
	}
	return actor, nil
}

func (c Config) runnerClient(ctx context.Context) (runnerclient.Client, error) {
	cli, _, err := c.RunnerExecution.Runner.Open(ctx, filepath.Join(c.RunDir, "runner"), positiveDuration(c.CommandTimeout, 30*time.Second))
	return cli, err
}
