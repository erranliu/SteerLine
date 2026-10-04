package steerlineapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/steerline/query"
	"github.com/erranliu/SteerLine/internal/steerlineclient"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func OpenClient(ctx context.Context, path string) (*steerlineclient.Client, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return openProjectClient(ctx, api.ProjectRef{ConfigPath: abs})
}

func openProjectClient(ctx context.Context, ref api.ProjectRef) (*steerlineclient.Client, error) {
	return steerlineclient.Open(ctx, ref, ClientResources)
}

// ClientResources is the application construction seam. It binds one immutable
// config/directory to pure ports; all product orchestration lives in the adapter.
func ClientResources(ctx context.Context, ref api.ProjectRef) (api.Resources, error) {
	c, err := LoadQueryConfig(ref.ConfigPath)
	if err != nil {
		return api.Resources{}, err
	}
	return clientResources(ctx, c, ref)
}

func clientResources(ctx context.Context, c steerline.Config, ref api.ProjectRef) (api.Resources, error) {
	if err := c.ResolveIdentity(ctx); err != nil {
		return api.Resources{}, err
	}
	abs, err := filepath.Abs(ref.ConfigPath)
	if err != nil {
		return api.Resources{}, err
	}
	ref.ConfigPath = abs
	ref.Identity = c.Identity()
	ref.RunDir = c.RunDir
	ref.Presentation = projectPresentation(c)
	ref.ApplicationHome, ref.ApplicationBin = HomeDir(), BinDir()
	ref.Presentation.ServiceInstruction = clientServiceInstruction(ref)
	if ref.Key == "" {
		ref.Key = ref.ConfigPath + "\x00" + ref.RunDir
	}
	freshness := 30 * time.Second
	if poll, err := time.ParseDuration(c.TaskPoll); err == nil && poll*3 > freshness {
		freshness = poll * 3
	}
	return api.Resources{
		Project: ref, CandidateFreshnessLimit: freshness, DefaultRequestedBy: steerline.DefaultRequestedBy(), DefaultAnswerer: steerline.AnswererName(), ExecutableTypes: append([]string(nil), c.ExecutableTypes...),
		OpenReader: func(ctx context.Context) (api.Reader, error) {
			cfg := c
			s, err := steerline.OpenNativeTaskReader(ctx, &cfg)
			if err != nil {
				return nil, err
			}
			return &clientReader{Service: query.NewService(s, c.Identity()), ConversationService: steerline.NewConversationQueryService(s, c.Identity()), store: s, project: c.Identity()}, nil
		},
		OpenCommands: func(ctx context.Context, mutation bool) (api.TaskCommands, error) {
			cfg := c
			s, err := steerline.OpenNativeTaskProject(ctx, &cfg)
			if err != nil {
				return nil, err
			}
			var lock *steerline.RepoLock
			if mutation {
				if err = s.Close(); err != nil {
					return nil, err
				}
				lock, err = steerline.LockRepository(c.Identity())
				if err != nil {
					return nil, err
				}
				s, err = steerline.OpenNativeTaskCommandStore(ctx, &cfg)
				if err != nil {
					lock.Close()
					return nil, err
				}
			}
			commands, err := newClientCommands(s, c, lock)
			if err != nil {
				_ = s.Close()
				if lock != nil {
					lock.Close()
				}
				return nil, err
			}
			return commands, nil
		},
		Connect: func(ctx context.Context, obs *api.ObservationSnapshot) api.ServiceConnection {
			if obs == nil {
				return steerline.ConnectService(ctx, c)
			}
			return steerline.ConnectServiceFromSnapshot(ctx, c, *obs)
		},
		Observe: func(rt *contracts.Runtime, err error, at time.Time) api.ObservationSnapshot {
			return steerline.ObserveRuntimeSnapshot(c, rt, err, at)
		},
		ProbeProcess: steerline.ProbeServiceProcess,
		Control:      clientControl{config: c}, Decision: clientDecision{config: c},
		TaskSnapshot: func(ctx context.Context) (*contracts.TaskSnapshot, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return steerline.ReadTaskSnapshot(c)
		},
		ResolveStartTask: func(ctx context.Context, id, profileID string) (productapi.ControlPayload, error) {
			if profileID == "" {
				return productapi.ControlPayload{}, errors.New("start_task requires -execution-profile")
			}
			_, p, err := c.ResolveExecutionProfile(profileID, "")
			if err != nil {
				return productapi.ControlPayload{}, err
			}
			return productapi.ControlPayload{TaskID: id, ExecutionProfileID: p.ID, ExecutionProfileDigest: p.Digest()}, nil
		},
		InspectRecovery: func(ctx context.Context, id string) (api.RecoveryInspection, error) {
			return steerline.InspectRecovery(ctx, c, id)
		},
		DecisionAvailability: func(ctx context.Context, run *contracts.Run) string {
			if run == nil || run.Decision == nil {
				return "当前没有待回答的问题"
			}
			if run.Decision.RequestPath != "" || run.Decision.AnswerPath != "" {
				return "问题绑定了已移除的 sidecar-answer 格式；保留状态并转入人工恢复，不导入答案文件"
			}
			if err := steerline.DecisionReplyEligibility(ctx, c, run); err != nil {
				return err.Error()
			}
			if c.ExecutionDirectory == nil {
				return "执行目录不可用；不能续跑原问题"
			}
			if _, err := c.ExecutionDirectory.ResolveIdentity(steerline.ExecutionRegistrationKey{Backend: executionbackend.BackendKind(run.Backend), Protocol: executionbackend.ExecutionProtocol(run.Protocol), Agent: run.Agent}); err != nil {
				return "原执行后端/协议不支持决策续跑"
			}
			return ""
		},
	}, nil
}

