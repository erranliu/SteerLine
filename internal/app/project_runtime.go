package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	host "github.com/erranliu/SteerLine/internal/app/host"
	api "github.com/erranliu/SteerLine/internal/product/api"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	productclient "github.com/erranliu/SteerLine/internal/product/client"
	"github.com/erranliu/SteerLine/internal/product/httpapi"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/projectconnection"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type ProjectRuntime struct {
	config              Config
	hostConfig          host.Config
	configID            string
	repoLock, runLock   *host.RepoLock
	log                 io.WriteCloser
	store               *sqlite.SQLiteStore
	runner              projectRunner
	product             *productclient.Client
	port                *runtimeProjectPort
	closeOnce           sync.Once
	localErr, runnerErr error
}

func (r *ProjectRuntime) prepare(ctx context.Context, home string, e *catalogEntry, global []byte) (err error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	stage := func(name string) { fmt.Fprintf(os.Stderr, "project %s startup: %s\n", e.view.ConfigID, name) }
	stage("load configuration")
	c, err := loadConfigSnapshot(e.ref.ConfigPath, e.data, global, nil, true)
	if err != nil {
		return err
	}
	// Resolve again from the frozen bytes immediately before ownership is taken.
	ref := e.ref
	if err = projectconnection.ResolveSnapshot(ctx, &ref, e.data); err != nil {
		return err
	}
	if ref.Identity != e.ref.Identity || canonicalConfigPath(ref.RunDir) != canonicalConfigPath(e.ref.RunDir) {
		return errors.New("project identity changed after configuration census")
	}
	stage("validate state ownership")
	hc, err := hostRuntimeConfig(ctx, c)
	if err != nil {
		return err
	}
	admission := serviceAdmission{config: &c}
	if err = admission.ValidateStateLocation(hc); err != nil {
		return err
	}
	r.repoLock, err = host.LockRepository(ref.Identity)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(ref.RunDir, 0700); err != nil {
		return err
	}
	r.runLock, err = host.LockRepository(ref.RunDir)
	if err != nil {
		return err
	}
	lockedRef := e.ref
	if err = projectconnection.ResolveSnapshot(ctx, &lockedRef, e.data); err != nil {
		return err
	}
	if lockedRef.Identity != e.ref.Identity || canonicalConfigPath(lockedRef.RunDir) != canonicalConfigPath(e.ref.RunDir) {
		return errors.New("project identity changed while acquiring locks")
	}
	// These gates may inspect execution tools but cannot initialize a project
	// until both ownership locks have been obtained.
	stage("execution preflight")
	hc, err = admission.Preflight(ctx, hc, true)
	if err != nil {
		return err
	}
	stage("initialize native storage")
	if r.store, err = openOwnedNativeTaskCommandStore(ctx, &c); err != nil {
		return err
	}
	logDir := filepath.Join(home, "logs", "projects")
	if err = os.MkdirAll(logDir, 0700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(logDir, e.view.ProjectID+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	r.log = logFile
	r.config, r.hostConfig = c, hc
	return nil
}

func (r *ProjectRuntime) Run(parent context.Context, home string, e *catalogEntry, global []byte, publish func(*runtimeProjectPort)) (result contract.ProjectShutdown, retErr error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	startupTimer := time.AfterFunc(120*time.Second, cancel)
	defer startupTimer.Stop()
	defer func() {
		cancel()
		r.close()
		result = contract.ProjectShutdown{ConfigID: r.configID, ProjectID: e.view.ProjectID, LocalClosed: r.localErr == nil, LocksReleased: r.localErr == nil, ResourcesSettled: r.runner.ResourceSettlementError() == nil}
		if r.port != nil {
			result.InstanceID = r.port.instance
		}
		retErr = errors.Join(retErr, r.localErr, r.runnerErr, r.runner.ResourceSettlementError())
		if !result.LocalClosed || !result.ResourcesSettled {
			result.Diagnostic = retErr.Error()
		}
	}()
	if err := r.prepare(ctx, home, e, global); err != nil {
		return result, err
	}
	if retErr = bindRunnerBuild(ctx, r.config, r.store); retErr != nil {
		return
	}
	if retErr = r.runner.start(ctx, r.config.RunnerExecution, r.config.RunDir, positiveDuration(r.config.CommandTimeout, 30*time.Second), r.config.runnerPublication()); retErr != nil {
		return
	}
	r.config.runtimeRunner = &r.runner.client
	hostPort := host.NewServiceHost(r.hostConfig, "off", r.log, serviceActivityFormatter{})
	var service *taskrunner.Actor
	service, retErr = newRunnerService(r.config, hostPort, r.store, r.runner.client, r.runner.info)
	if retErr != nil {
		return
	}
	instance := service.InstanceID()
	fmt.Fprintf(r.log, "runtime instance=%s executable=%s home=%s config=%s source_commit=%s source_dirty=%s\n", instance, r.hostConfig.RuntimeExecutable, r.hostConfig.RuntimeHome, r.hostConfig.RuntimeConfig, r.hostConfig.SourceCommit, r.hostConfig.SourceDirty)
	r.config.runtimeHost = &r.hostConfig
	r.product, retErr = openServiceProjectClientWithCommands(ctx, r.config, service, func(context.Context, bool) (productclient.TaskCommands, error) {
		return &runtimeTaskCommands{clientTaskCommands: newClientTaskCommands(r.store, r.config), service: service}, nil
	}, r.store)
	if retErr != nil {
		return
	}
	var port *httpapi.Server
	port, retErr = httpapi.NewProject(httpapi.Ports{Query: r.product, Commands: r.product, Authority: r.product, NativeViewer: r.product}, httpapi.Options{RunDir: r.config.RunDir, ProjectIdentity: r.config.Identity(), InstanceID: instance, Process: localAPIProcessIdentity(service.State.Service.ProcessIdentity), Capabilities: publicProjectCapabilities(service.State.Service.Capabilities)})
	if retErr != nil {
		return
	}
	r.port = &runtimeProjectPort{Server: port, product: r.product, config: r.config, store: r.store, instance: instance}
	actor := serviceReadyHook{Actor: service, ready: func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := port.SetReady(instance, service.State.Service.Ready, serviceReadyDiagnostic(service)); err != nil {
			return err
		}
		if service.State.Service.Ready {
			startupTimer.Stop()
		}
		publish(r.port)
		return nil
	}}
	retErr = host.RunService(ctx, actor, r.hostConfig, "manual")
	return
}

// close is the sole final resource closer, including partial startup failures.
// Remote Runner uncertainty never prevents local cleanup or changes its result.
func (r *ProjectRuntime) close() {
	r.closeOnce.Do(func() {
		if r.port != nil {
			r.port.StopAccepting()
			r.port.drain()
			r.localErr = errors.Join(r.localErr, r.port.Server.Close(context.Background()))
		}
		if r.product != nil {
			r.localErr = errors.Join(r.localErr, r.product.Close())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		r.runnerErr = r.runner.shutdown(ctx)
		cancel()
		// Preserve the former EndActor/Close pair's bounded follow-up. A confirmed
		// stop only re-reads failed evidence; an unknown stop retains its request ID.
		if r.runner.ResourceSettlementError() != nil && r.runner.stop != nil {
			ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
			r.runnerErr = errors.Join(r.runnerErr, r.runner.shutdown(ctx))
			cancel()
		}
		if r.store != nil {
			r.localErr = errors.Join(r.localErr, r.store.Close())
		}
		if r.log != nil {
			r.localErr = errors.Join(r.localErr, r.log.Close())
		}
		if r.localErr == nil {
			if r.runLock != nil {
				r.runLock.Close()
			}
			if r.repoLock != nil {
				r.repoLock.Close()
			}
		}
	})
}

type runtimeProjectPort struct {
	instance string
	*httpapi.Server
	product  *productclient.Client
	config   Config
	store    taskStorage
	stopping atomic.Bool
	users    sync.RWMutex
}

func (p *runtimeProjectPort) StopAccepting() { p.stopping.Store(true) }
func (p *runtimeProjectPort) Command(ctx context.Context, in httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	if err := p.enter(); err != nil {
		return httpcontract.CommandResponse{}, err
	}
	defer p.users.RUnlock()
	return p.Server.Command(ctx, in)
}
func (p *runtimeProjectPort) Task(ctx context.Context, method, route string, body json.RawMessage) (any, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer p.users.RUnlock()
	if method == "GET" {
		kind, target, id := "create_task_batch", p.config.Identity(), strings.TrimPrefix(route, "/task-batches/")
		if strings.HasPrefix(route, "/tasks/") {
			parts := strings.Split(strings.Trim(route, "/"), "/")
			target = parts[1]
			id = parts[4]
			kind = "add_task_predecessors"
		}
		commands := NewTaskCommands(p.store, p.config)
		value, err := commands.ReadReceipt(ctx, id, kind, target)
		if err != nil {
			var domain *taskmodel.Error
			if errors.As(err, &domain) {
				status := 409
				if domain.Code == "command_not_found" {
					status = 404
				} else if domain.Code == "outcome_unknown" {
					status = 503
				}
				return nil, &httpclient.ClientError{Status: status, Code: domain.Code, Reason: domain.Message}
			}
		}
		return value, err
	}
	var in struct {
		ProjectID  string          `json:"project_id"`
		InstanceID string          `json:"instance_id"`
		Payload    json.RawMessage `json:"payload"`
	}
	if err := decodeStrictJSON(body, &in); err != nil {
		return nil, err
	}
	if in.ProjectID != httpcontract.ProjectID(p.config.Identity()) || in.InstanceID != p.Server.Identity(ctx).InstanceID {
		return nil, &httpclient.ClientError{Status: 409, Code: "stale_target", Reason: "task request target differs"}
	}
	if p.stopping.Load() || !p.Server.Identity(ctx).Ready {
		return nil, &httpclient.ClientError{Status: 503, Code: "service_not_ready", Reason: "project is not accepting task commands"}
	}
	switch route {
	case "/task-batch-validation":
		return p.product.ValidateTaskBatch(ctx, in.Payload)
	case "/task-batches":
		var wire struct {
			Payload                  json.RawMessage `json:"payload"`
			RequestID                string          `json:"request_id"`
			RequestedBy              string          `json:"requested_by"`
			ExpectedAuthorityVersion uint64          `json:"expected_authority_version"`
		}
		if err := decodeStrictJSON(in.Payload, &wire); err != nil {
			return nil, err
		}
		return p.product.CreateTaskBatch(ctx, api.CreateInput{Payload: wire.Payload, RequestID: wire.RequestID, RequestedBy: wire.RequestedBy, ExpectedAuthorityVersion: wire.ExpectedAuthorityVersion})
	default:
		var input api.TaskPredecessorAddInput
		if err := decodeStrictJSON(in.Payload, &input); err != nil {
			return nil, err
		}
		if route != "/tasks/"+input.TaskID+"/predecessors" {
			return nil, fmt.Errorf("predecessor route target differs")
		}
		return p.product.AddPredecessors(ctx, input)
	}
}

// Admission covers reads and direct task receipts as well as command writes.
func (p *runtimeProjectPort) enter() error {
	// Reject late callers before a waiting drain writer can block their RLock.
	if p.stopping.Load() {
		return projectStoppingError()
	}
	p.users.RLock()
	if p.stopping.Load() {
		p.users.RUnlock()
		return projectStoppingError()
	}
	return nil
}
func (p *runtimeProjectPort) drain() { p.users.Lock(); p.users.Unlock() }
func (p *runtimeProjectPort) Read(ctx context.Context, route string, values url.Values) (any, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer p.users.RUnlock()
	return p.Server.Read(ctx, route, values)
}

func projectStoppingError() error {
	return &httpclient.ClientError{Status: 503, Code: "service_stopping", Reason: "application host is stopping"}
}
