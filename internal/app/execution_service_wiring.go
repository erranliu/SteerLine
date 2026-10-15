package app

import (
	"context"
	"errors"
	"sync"
	"time"

	steerline "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
	"github.com/erranliu/SteerLine/internal/execution/hostport"
	productclient "github.com/erranliu/SteerLine/internal/product/client"
	"github.com/erranliu/SteerLine/internal/product/httpapi"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

type executionServiceLifetime struct {
	service  *execution.Service
	config   steerline.Config
	mode     string
	product  *productclient.Client
	localAPI *serviceLocalAPI
}

func (r executionServiceLifetime) Run(ctx context.Context) error {
	actor := serviceReadyHook{Service: r.service, ready: func() error {
		return r.localAPI.Publish(r.service.State.Service.InstanceID, r.service.State.Service.ProcessIdentity,
			r.service.State.Service.Capabilities, r.service.State.Service.Ready, serviceReadyDiagnostic(r.service))
	}}
	runErr := steerline.RunService(ctx, actor, r.config, r.mode)
	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return errors.Join(runErr, r.localAPI.Close(closeCtx))
}

func (r executionServiceLifetime) Close() error {
	var closeProduct error
	if r.product != nil {
		closeProduct = r.product.Close()
	}
	var closeAPI error
	if r.localAPI != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		closeAPI = r.localAPI.Close(ctx)
	}
	return errors.Join(r.service.Close(), closeProduct, closeAPI)
}

type serviceReadyHook struct {
	*execution.Service
	ready func() error
}

func (s serviceReadyHook) Initialize(ctx context.Context, cancel context.CancelFunc) error {
	if err := s.Service.Initialize(ctx, cancel); err != nil {
		return err
	}
	if s.ready != nil {
		return s.ready()
	}
	return nil
}

func localAPIProcessIdentity(value *executionbackend.ProcessIdentity) httpapi.ProcessIdentity {
	if value == nil {
		return httpapi.ProcessIdentity{}
	}
	return httpapi.ProcessIdentity{Kind: value.Kind, PID: value.PID, Created: value.Created, Executable: value.Executable}
}

func serviceReadyDiagnostic(service *execution.Service) string {
	if service == nil {
		return "service instance is unavailable"
	}
	if service.State.Service.Error != nil {
		return *service.State.Service.Error
	}
	if !service.State.Service.Ready {
		return "service initialization completed without publishing ready=true"
	}
	return ""
}

type serviceLocalAPI struct {
	mu           sync.Mutex
	product      *productclient.Client
	runDir       string
	projectID    string
	capabilities []string
	endpoint     *httpapi.Server
	published    bool
	ready        bool
	diagnostic   string
}

func (s *serviceLocalAPI) Publish(instance string, process *executionbackend.ProcessIdentity, capabilities []string, ready bool, diagnostic string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.endpoint != nil && s.published && s.ready == ready && s.diagnostic == diagnostic {
		return nil
	}
	if s.endpoint == nil {
		if len(capabilities) == 0 {
			capabilities = s.capabilities
		}
		endpoint, err := httpapi.Prepare(context.Background(), httpapi.Ports{Query: s.product, Commands: s.product,
			Authority: s.product, NativeViewer: s.product}, httpapi.Options{RunDir: s.runDir, ProjectIdentity: s.projectID,
			InstanceID: instance, Process: localAPIProcessIdentity(process), Capabilities: append([]string(nil), capabilities...)})
		if err != nil {
			return err
		}
		if err = endpoint.Start(); err != nil {
			return errors.Join(err, endpoint.Close(context.Background()))
		}
		s.endpoint = endpoint
	}
	if err := s.endpoint.SetReady(instance, ready, diagnostic); err != nil {
		return err
	}
	s.published, s.ready, s.diagnostic = true, ready, diagnostic
	return nil
}

func (s *serviceLocalAPI) Close(ctx context.Context) error {
	s.mu.Lock()
	endpoint := s.endpoint
	s.endpoint = nil
	s.published = false
	s.mu.Unlock()
	var apiErr error
	if endpoint != nil {
		apiErr = endpoint.Close(ctx)
	}
	return apiErr
}

type serviceHTTPHost struct {
	hostport.HostPort
	localAPI *serviceLocalAPI
}

func (h serviceHTTPHost) PublishReady(instanceID string, processIdentity *executionbackend.ProcessIdentity, dispatchMode string) error {
	if err := h.localAPI.Publish(instanceID, processIdentity, h.localAPI.capabilities, true, ""); err != nil {
		return err
	}
	return h.HostPort.PublishReady(instanceID, processIdentity, dispatchMode)
}

// NewService uses the directory and process adapter already bound to the
// loaded application configuration.
func NewService(config Config) (*execution.Service, error) {
	return NewServiceWithLifecycleResolver(config, config.ExecutionDirectory)
}

// NewServiceWithLifecycleResolver permits an explicit decorator around the
// same catalog's lifecycle ports, while preserving all other app wiring.
func NewServiceWithLifecycleResolver(config Config, resolver executionbackend.LifecycleResolver) (*execution.Service, error) {
	return NewServiceWithPorts(config, resolver, newConversationSources())
}

// NewServiceWithPorts composes explicitly supplied execution and history ports
// with the directory already accepted by this application instance.
func NewServiceWithPorts(config Config, resolver executionbackend.LifecycleResolver, sources conversationtypes.ConversationSourcePort) (*execution.Service, error) {
	return newServiceWithHost(config, resolver, sources, nil)
}

func newConfiguredServiceWithHost(config Config, host hostport.HostPort) (*execution.Service, error) {
	return newServiceWithHost(config, config.ExecutionDirectory, newConversationSources(), host)
}

func newServiceWithHost(config Config, resolver executionbackend.LifecycleResolver, sources conversationtypes.ConversationSourcePort, host hostport.HostPort) (*execution.Service, error) {
	return newServiceWithTaskStore(context.Background(), config, resolver, sources, host, nil)
}
func newServiceWithTaskStore(ctx context.Context, config Config, resolver executionbackend.LifecycleResolver, sources conversationtypes.ConversationSourcePort, host hostport.HostPort, bind func(*sqlite.SQLiteStore)) (*execution.Service, error) {
	if config.ExecutionDirectory == nil || config.ProcessHost == nil {
		return nil, errors.New("application configuration is missing its execution directory or process host")
	}
	var err error
	config, err = PrepareServiceConfig(config)
	if err != nil {
		return nil, err
	}
	hostConfig, err := hostRuntimeConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	connection, err := OpenNativeTaskCommandStore(ctx, &config)
	if err != nil {
		return nil, err
	}
	if bind != nil {
		bind(connection)
	}
	tasks := NewTaskService(connection, config)
	if host == nil {
		host = steerline.NewServiceHost(hostConfig, "", "", nil, serviceAdmission{config: &config}, serviceActivityFormatter{})
	}
	stores := execution.ServiceStorage{State: connection, Commands: connection, Worker: connection, Conversations: connection, Records: connection, Resources: executionResources{}, Close: connection.Close}
	service, err := execution.NewService(config.executionConfig(), config.runtimeDependencies(), config.runtimeSettings(), resolver, sources, stores, tasks, tasks, host)
	if err != nil {
		return nil, errors.Join(err, connection.Close())
	}
	return service, nil

}
