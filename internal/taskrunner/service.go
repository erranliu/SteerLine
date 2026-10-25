// Package taskrunner coordinates task candidates through SteerRunner's CLI.
// It owns task eligibility and delivery; it never executes native operations.
package taskrunner

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type Store interface {
	runnerbridge.Store
	taskapp.Reader
	RunnerCandidates(context.Context, string, string) ([]runnerbridge.Candidate, error)
	ReserveRunnerCandidate(context.Context, taskmodel.FixedWork, runnerbridge.Candidate, bool) error
	StopRunnerCandidate(context.Context, string, string, uint64, string, string) (runnerbridge.Candidate, error)
	TaskGraphSnapshot(context.Context, string) (storageports.TaskGraphSnapshot, error)
	TaskResult(context.Context, string, string) (taskmodel.ResultReceipt, bool, error)
	CommitRunnerTaskResult(context.Context, storageports.RunnerResult) (taskmodel.TaskDeliveryReceipt, error)
}
type Profile struct {
	ID, Name          string
	UnavailableReason string
	Input             runnerclient.RunInput
}
type Service struct {
	Project, Claimant, StateDir, DefaultProfile string
	Profiles                                    map[string]Profile
	Tasks                                       taskapp.Service
	Store                                       Store
	Bridge                                      *runnerbridge.Bridge
	ResolveBase                                 func(context.Context, taskmodel.FixedWork, bool) (string, error)
	Admit                                       func(context.Context) error
	RunnerBuild                                 runnercontract.BuildPin
	ReserveExecution                            func(context.Context, taskmodel.FixedWork, runnerbridge.Candidate, bool) error
	PrepareExecution                            func(context.Context, taskmodel.FixedWork, bool) (taskmodel.FixedWork, string, error)
	PrepareTestPlan                             func(context.Context, taskmodel.FixedWork, *runnerclient.RunInput) (json.RawMessage, error)
	mu                                          sync.Mutex
}
type Start struct {
	RequestID     string `json:"request_id"`
	TaskID        string `json:"task_id"`
	TaskVersion   uint64 `json:"task_version"`
	GraphRevision uint64 `json:"graph_revision,omitempty"`
	ProfileID     string `json:"profile_id"`
	Candidate     bool   `json:"candidate"`
	Explicit      bool   `json:"explicit"`
}

func digest(value any) string {
	body, _ := json.Marshal(value)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func id() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw[:])
}
func failure(code, message string) error { return &taskmodel.Error{Code: code, Message: message} }