type clientReader struct {
	query.Service
	query.ConversationService
	store   persistence.Connection
	project string
}

func (r *clientReader) Close() error { return r.store.Close() }

func (r *clientReader) Domain(ctx context.Context) (contracts.DomainSnapshot, error) {
	return r.store.Query(ctx, r.project)
}

func (r *clientReader) DatabaseVersion(ctx context.Context) (string, uint64, error) {
	return r.store.DatabaseVersion(ctx)
}

func (r *clientReader) Command(ctx context.Context, id string) (storageports.CommandRecord, error) {
	v, err := r.store.Command(ctx, r.project, id)
	if errors.Is(err, storageports.ErrNotFound) {
		return v, errors.Join(productapi.ErrRecordNotFound, err)
	}
	return v, err
}

// These compile-time assertions keep the injected port tied to real consumers.
var _ api.Reader = (*clientReader)(nil)

type clientControl struct{ config steerline.Config }

func (r clientControl) Submit(ctx context.Context, in productapi.ControlInput) (productapi.ControlRequest, error) {
	if err := ctx.Err(); err != nil {
		return productapi.ControlRequest{}, err
	}
	var payload any
	if in.Payload != (productapi.ControlPayload{}) {
		payload = in.Payload
	}
	return steerline.SubmitControlTargeted(r.config, string(in.Action), payload, in.RequestedBy, in.RequestID, in.InstanceID, in.ExpectedRevision)
}

func (r clientControl) Receipt(ctx context.Context, id string) (*productapi.ControlReceipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := r.Request(ctx, id); err != nil {
		return nil, err
	}
	return steerline.ReadVerifiedControlReceipt(r.config, id)
}

func (r clientControl) Request(ctx context.Context, id string) (*productapi.ControlRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	req, err := steerline.ReadControlRequest(r.config, id)
	if err == nil && req.RepositoryIdentity != r.config.Identity() {
		return nil, errors.New("control request project identity does not match")
	}
	return req, err
}

func (r clientControl) Stop(ctx context.Context, by string, timeout time.Duration) (productapi.ServiceStopResult, error) {
	return steerline.StopService(ctx, r.config, by, timeout)
}

type clientDecision struct{ config steerline.Config }

func (r clientDecision) Eligibility(ctx context.Context, run *contracts.Run) error {
	return steerline.DecisionReplyEligibility(ctx, r.config, run)
}

func (r clientDecision) Submit(ctx context.Context, in api.DecisionInput) (storageports.CommandRecord, bool, error) {
	a := in.Answer
	return steerline.SubmitAnswerDecision(ctx, r.config, a.RunID, a.DecisionID, a.OptionID, a.Note, in.RequestedBy, a.ExpectedDecisionVersion, a.ExpectedRunRevision, a.ServiceInstanceID, in.CommandID)
}

func (r clientDecision) Answer(ctx context.Context, in productapi.AnswerInput) (storageports.CommandRecord, bool, error) {
	return steerline.SubmitDecisionAnswer(ctx, r.config, steerline.AnswerOptions{RunID: in.RunID, DecisionID: in.DecisionID, Option: in.Option, Note: in.Note, By: in.RequestedBy, ExpectedVersion: in.ExpectedVersion, CommandID: in.CommandID})
}

// projectPresentation exposes only accepted non-secret display fields and fixed
// profile digests. Configuration compilation stays with the original owner.
func projectPresentation(c steerline.Config) api.ProjectPresentation {
	result := api.ProjectPresentation{NativeWorkflow: c.WorkflowProtocol == executionmodel.Version, ConfigDigest: c.LoadedConfigDigest(), TaskAuthority: c.TaskAuthority, WorkflowProtocol: c.WorkflowProtocol, ShutdownGrace: c.ShutdownGrace, DefaultProfileID: c.DefaultProfileID()}
	for _, p := range c.AvailableExecutionProfiles() {
		reason := p.StaticUnavailableReason()
		if reason == "" {
			if _, _, err := c.ResolveExecutionProfile(p.ID, p.Digest()); err != nil {
				reason = err.Error()
			}
		}
		result.Profiles = append(result.Profiles, api.ExecutionProfileSummary{ID: p.ID, Name: p.Name, Host: p.Host, Agent: p.Agent, Digest: p.Digest(), AcceptedConfigVersion: p.AcceptedConfigVersion, UnavailableReason: reason, ExecutionPolicy: p.ExecutionPolicy, PublicationPolicy: p.PublicationPolicy, Publication: p.Publication, ImplementationDisplay: append([]string(nil), p.ImplementationDisplay...)})
	}
	return result
}

func clientServiceInstruction(ref api.ProjectRef) string {
	if runtime.GOOS != "windows" {
		return "steerline start-service -config " + ref.ConfigPath
	}
	launcher := filepath.Join(ref.ApplicationHome, "service-window.bat")
	binary := filepath.Join(ref.ApplicationBin, "steerline.exe")
	if _, err := os.Stat(binary); err != nil {
		return fmt.Sprintf("服务窗口不可用：缺少 %s；运行 steerline install", binary)
	}
	if _, err := os.Stat(launcher); err != nil {
		return fmt.Sprintf("服务窗口不可用：缺少 %s；运行 steerline launchers", launcher)
	}
	return launcher
}

func (r *clientReader) ActivityEvents(ctx context.Context, execution, attempt string, after int64) ([]storageports.DomainEvent, error) {
	return r.store.Events(ctx, r.project, execution, attempt, after, query.DefaultEventPageSize, query.MaxEventPageBytes)
}
