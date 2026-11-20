package refactoring

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	decision "github.com/erranliu/SteerLine/internal/steeragent/maintenance"
)

type contextOperation struct {
	ID            string                       `json:"operation_id"`
	TargetID      string                       `json:"target_id,omitempty"`
	Purpose       string                       `json:"purpose"`
	Prompt        string                       `json:"prompt,omitempty"`
	Feedback      string                       `json:"feedback,omitempty"`
	Status        string                       `json:"status,omitempty"`
	NativeOutcome string                       `json:"native_outcome,omitempty"`
	Committed     bool                         `json:"committed"`
	Diagnostic    string                       `json:"diagnostic,omitempty"`
	Result        *model.Result                `json:"result,omitempty"`
	Question      *runnercontract.Question     `json:"question,omitempty"`
	WorkspaceGit  *runnercontract.WorkspaceGit `json:"workspace_git,omitempty"`
}

type acceptedSummary struct {
	ID       string `json:"operation_id"`
	TargetID string `json:"target_id,omitempty"`
	Outcome  string `json:"outcome"`
	Summary  string `json:"summary,omitempty"`
}

type decisionContext struct {
	Mode            string                 `json:"mode"`
	DirectionPrompt string                 `json:"direction_prompt"`
	TargetID        string                 `json:"target_id,omitempty"`
	TargetStart     string                 `json:"target_start,omitempty"`
	TargetPrompt    string                 `json:"target_prompt,omitempty"`
	PR              *runnercontract.PRFact `json:"pr,omitempty"`
	Checks          string                 `json:"checks,omitempty"`
	Feedback        []contextOperation     `json:"feedback,omitempty"`
	History         []acceptedSummary      `json:"history,omitempty"`
	Answers         []model.Answer         `json:"answers,omitempty"`
	Question        *model.Question        `json:"answered_question,omitempty"`
}

func clippedSummary(s string, size int) string {
	if len(s) <= size {
		return s
	}
	for size > 0 && !utf8.RuneStart(s[size]) {
		size--
	}
	return s[:size] + "…"
}

func (s *Service) context(r model.Record, review model.ReviewEvidence) decisionContext {
	f := decisionContext{
		Mode: r.Phase, DirectionPrompt: r.Frozen.Direction.Prompt,
	}
	if r.Phase == "report" {
		f.PR, f.Checks = r.PR, review.Checks
		accepted := map[string]bool{}
		for _, op := range r.Operations {
			if op.AcceptedOperation != "" {
				accepted[op.AcceptedOperation] = true
			}
		}
		for _, op := range r.Operations {
			if r.PR != nil && accepted[op.ID] && op.Settled() && executionCommitted(op) && op.CommittedHead() == r.PR.HeadRevision {
				f.Feedback = append(f.Feedback, contextOperation{ID: op.ID, TargetID: op.TargetID, Purpose: op.Purpose,
					Prompt: op.Prompt, Feedback: op.Feedback, Result: op.Result, WorkspaceGit: op.WorkspaceGit,
					Status: op.Run.Status, NativeOutcome: op.Run.Final.NativeOutcome, Committed: true})
			}
		}
		return f
	}
	f.TargetID, f.TargetStart, f.TargetPrompt = r.TargetID, r.TargetStart, r.TargetPrompt
	if n := len(r.Answers); n > 0 {
		f.Answers = r.Answers
		for i := len(r.Operations) - 1; i >= 0; i-- {
			if q := r.Operations[i].Question; q != nil && q.ID == f.Answers[n-1].QuestionID {
				f.Question = q
				break
			}
		}

	}
	for i := 0; i < len(r.Operations); i++ {
		o := r.Operations[i]
		if !o.Settled() {
			continue
		}
		if o.Content(r).WorkflowID == "execute" && o.Run != nil && o.TargetID == r.TargetID {
			item := contextOperation{ID: o.ID, TargetID: o.TargetID, Purpose: o.Purpose, Prompt: o.Prompt, Result: o.Result, Question: o.Run.Question, WorkspaceGit: o.WorkspaceGit, Status: o.Run.Status, Diagnostic: o.Run.Diagnostic}
			item.Committed = executionCommitted(o)
			if o.Run.Final != nil {
				item.NativeOutcome = o.Run.Final.NativeOutcome
			}
			if o.Result == nil {
				item.Feedback = o.Feedback
			}
			f.Feedback = append(f.Feedback, item)
		}
	}
	for i := len(r.Operations) - 1; i >= 0 && len(f.History) < 24; i-- {
		o := r.Operations[i]
		if o.Settled() && o.AcceptedOperation != "" {
			f.History = append(f.History, acceptedSummary{ID: o.ID, TargetID: o.TargetID, Outcome: o.Purpose, Summary: clippedSummary(o.DecisionSummary, 512)})
		}
	}
	return f
}

