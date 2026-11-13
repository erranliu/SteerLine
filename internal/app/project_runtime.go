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
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/projectconnection"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type ProjectRuntime struct {
	runner            *applicationRunner // borrowed; only ApplicationDirectory stops it
	config            Config
	hostConfig        host.Config
	configID          string
	repoLock, runLock *host.RepoLock
	log               io.WriteCloser
	store             *sqlite.SQLiteStore
	tests             *testManagementPort
	environment       *environmentPort
	product           *productclient.Client
	port              *runtimeProjectPort
	closeOnce         sync.Once
	localErr          error
}

func (r *ProjectRuntime) prepare(ctx context.Context, home string, e *catalogEntry, global []byte) (err error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	started, previous := time.Now(), time.Now()
	stage := func(name string) {
		fmt.Fprintf(os.Stderr, "project %s startup: %s elapsed=%s previous=%s\n", e.view.ConfigID, name, time.Since(started), time.Since(previous))
		previous = time.Now()
	}
	stage("load configuration")
	var c Config
	defer func() { c.finishStartupQueries() }()
	if e.startupConfig != nil {
		c = *e.startupConfig
		c.startup = newStartupQueries(ctx)
		if err = c.ValidateProfiles(); err != nil {
			return err
		}
	} else {
		c, err = loadConfigSnapshotWithQueries(e.ref.ConfigPath, e.data, global, nil, true, newStartupQueries(ctx))
		if err != nil {
			return err
		}
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
	// A new ownership boundary requires fresh Git evidence. Do not let pre-lock
	// memoized queries authorize opening the active project writer.
	c.resetStartupQueries(ctx)
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
	c.finishStartupQueries()
	stage("native storage ready")
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

func (r *ProjectRuntime) Run(parent context.Context, home string, e *catalogEntry, global []byte, runner *applicationRunner, publish func(*runtimeProjectPort)) (result contract.ProjectShutdown, retErr error) {
	r.runner = runner
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var maintenanceDone chan struct{}
	var preparationDone chan struct{}
	var environmentDone chan struct{}
	var advisoryDone chan struct{}
	var planningDone chan struct{}
	var refactoringDone chan struct{}
	startupTimer := time.AfterFunc(120*time.Second, cancel)
	defer startupTimer.Stop()
	defer func() {
		cancel()
		if refactoringDone != nil {
			<-refactoringDone
		}
		if r.config.runtimeMessages != nil {
			r.config.runtimeMessages.release(r.config.Identity())
		}
		if advisoryDone != nil {
			<-advisoryDone
		}
		if planningDone != nil {
			<-planningDone
		}
		if environmentDone != nil {
			<-environmentDone
		}
		if maintenanceDone != nil {
			<-maintenanceDone
		}
		if preparationDone != nil {
			<-preparationDone
		}
		r.close()
		result = contract.ProjectShutdown{ConfigID: r.configID, ProjectID: e.view.ProjectID, LocalClosed: r.localErr == nil, LocksReleased: r.localErr == nil}
		if r.port != nil {
			result.InstanceID = r.port.instance
		}
		retErr = errors.Join(retErr, r.localErr)
		if !result.LocalClosed {
			result.Diagnostic = retErr.Error()
		}
	}()
	if err := r.prepare(ctx, home, e, global); err != nil {
		return result, err
	}
	r.config.runtimeRunner = &runner.client
	r.config.runtimeMessages = runner.messages
	if runner.messages != nil {
		runner.messages.register(ctx, r.config.Identity(), r.store)
	}
	r.config.runtimeGenerations = runner.generations
	r.config.runtimeCatalog = runner.catalog
	r.config.runtimeNative = runner.native
	r.config.runtimeEnvironmentAdmission = runner.environments
	if runner.environments != nil {
		runner.environments.register(r.config)
	}
	maintenance := newProjectMaintenance(r.config, r.store)
	r.environment = newEnvironmentPort(r.config, r.store, runner.client, nil, maintenance)
	r.config.runtimeEnvironment = r.environment
	if retErr = bindRunnerBuild(ctx, r.config, r.store); retErr != nil {
		return
	}
	if retErr = runner.validateProject(r.config); retErr != nil {
		return
	}
	r.config.runtimeMaintenance = maintenance
	maintenance.environmentObservation = r.environment.observeBasis
	maintenanceDone = make(chan struct{})
	go func() { defer close(maintenanceDone); maintenance.observe(ctx) }()
	hostPort := host.NewServiceHost(r.hostConfig, "off", r.log, serviceActivityFormatter{})
	var service *taskrunner.Actor
	service, retErr = newRunnerService(r.config, hostPort, r.store, runner.client, runner.info, maintenance)
	if retErr != nil {
		return
	}
	instance := service.InstanceID()
	r.tests = newTestManagementPort(r.config, r.store, runner.client, service.VerifyRunnerAdmission, maintenance, maintenance, maintenance)
	r.environment.service.Admit = service.VerifyRunnerAdmission
	r.config.runtimeTests = r.tests
	taskAdmission := newRoadmapTaskCommands(r.store, r.config, service.ExecuteTaskMutation)
	r.config.runtimeRoadmaps = newRoadmapPort(r.config, r.store, runner.client, service.VerifyRunnerAdmission, maintenance, taskAdmission, maintenance, maintenance.refreshBaselineForPush)
	r.config.runtimeContinuationSuggestion = newContinuationSuggestionPort(r.config, r.store, runner.client, service.VerifyRunnerAdmission)
	r.config.runtimeRefactoring = newRefactoringPort(r.config, r.store, runner.client, service.VerifyRunnerAdmission, maintenance, maintenance)
	if retErr = r.config.runtimeRefactoring.service.Recover(ctx); retErr != nil {
		return
	}
	service.ObserveAuxiliary = auxiliaryObserver(r.tests.progress.ObserveNext, r.config.runtimeRoadmaps.service.ObserveNext)
	fmt.Fprintf(r.log, "runtime instance=%s executable=%s home=%s config=%s source_commit=%s source_dirty=%s\n", instance, r.hostConfig.RuntimeExecutable, r.hostConfig.RuntimeHome, r.hostConfig.RuntimeConfig, r.hostConfig.SourceCommit, r.hostConfig.SourceDirty)
	r.hostConfig.ControlPublished = func(action string) {
		if action == "stop_candidate" {
			service.NotifyControl()
		}
	}
	r.config.runtimeHost = &r.hostConfig
	r.product, retErr = openServiceProjectClientWithCommands(ctx, r.config, service, func(context.Context) (productclient.TaskCommands, error) {
		return &runtimeTaskCommands{clientTaskCommands: newClientTaskCommands(r.store, r.config), service: service}, nil
	}, r.store, maintenance, maintenance)
	if retErr != nil {
		return
	}
	var port *httpapi.Server
	port, retErr = httpapi.NewProject(httpapi.Ports{Query: r.product, Commands: r.product, Authority: r.product, NativeViewer: r.product}, httpapi.Options{RunDir: r.config.RunDir, ProjectIdentity: r.config.Identity(), InstanceID: instance, Process: localAPIProcessIdentity(service.State.Service.ProcessIdentity), Capabilities: publicProjectCapabilities(service.State.Service.Capabilities)})
	if retErr != nil {
		return
	}
	r.port = &runtimeProjectPort{actor: service, Server: port, product: r.product, config: r.config, store: r.store, instance: instance, readingBodies: runner.readingBodies, readiness: maintenance, conversationReaders: newActivityConversationReaders(r.store)}
	actor := serviceReadyHook{Actor: service, ready: func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := port.SetReady(instance, service.State.Service.Ready, serviceReadyDiagnostic(service)); err != nil {
			return err
		}
		if service.State.Service.Ready {
			startupTimer.Stop()
			if planningDone == nil {
				planningDone = make(chan struct{})
				go func() { defer close(planningDone); r.config.runtimeRoadmaps.planning.run(ctx) }()
			}
			if refactoringDone == nil {
				refactoringDone = make(chan struct{})
				go func() { defer close(refactoringDone); r.config.runtimeRefactoring.run(ctx) }()
			}
			if advisoryDone == nil {
				advisoryDone = make(chan struct{})
				go func() { defer close(advisoryDone); r.config.runtimeContinuationSuggestion.run(ctx) }()
			}
			if preparationDone == nil {
				preparationDone = make(chan struct{})
				go func() { defer close(preparationDone); maintenance.runPreparation(ctx) }()
			}
			if environmentDone == nil {
				environmentDone = make(chan struct{})
				go func() {
					defer close(environmentDone)
					r.environment.observe(ctx)
				}()
			}
		}
		publish(r.port)
		return nil
	}}
	retErr = host.RunService(ctx, actor, r.hostConfig, "manual")
	return
}

// The project actor calls the observer serially with its shared observation budget.
func auxiliaryObserver(stages ...func(context.Context) error) func(context.Context) error {
	next := 0
	return func(ctx context.Context) error {
		var failures []error
		for range stages {
			if ctx.Err() != nil {
				break
			}
			stage := stages[next]
			// Advance before the call so a timeout cannot starve the next stage.
			next = (next + 1) % len(stages)
			failures = append(failures, stage(ctx))
		}
		return errors.Join(append(failures, ctx.Err())...)
	}
}

// close is the sole final resource closer, including partial startup failures.
// Remote Runner uncertainty never prevents local cleanup or changes its result.
func (r *ProjectRuntime) close() {
	r.closeOnce.Do(func() {
		if r.port != nil {
			r.port.StopAccepting()
			r.port.drain()
		}
		if r.config.runtimeContinuationSuggestion != nil {
			stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := r.config.runtimeContinuationSuggestion.service.Close(stopCtx); err != nil && r.log != nil {
				fmt.Fprintf(r.log, "advisory stop unverified: %v\n", err)
			}
			cancel()
		}
		if r.config.runtimeRefactoring != nil {
			stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := r.config.runtimeRefactoring.service.Close(stopCtx); err != nil && r.log != nil {
				fmt.Fprintf(r.log, "refactoring stop unverified: %v\n", err)
			}
			cancel()
		}
		if r.config.runtimeRoadmaps != nil {
			stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := r.config.runtimeRoadmaps.planning.close(stopCtx); err != nil && r.log != nil {
				fmt.Fprintf(r.log, "roadmap planning stop remains unverified: %v\n", err)
			}
			cancel()
			stopCtx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
			if err := r.config.runtimeRoadmaps.service.Close(stopCtx); err != nil && r.log != nil {
				fmt.Fprintf(r.log, "roadmap generation stop remains unverified: %v\n", err)
			}
			cancel()
		}
		if r.environment != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := r.environment.service.Close(ctx); err != nil && r.log != nil {
				fmt.Fprintf(r.log, "environment stop remains unverified: %v\n", err)
			}
			cancel()
		}
		if r.port != nil {
			r.localErr = errors.Join(r.localErr, r.port.Server.Close(context.Background()))
		}
		if r.product != nil {
			r.localErr = errors.Join(r.localErr, r.product.Close())
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
	actor    *taskrunner.Actor
	instance string
	*httpapi.Server
	product             *productclient.Client
	config              Config
	store               taskStorage
	stopping            atomic.Bool
	users               sync.RWMutex
	readingOnce         sync.Once
	readingBodies       *query.ReadingBodyCache
	readiness           projectReadinessReader
	conversationReaders map[string]activityConversationReader
}

func (p *runtimeProjectPort) StopAccepting() { p.stopping.Store(true) }
func (p *runtimeProjectPort) Command(ctx context.Context, in httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	if err := p.enter(); err != nil {
		return httpcontract.CommandResponse{}, err
	}
	defer p.users.RUnlock()
	if strings.HasPrefix(in.Action, "refactoring_") {
		return p.refactoringCommand(ctx, in)
	}
	if strings.HasPrefix(in.Action, "main_agent_") {
		return p.Server.CommandContinuationSuggestion(ctx, in, p)
	}
	if strings.HasPrefix(in.Action, "roadmap_") {
		return p.Server.CommandRoadmap(ctx, in, p)
	}
	if strings.HasPrefix(in.Action, "maintenance_") {
		return p.maintenanceCommand(ctx, in)
	}
	if strings.HasPrefix(in.Action, "environment_") {
		return p.environmentCommand(ctx, in)
	}
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
	if route == "/dispatch" {
		if len(values) != 0 {
			return nil, &httpclient.ClientError{Status: 400, Code: "invalid_query", Reason: "dispatch observation does not accept parameters"}
		}
		return p.readDispatch(ctx)
	}
	if route == "/activities" {
		return p.readAsyncActivities(ctx, values)
	}
	if route == "/refactoring" || strings.HasPrefix(route, "/refactoring/") {
		if p.config.runtimeRefactoring == nil {
			return nil, errors.New("refactoring unavailable")
		}
		return p.config.runtimeRefactoring.read(ctx, route, values)
	}
	if strings.HasPrefix(route, "/activities/") {
		return p.readActivity(ctx, route, values)
	}
	if route == "/main-agent" || strings.HasPrefix(route, "/main-agent/") {
		if p.config.runtimeContinuationSuggestion == nil {
			return nil, errors.New("main Agent unavailable")
		}
		return p.config.runtimeContinuationSuggestion.read(ctx, route, values)
	}
	if strings.HasPrefix(route, "/roadmaps") {
		if p.config.runtimeRoadmaps == nil {
			return nil, errors.New("roadmap unavailable")
		}
		return p.config.runtimeRoadmaps.read(ctx, route, values)
	}
	if route == planningRoute || strings.HasPrefix(route, planningRoute+"/") {
		if p.config.runtimeRoadmaps == nil || p.config.runtimeRoadmaps.planning == nil {
			return nil, errors.New("roadmap planning unavailable")
		}
		return p.config.runtimeRoadmaps.planning.read(ctx, route, values)
	}
	if route == "/environment" || strings.HasPrefix(route, "/environment/") {
		return p.environmentRead(ctx, route, values)
	}
	if strings.HasPrefix(route, "/runs/") && strings.HasSuffix(route, "/documents") && strings.Count(route, "/") == 3 {
		if len(values) != 0 {
			return nil, errors.New("document query does not accept parameters")
		}
		return p.config.runtimeMaintenance.candidateDocuments(ctx, strings.TrimSuffix(strings.TrimPrefix(route, "/runs/"), "/documents"))
	}
	if strings.HasPrefix(route, "/maintenance") {
		if strings.HasPrefix(route, "/maintenance/tasks/") && len(values) > 0 {
			if len(values) != 1 || len(values["view"]) != 1 || values.Get("view") != "status" {
				return nil, &httpclient.ClientError{Status: 400, Code: "invalid_query", Reason: "unsupported or repeated maintenance query"}
			}
			if p.config.runtimeMaintenance == nil {
				return nil, errors.New("maintenance unavailable")
			}
			return p.config.runtimeMaintenance.taskReadiness(ctx, strings.TrimPrefix(route, "/maintenance/tasks/"))
		}
		for key, list := range values {
			if (route != "/maintenance" && route != "/maintenance/log") || (key != "page" && key != "scope") || len(list) != 1 {
				return nil, &httpclient.ClientError{Status: 400, Code: "invalid_query", Reason: "unsupported or repeated maintenance query"}
			}
		}
		return p.maintenanceRead(ctx, route, values.Get("page"), values.Get("scope"))
	}
	if strings.HasPrefix(route, "/commands/") {
		if p.config.runtimeRefactoring != nil {
			if r, e := p.refactoringReceipt(ctx, strings.TrimPrefix(route, "/commands/")); e != nil {
				return nil, e
			} else if r != nil {
				return r, nil
			}
		}
		if p.config.runtimeContinuationSuggestion != nil {
			if r, e := p.ContinuationSuggestionReceipt(ctx, strings.TrimPrefix(route, "/commands/")); e != nil {
				return nil, e
			} else if r != nil {
				return r, nil
			}
		}
		if p.config.runtimeRoadmaps != nil {
			if r, e := p.RoadmapReceipt(ctx, strings.TrimPrefix(route, "/commands/")); e != nil {
				return nil, e
			} else if r != nil {
				return r, nil
			}
		}
		if r, e := p.environmentReceipt(ctx, strings.TrimPrefix(route, "/commands/")); e != nil {
			return nil, e
		} else if r != nil {
			return r, nil
		}
		if r, e := p.maintenanceReceipt(ctx, strings.TrimPrefix(route, "/commands/")); e != nil {
			return nil, e
		} else if r != nil {
			return r, nil
		}
	}
	return p.Server.Read(ctx, route, values)
}

func projectStoppingError() error {
	return &httpclient.ClientError{Status: 503, Code: "service_stopping", Reason: "application host is stopping"}
}
