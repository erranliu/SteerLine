package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
)

type ProjectRuntime struct {
	config            Config
	hostConfig        host.Config
	configID          string
	repoLock, runLock *host.RepoLock
	log               *os.File
}

func (d *ApplicationDirectory) prepareProject(e *catalogEntry) (r *ProjectRuntime, err error) {
	ctx, cancel := context.WithTimeout(d.ctx, 60*time.Second)
	defer cancel()
	stage := func(name string) { fmt.Fprintf(os.Stderr, "project %s startup: %s\n", e.view.ConfigID, name) }
	stage("load configuration")
	c, err := loadConfigSnapshot(e.ref.ConfigPath, e.data, d.global, nil, nil, true)
	if err != nil {
		return nil, err
	}
	// Resolve again from the frozen bytes immediately before ownership is taken.
	ref := e.ref
	if err = projectconnection.ResolveSnapshot(ctx, &ref, e.data); err != nil {
		return nil, err
	}
	if ref.Identity != e.ref.Identity || canonicalConfigPath(ref.RunDir) != canonicalConfigPath(e.ref.RunDir) {
		return nil, errors.New("project identity changed after configuration census")
	}
	stage("validate state ownership")
	hc, err := hostRuntimeConfig(ctx, c)
	if err != nil {
		return nil, err
	}
	admission := serviceAdmission{config: &c}
	if err = admission.ValidateStateLocation(hc); err != nil {
		return nil, err
	}
	repoLock, err := host.LockRepository(ref.Identity)
	if err != nil {
		return nil, err
	}
	var runLock *host.RepoLock
	defer func() {
		if err != nil {
			if runLock != nil {
				runLock.Close()
			}
			repoLock.Close()
		}
	}()
	if err = os.MkdirAll(ref.RunDir, 0700); err != nil {
		return nil, err
	}
	runLock, err = host.LockRepository(ref.RunDir)
	if err != nil {
		return nil, err
	}
	lockedRef := e.ref
	if err = projectconnection.ResolveSnapshot(ctx, &lockedRef, e.data); err != nil {
		return nil, err
	}
	if lockedRef.Identity != e.ref.Identity || canonicalConfigPath(lockedRef.RunDir) != canonicalConfigPath(e.ref.RunDir) {
		return nil, errors.New("project identity changed while acquiring locks")
	}
	// These gates may inspect execution tools but cannot initialize a project
	// until both ownership locks have been obtained.
	stage("execution preflight")
	hc, err = admission.Preflight(ctx, hc, true)
	if err != nil {
		return nil, err
	}
	stage("initialize native storage")
	if err = admission.InitializeNativeProject(ctx, hc); err != nil {
		return nil, err
	}
	logDir := filepath.Join(d.home, "logs", "projects")
	if err = os.MkdirAll(logDir, 0700); err != nil {
		return nil, err
	}
	log, err := os.OpenFile(filepath.Join(logDir, e.view.ProjectID+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	return &ProjectRuntime{config: c, hostConfig: hc, configID: e.view.ConfigID, repoLock: repoLock, runLock: runLock, log: log}, nil
}

func (r *ProjectRuntime) Run(parent context.Context, publish func(*runtimeProjectPort)) (result contract.ProjectShutdown, retErr error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	startupTimer := time.AfterFunc(120*time.Second, cancel)
	defer startupTimer.Stop()
	result = contract.ProjectShutdown{ConfigID: r.configID, ProjectID: httpcontract.ProjectID(r.config.Identity())}
	defer func() {
		r.log.Close()
		if result.LocalClosed {
			r.runLock.Close()
			r.repoLock.Close()
			result.LocksReleased = true
		}
	}()
	admission := serviceAdmission{config: &r.config}
	hostPort := host.NewServiceHost(r.hostConfig, "", "off", r.log, admission, serviceActivityFormatter{})
	var store *sqlite.SQLiteStore
	service, err := newServiceWithTaskStore(ctx, r.config, r.config.ExecutionDirectory, newConversationSources(), hostPort, func(value *sqlite.SQLiteStore) { store = value })
	if err != nil {
		result.LocalClosed = true
		result.ResourcesSettled = true
		return result, err
	}
	result.InstanceID = service.InstanceID()
	fmt.Fprintf(r.log, "runtime instance=%s executable=%s home=%s config=%s source_commit=%s source_dirty=%s\n", result.InstanceID, r.hostConfig.RuntimeExecutable, r.hostConfig.RuntimeHome, r.hostConfig.RuntimeConfig, r.hostConfig.SourceCommit, r.hostConfig.SourceDirty)
	r.config.runtimeHost = &r.hostConfig
	product, err := openServiceProjectClientWithCommands(ctx, r.config, service, func(context.Context, bool) (productclient.TaskCommands, error) {
		return &runtimeTaskCommands{clientTaskCommands: newClientTaskCommands(store, r.config), service: service}, nil
	}, store)
	if err != nil {
		closeErr := service.Close()
		result.LocalClosed = closeErr == nil
		result.ResourcesSettled = true
		return result, errors.Join(err, closeErr)
	}
	port, err := httpapi.NewProject(httpapi.Ports{Query: product, Commands: product, Authority: product, NativeViewer: product}, httpapi.Options{RunDir: r.config.RunDir, ProjectIdentity: r.config.Identity(), InstanceID: result.InstanceID, Process: localAPIProcessIdentity(service.State.Service.ProcessIdentity), Capabilities: publicProjectCapabilities(service.State.Service.Capabilities)})
	if err != nil {
		closeErr := errors.Join(product.Close(), service.Close())
		result.LocalClosed = closeErr == nil
		result.ResourcesSettled = true
		return result, errors.Join(err, closeErr)
	}
	project := &runtimeProjectPort{Server: port, product: product, config: r.config, store: store, instance: result.InstanceID}
	actor := serviceReadyHook{Service: service, ready: func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := port.SetReady(result.InstanceID, service.State.Service.Ready, serviceReadyDiagnostic(service)); err != nil {
			return err
		}
		if service.State.Service.Ready {
			startupTimer.Stop()
		}
		publish(project)
		return nil
	}}
	runErr := host.RunService(ctx, actor, r.hostConfig, "manual")
	project.StopAccepting()
	closeErr := errors.Join(port.Close(context.Background()), product.Close(), service.Close())
	result.LocalClosed = closeErr == nil
	result.ResourcesSettled = service.ResourceSettlementError() == nil
	if !result.ResourcesSettled {
		result.Diagnostic = service.ResourceSettlementError().Error()
	}
	retErr = errors.Join(runErr, closeErr)
	return
}

type runtimeProjectPort struct {
	instance string
	*httpapi.Server
	product  *productclient.Client
	config   Config
	store    taskStorage
	stopping atomic.Bool
}

func (p *runtimeProjectPort) StopAccepting() { p.stopping.Store(true) }
func (p *runtimeProjectPort) Command(ctx context.Context, in httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	if p.stopping.Load() {
		return httpcontract.CommandResponse{}, &httpclient.ClientError{Status: 503, Code: "service_stopping", Reason: "application host is stopping"}
	}
	return p.Server.Command(ctx, in)
}
func (p *runtimeProjectPort) Task(ctx context.Context, method, route string, body json.RawMessage) (any, error) {
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