type executionContext struct {
	Publication   map[string]string      `json:"publication,omitempty"`
	ManagedBranch string                 `json:"managed_branch,omitempty"`
	PR            *runnercontract.PRFact `json:"pr,omitempty"`
	AcceptedHead  string                 `json:"accepted_head,omitempty"`
}

type executionRequest struct {
	ActionID    string           `json:"action_id"`
	OperationID string           `json:"operation_id"`
	Instruction string           `json:"instruction"`
	Context     executionContext `json:"context"`
}

func (s *Service) prepareOperation(r model.Record, id, directory string, review model.ReviewEvidence) (model.Operation, error) {
	now := s.now()
	o := model.Operation{ID: id, Purpose: r.Phase, TargetID: r.TargetID, State: "pending", CreatedAt: now, Investigation: directory, Closing: r.FinishRequested || r.Input.Budget.DefaultTarget() && r.TargetID != ""}
	if r.Phase == "report" {
		o.PRVersion = prVersion(r.PR)
		o.ReviewChecks = review.Checks
	}
	correlation := map[string]string{"steerline_project": r.Project, "steerline_refactoring": r.ID, "refactoring_operation": id}
	var input any
	if r.Phase == "execute" {
		o.Purpose, o.Prompt = r.WriteKind, r.PendingPrompt
		var context executionContext
		local := o.Purpose == "implement" || o.Purpose == "repair"
		if !local {
			var err error
			o.Prompt, err = prompts.MaintenanceDelivery(o.Purpose)
			if err != nil {
				return o, err
			}
			context.Publication, context.PR = r.Frozen.Publication, r.PR
			if o.Purpose == "publish" {
				context.AcceptedHead = r.AcceptedHead()
				if context.AcceptedHead == "" {
					return o, errors.New("缺少已接受成果的提交")
				}
			}
		}
		for i := len(r.Operations) - 1; i >= 0; i-- {
			if run := r.Operations[i].Run; run != nil && run.RunID == r.LatestOwner {
				context.ManagedBranch = run.Execution.Branch
				break
			}
		}
		if strings.TrimSpace(o.Prompt) == "" {
			return o, errors.New("缺少本次执行指令")
		}
		data, err := json.Marshal(executionRequest{ActionID: r.ID, OperationID: id, Instruction: o.Prompt, Context: context})
		if err != nil {
			return o, err
		}
		if local {
			data = []byte(o.Prompt)
		}
		body, err := prompts.MaintenanceExecute(data, local)
		if err != nil {
			return o, err
		}
		input = runnercontract.RunInput{Title: "重构与维护 · " + o.Purpose, NamePrefix: "maintenance", Correlation: correlation, Repository: r.Frozen.Repository, Documents: r.Frozen.Documents, Execution: r.Frozen.Write, Workflow: runnercontract.Workflow{ID: "execute", Version: 1, PromptOverrides: map[string]string{"execute": body}}, ReuseWorktreeFromRunID: r.LatestOwner}
	} else {
		retryJSON := false
		if n := len(r.Operations); n > 0 {
			prior := r.Operations[n-1]
			retryJSON = prior.State == "complete" && prior.Purpose == r.Phase && prior.TargetID == r.TargetID &&
				prior.Run != nil && decisionNeedsJSONRepair(prior.Run.ContentResult)
		}
		body, err := prompts.MaintenanceDecision(r.Phase, r.Input.Strategy, retryJSON, r.FinishRequested || r.Input.Budget.DefaultTarget() && r.TargetID != "")
		if err != nil {
			return o, err
		}
		schema := decision.Schema(r.Phase)
		if r.Phase == "report" {
			schema = decision.ReportSchema()
		}
		if r.Frozen.Direction.Prompt == "" {
			return o, errors.New("缺少冻结的维护方向")
		}
		data, err := json.Marshal(s.context(r, review))
		if err != nil {
			return o, err
		}
		if len(data)+len(body)+len(review.Diff) > 1<<20 {
			return o, errors.New("本阶段必要的维护上下文超过限额，需缩小证据范围")
		}
		contextBody := string(data)
		g := runnercontract.GenerateInput{Instruction: body, OutputSchema: schema, Materials: []runnercontract.Document{
			{Kind: "file", Path: "maintenance/context.json", Content: &contextBody},
		}}
		if r.Phase == "report" {
			g.Materials = append(g.Materials, runnercontract.Document{Kind: "file", Path: "maintenance/pr.diff", Content: &review.Diff})
		}
		if r.Phase == "first" || r.Phase == "report" {
			if directory == "" {
				return o, errors.New("缺少固定只读源码目录")
			}
			g.Directory = &runnercontract.DirectoryAccess{Path: directory, Access: "read_only"}
		}
		o.Prompt = body
		input = runnercontract.GenerateRunInput{Title: "重构与维护 · " + o.Purpose, Correlation: correlation, Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Execution: r.Frozen.Read, Generate: g}
	}
	raw, err := json.Marshal(input)
	o.Request = runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: model.ID(), Input: raw}
	return o, err
}
