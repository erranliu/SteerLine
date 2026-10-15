package app

import (
	"context"
	"errors"
	"sync"
	"time"

	steerline "github.com/erranliu/SteerLine/internal/app/host"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/hostport"
	productclient "github.com/erranliu/SteerLine/internal/product/client"
	"github.com/erranliu/SteerLine/internal/product/httpapi"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type executionServiceLifetime struct {
	service  *taskrunner.Actor
	config   steerline.Config
	mode     string
	product  *productclient.Client
	localAPI *serviceLocalAPI
}

func (r executionServiceLifetime) Run(ctx context.Context) error {
	actor := serviceReadyHook{Actor: r.service, ready: func() error {
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
	*taskrunner.Actor
	ready func() error
}

func (s serviceReadyHook) Initialize(ctx context.Context, cancel context.CancelFunc) error {
	if err := s.Actor.Initialize(ctx, cancel); err != nil {
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

func serviceReadyDiagnostic(service *taskrunner.Actor) string {
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

// NewService constructs the caller Task actor; SteerRunner owns execution.
func NewService(config Config) (*taskrunner.Actor, error) {
	return newRunnerServiceWithHost(config, nil, "manual")
}
func newConfiguredServiceWithHost(config Config, host hostport.HostPort, mode string) (*taskrunner.Actor, error) {
	return newRunnerServiceWithHost(config, host, mode)
}
