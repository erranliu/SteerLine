package steerlineapp

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/query"
	"github.com/erranliu/SteerLine/internal/steerline/task"

	"github.com/erranliu/SteerLine/internal/steerlineclient"
)

func OpenClient(ctx context.Context, path string) (*steerlineclient.Client, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return steerlineclient.Open(ctx, api.ProjectRef{ConfigPath: abs}, ClientResources)
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
	if ref.Key == "" {
		ref.Key = ref.ConfigPath + "\x00" + ref.RunDir
	}
	logs := steerlineclient.NewLogReader(func(first []byte) api.LogNormalizer { return selectLogNormalizer(first) })
	freshness := 30 * time.Second
	if poll, err := time.ParseDuration(c.TaskPoll); err == nil && poll*3 > freshness {
		freshness = poll * 3
	}
	return api.Resources{
		Project: ref, CandidateFreshnessLimit: freshness, DefaultRequestedBy: steerline.DefaultRequestedBy(), ExecutableTypes: append([]string(nil), c.ExecutableTypes...),
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
			commands, err := NewCommandService(s, c)
			if err != nil {
				_ = s.Close()
				if lock != nil {
					lock.Close()
				}
				return nil, err
			}
			return &clientCommands{service: commands, config: c, store: s, lock: lock}, nil
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
		ResolveStartTask: func(ctx context.Context, id, profileID string) (api.ControlPayload, error) {
			if profileID == "" {
				return api.ControlPayload{}, errors.New("start_task requires -execution-profile")
			}
			_, p, err := c.ResolveExecutionProfile(profileID, "")
			if err != nil {
				return api.ControlPayload{}, err
			}
			return api.ControlPayload{TaskID: id, ExecutionProfileID: p.ID, ExecutionProfileDigest: p.Digest()}, nil
		},
		ReadLog: logs.ReadLog,
	}, nil
}

type clientReader struct {
	query.Service
	query.ConversationService
	store   *steerline.SQLiteStore
	project string
}

func (r *clientReader) Close() error { return r.store.Close() }
func (r *clientReader) Domain(ctx context.Context) (contracts.DomainSnapshot, error) {
	return r.store.Query(ctx, r.project)
}
func (r *clientReader) DatabaseVersion(ctx context.Context) (string, uint64, error) {
	return r.store.DatabaseVersion(ctx)
}
func (r *clientReader) Command(ctx context.Context, id string) (contracts.CommandRecord, error) {
	v, err := r.store.Command(ctx, r.project, id)
	if errors.Is(err, sql.ErrNoRows) {
		return v, errors.Join(api.ErrRecordNotFound, err)
	}
	return v, err
}

// These compile-time assertions keep the injected port tied to real consumers.
var _ api.Reader = (*clientReader)(nil)

type clientCommands struct {
	service steerline.CommandService
	config  steerline.Config
	store   *steerline.SQLiteStore
	lock    *steerline.RepoLock
}

func (r *clientCommands) Close() error {
	err := r.store.Close()
	if r.lock != nil {
		r.lock.Close()
	}
	return err
}
func (r *clientCommands) Validate(ctx context.Context, payload []byte) (task.TaskBatchValidation, error) {
	return r.service.ValidateTaskBatch(ctx, r.config, payload)
}
func (r *clientCommands) Create(ctx context.Context, in api.CreateInput) (task.TaskBatchReceipt, error) {
	return r.service.CreateTaskBatch(ctx, r.config, in.Payload, in.RequestID, in.ExpectedAuthorityVersion, in.RequestedBy)
}
func (r *clientCommands) Discard(ctx context.Context, in api.DiscardInput) (task.TaskDiscardReceipt, error) {
	return r.service.DiscardNativeTask(ctx, r.config, in.TaskID, in.RequestID, in.ExpectedVersion, in.Reason, in.RequestedBy)
}
func (r *clientCommands) AddPredecessors(ctx context.Context, in task.TaskPredecessorAddInput) (task.TaskPredecessorAddReceipt, error) {
	return r.service.AddNativeTaskPredecessors(ctx, r.config, in)
}

type clientControl struct{ config steerline.Config }

func (r clientControl) Submit(ctx context.Context, in api.ControlInput) (api.ControlRequest, error) {
	if err := ctx.Err(); err != nil {
		return api.ControlRequest{}, err
	}
	var payload any
	if in.Payload != (api.ControlPayload{}) {
		payload = in.Payload
	}
	return steerline.SubmitControlTargeted(r.config, string(in.Action), payload, in.RequestedBy, in.RequestID, in.InstanceID, in.ExpectedRevision)
}
func (r clientControl) Receipt(ctx context.Context, id string) (*api.ControlReceipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := r.Request(ctx, id); err != nil {
		return nil, err
	}
	return steerline.ReadVerifiedControlReceipt(r.config, id)
}
func (r clientControl) Request(ctx context.Context, id string) (*api.ControlRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	req, err := steerline.ReadControlRequest(r.config, id)
	if err == nil && req.RepositoryIdentity != r.config.Identity() {
		return nil, errors.New("control request project identity does not match")
	}
	return req, err
}
func (r clientControl) Stop(ctx context.Context, by string, timeout time.Duration) (api.ServiceStopResult, error) {
	return steerline.StopService(ctx, r.config, by, timeout)
}

type clientDecision struct{ config steerline.Config }

func (r clientDecision) Eligibility(ctx context.Context, run *contracts.Run) error {
	return steerline.DecisionReplyEligibility(ctx, r.config, run)
}
func (r clientDecision) Submit(ctx context.Context, in api.DecisionInput) (contracts.CommandRecord, bool, error) {
	a := in.Answer
	return steerline.SubmitAnswerDecision(ctx, r.config, a.RunID, a.DecisionID, a.OptionID, a.Note, in.RequestedBy, a.ExpectedDecisionVersion, a.ExpectedRunRevision, a.ServiceInstanceID, in.CommandID)
}
func (r clientDecision) Answer(ctx context.Context, in api.AnswerInput) (contracts.CommandRecord, bool, error) {
	return steerline.SubmitDecisionAnswer(ctx, r.config, steerline.AnswerOptions{RunID: in.RunID, DecisionID: in.DecisionID, Option: in.Option, Note: in.Note, By: in.RequestedBy, ExpectedVersion: in.ExpectedVersion, CommandID: in.CommandID})
}