func (s *Service) Start(ctx context.Context, input Start) (runnerbridge.Candidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if input.RequestID == "" || input.TaskVersion == 0 || input.Explicit && input.GraphRevision == 0 || !filepath.IsAbs(s.StateDir) {
		return runnerbridge.Candidate{}, failure("invalid_batch", "start requires fixed task version, request ID and absolute Runner state directory")
	}
	if old, err := s.Store.Candidate(ctx, s.Project, input.RequestID); err == nil {
		if old.StartDigest != digest(input) {
			return old, failure("idempotency_conflict", "start request belongs to different original input")
		}
		return old, nil
	} else if !errors.Is(err, storageports.ErrNotFound) {
		return runnerbridge.Candidate{}, err
	}
	if s.Admit != nil {
		if err := s.Admit(ctx); err != nil {
			return runnerbridge.Candidate{}, failure("runner_unavailable", err.Error())
		}
	}
	work, err := s.Tasks.Fixed(ctx, s.Project, input.TaskID, input.TaskVersion, input.Candidate)
	if err != nil {
		return runnerbridge.Candidate{}, err
	}
	if input.GraphRevision != 0 && input.GraphRevision != work.Authority.GraphRevision {
		return runnerbridge.Candidate{}, failure("stale_version", "displayed task graph changed before reservation")
	}
	if !s.Tasks.CanExecuteType(work.Record.Type) {
		return runnerbridge.Candidate{}, failure("task_not_ready", "task type cannot execute")
	}
	profileID := input.ProfileID
	if profileID == "" {
		profileID = s.DefaultProfile
	}
	profile, ok := s.Profiles[profileID]
	if !ok {
		return runnerbridge.Candidate{}, failure("profile_not_found", "selected Runner profile is unavailable")
	}
	if profile.UnavailableReason != "" {
		return runnerbridge.Candidate{}, failure("unsupported_capability", profile.UnavailableReason)
	}
	if profile.Input.Workflow.ID == "" || profile.Input.Workflow.Version <= 0 {
		return runnerbridge.Candidate{}, failure("invalid_profile", "execution preset must select a published workflow explicitly")
	}
	if profile.Input.Workflow.ID == "generate" || profile.Input.Workflow.ID == "conversation" {
		return runnerbridge.Candidate{}, failure("invalid_profile", "content workflows cannot execute a development task")
	}
	var base string
	if s.PrepareExecution != nil {
		work, base, err = s.PrepareExecution(ctx, work, input.Candidate)
	} else {
		base, err = s.ResolveBase(ctx, work, input.Candidate)
	}
	if err != nil {
		return runnerbridge.Candidate{}, err
	}
	if !taskmodel.ValidCommit(base) {
		return runnerbridge.Candidate{}, failure("base_missing_delivery", "fixed code base is invalid")
	}
	if err := s.Tasks.VerifyContractFiles(ctx, work.Contract); err != nil {
		return runnerbridge.Candidate{}, err
	}
	var run runnerclient.RunInput
	// Profile values are frozen per candidate; do not retain shared map/slice aliases.
	raw, _ := json.Marshal(profile.Input)
	if err := json.Unmarshal(raw, &run); err != nil {
		return runnerbridge.Candidate{}, err
	}
	run.Title = work.Record.Title
	run.Repository.URL, run.Repository.Remote, run.Repository.BaseBranch, run.Repository.BaseRevision = work.Contract.Repository, work.Contract.Remote, work.Contract.Branch, base
	run.Publication.BaseBranch = work.Contract.Branch
	doc := func(ref taskmodel.FileReference) runnerclient.Document {
		return runnerclient.Document{Path: ref.Path}
	}
	run.Documents = runnerclient.Documents{Spec: doc(work.Contract.SpecRef), Plan: doc(work.Contract.PlanRef)}
	seen := map[string]bool{work.Contract.SpecRef.Path: true, work.Contract.PlanRef.Path: true}
	for _, ref := range append(append([]taskmodel.FileReference{}, work.Contract.CommonRefs...), work.Contract.AcceptanceRefs...) {
		if !seen[ref.Path] {
			run.Documents.References = append(run.Documents.References, doc(ref))
			seen[ref.Path] = true
		}
	}
	if run.Correlation == nil {
		run.Correlation = map[string]string{}
	}
	var testPlan json.RawMessage
	if s.PrepareTestPlan != nil {
		testPlan, err = s.PrepareTestPlan(ctx, work, &run)
		if err != nil {
			return runnerbridge.Candidate{}, err
		}
	}

	run.Correlation["steerline_project"] = s.Project
	run.Correlation["steerline_task_id"], run.Correlation["steerline_candidate_id"] = input.TaskID, input.RequestID
	raw, err = json.Marshal(run)
	if err != nil {
		return runnerbridge.Candidate{}, err
	}
	claimant := s.Claimant
	if input.Candidate {
		claimant = work.Record.Claimant
	}
	c := runnerbridge.Candidate{Project: s.Project, TaskID: input.TaskID, ID: input.RequestID, Revision: 1, Explicit: input.Explicit, StateDir: s.StateDir, Claimant: claimant, ContractDigest: work.Contract.Digest, BaseRevision: base, Participation: "active", StartDigest: digest(input), ProfileID: profileID, ProfileName: profile.Name, ProfileDigest: runnercontract.ConfigurationDigest(profile.Input.Execution, profile.Input.Workflow), RunnerBuild: s.RunnerBuild, CreatedAt: time.Now().UTC(), SubmitState: "pending", Submit: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: id(), Input: raw}}
	c.TestPlan = testPlan
	c.Contract = work.Contract
	c.MaterialDigest = work.MaterialDigest
	reserve := s.Store.ReserveRunnerCandidate
	if s.ReserveExecution != nil {
		reserve = s.ReserveExecution
	}
	if err := reserve(ctx, work, c, input.Candidate); err != nil {
		return runnerbridge.Candidate{}, err
	}
	return s.Store.Candidate(ctx, s.Project, c.ID)
}
func (s *Service) Reconcile(ctx context.Context) error {
	rows, err := s.Store.RunnerCandidates(ctx, s.Project, "")
	if err != nil {
		return err
	}
	var failures []error
	for _, c := range rows {
		if ctx.Err() != nil {
			break
		}
		if c.SubmitState == "cancelled" || c.SubmitState == "rejected" && c.RunID == "" {
			continue
		}
		if err := s.Bridge.Step(ctx, s.Project, c.ID); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *Service) RecoverInputs(ctx context.Context) error {
	rows, err := s.Store.RunnerCandidates(ctx, s.Project, "")
	if err != nil {
		return err
	}
	for _, c := range rows {
		if err := s.Bridge.RecoverInputs(ctx, s.Project, c.ID); err != nil {
			return err
		}
		if err := s.Bridge.VerifyBinding(ctx, s.Project, c.ID); err != nil {
			return fmt.Errorf("original Runner binding remains unverified for candidate %s: %w", c.ID, err)
		}
	}
	return nil
}

func (s *Service) Stop(ctx context.Context, candidate string, revision uint64, requestID, requestedBy string) (runnerbridge.Candidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result runnerbridge.Candidate
	err := s.Bridge.WithCandidate(s.Project, candidate, func() error {
		var err error
		result, err = s.Store.StopRunnerCandidate(ctx, s.Project, candidate, revision, requestID, requestedBy)
		return err
	})
	return result, err
}
func (s *Service) Dispatch(ctx context.Context) error {
	ready, err := s.Tasks.Ready(ctx, s.Project)
	if err != nil {
		return err
	}
	for _, task := range ready {
		if !s.Tasks.CanExecuteType(task.Type) {
			continue
		}
		_, err := s.Start(ctx, Start{RequestID: id(), TaskID: task.TaskID, TaskVersion: task.Version})
		return err
	}
	return nil
}
func (s *Service) Deliver(ctx context.Context, taskID, candidateID string, version uint64) (taskmodel.TaskDeliveryReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	graph, err := s.Store.TaskGraphSnapshot(ctx, s.Project)
	if err != nil {
		return taskmodel.TaskDeliveryReceipt{}, err
	}
	record, ok := graph.ByID[taskID]
	if !ok {
		return taskmodel.TaskDeliveryReceipt{}, failure("task_not_found", "task was not found")
	}
	if existing, ok := graph.Deliveries[taskID]; ok {
		if record.Status != "closed" || existing.SelectedWorkflowRunID != candidateID {
			return taskmodel.TaskDeliveryReceipt{}, failure("inconsistent_delivery", "task already has another delivery")
		}
		original, found, err := s.Store.TaskResult(ctx, s.Project, candidateID)
		if err != nil {
			return taskmodel.TaskDeliveryReceipt{}, err
		}
		if !found || original.Input.ExpectedTaskVersion != version {
			return taskmodel.TaskDeliveryReceipt{}, failure("stale_version", "delivery retry differs from the original task version")
		}
		return taskmodel.TaskDeliveryReceipt{TaskID: taskID, WorkflowRunID: candidateID, TaskVersion: original.TaskVersion, GraphRevision: original.GraphRevision, Status: "closed", Delivery: existing}, nil
	}
	if record.Status == "discarded" {
		return taskmodel.TaskDeliveryReceipt{}, failure("task_discarded", "discarded task cannot receive delivery")
	}
	if record.Status != "in_progress" || record.Version != version {
		return taskmodel.TaskDeliveryReceipt{}, failure("stale_version", "task version changed")
	}
	c, err := s.Store.Candidate(ctx, s.Project, candidateID)
	if err != nil {
		return taskmodel.TaskDeliveryReceipt{}, err
	}
	if c.TaskID != taskID {
		return taskmodel.TaskDeliveryReceipt{}, failure("immutable_task", "candidate belongs to another task")
	}
	c, started, err := s.Bridge.RefreshPR(ctx, s.Project, candidateID)
	if err != nil {
		return taskmodel.TaskDeliveryReceipt{}, err
	}
	return s.Store.CommitRunnerTaskResult(ctx, storageports.RunnerResult{Project: s.Project, TaskID: taskID, CandidateID: c.ID, TaskVersion: version, CandidateRevision: c.Revision, AuthorityVersion: graph.Authority.AuthorityVersion, GraphRevision: graph.Authority.GraphRevision, VerificationStarted: started})
}
