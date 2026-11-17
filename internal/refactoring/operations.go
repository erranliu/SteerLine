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
	Round         int                          `json:"round"`
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

type roundSummary struct {
	ID      string `json:"operation_id"`
	Round   int    `json:"round"`
	Outcome string `json:"outcome"`
	Summary string `json:"summary,omitempty"`
}

type decisionContext struct {
	Mode            string                 `json:"mode"`
	Requirements    string                 `json:"requirements"`
	DirectionPrompt string                 `json:"direction_prompt"`
	Round           int                    `json:"round,omitempty"`
	RoundStart      string                 `json:"round_start,omitempty"`
	RoundGoal       string                 `json:"round_goal,omitempty"`
	PR              *runnercontract.PRFact `json:"pr,omitempty"`
	Checks          string                 `json:"checks,omitempty"`
	Feedback        []contextOperation     `json:"feedback,omitempty"`
	History         []roundSummary         `json:"history,omitempty"`
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
		Mode: r.Phase, Requirements: r.Input.Requirements, DirectionPrompt: r.Frozen.Direction.Prompt,
	}
	if r.Phase == "report" {
		f.PR, f.Checks = r.PR, review.Checks
		return f
	}
	f.Round = r.Round
	if r.Phase == "audit" {
		f.RoundStart = roundStart(r)
	}
	if n := len(r.Answers); n > 0 {
		f.Answers = r.Answers[n-1:]
		for i := len(r.Operations) - 1; i >= 0; i-- {
			if q := r.Operations[i].Question; q != nil && q.ID == f.Answers[0].QuestionID {
				f.Question = q
				break
			}
		}

	}
	executions := 0
	for i := len(r.Operations) - 1; i >= 0; i-- {
		o := r.Operations[i]
		if !o.Settled() {
			continue
		}
		if r.Phase == "audit" && o.Round == r.Round && o.Purpose == "implement" && f.RoundGoal == "" {
			f.RoundGoal = clippedSummary(o.Prompt, 16<<10)
		}
		if o.Content(r).WorkflowID == "execute" && o.Run != nil && executions < 2 {
			item := contextOperation{ID: o.ID, Round: o.Round, Purpose: o.Purpose, Prompt: o.Prompt, Result: o.Result, Question: o.Run.Question, WorkspaceGit: o.WorkspaceGit, Status: o.Run.Status, Diagnostic: clippedSummary(o.Run.Diagnostic, 2048)}
			item.Committed = executionCommitted(o)
			if o.Run.Final != nil {
				item.NativeOutcome = o.Run.Final.NativeOutcome
			}
			if o.Result == nil {
				item.Feedback = clippedSummary(o.Feedback, 16<<10)
			}
			f.Feedback = append(f.Feedback, item)
			executions++
		}
	}
	for i := len(r.Operations) - 1; i >= 0 && len(f.History) < 24; i-- {
		o := r.Operations[i]
		if o.Settled() && o.AcceptedOperation != "" {
			f.History = append(f.History, roundSummary{ID: o.ID, Round: o.Round, Outcome: o.Purpose, Summary: clippedSummary(o.DecisionSummary, 512)})
		}
	}
	return f
}

type executionContext struct {
	Publication   map[string]string      `json:"publication,omitempty"`
	ManagedBranch string                 `json:"managed_branch,omitempty"`
	PR            *runnercontract.PRFact `json:"pr,omitempty"`
	RoundStart    string                 `json:"round_start,omitempty"`
	AcceptedHead  string                 `json:"accepted_head,omitempty"`
}

type executionRequest struct {
	ActionID    string           `json:"action_id"`
	OperationID string           `json:"operation_id"`
	Instruction string           `json:"instruction"`
	Context     executionContext `json:"context"`
}

func committedHead(o model.Operation) string {
	if git := o.WorkspaceGit; git != nil && git.Verified && git.Clean != nil && *git.Clean && git.Commit != nil && model.ValidRevision(*git.Commit) {
		return *git.Commit
	}
	return ""
}

func acceptedHead(r model.Record, round int) string {
	var accepted string
	for i := len(r.Operations) - 1; i >= 0; i-- {
		o := r.Operations[i]
		if round > 0 && o.Round != round {
			continue
		}
		if accepted == "" {
			accepted = o.AcceptedOperation
		} else if o.ID == accepted {
			return committedHead(o)
		}
	}
	return ""
}

func roundStart(r model.Record) string {
	if r.Round == 1 {
		return r.Frozen.Repository.BaseRevision
	}
	if r.Round > 1 {
		return acceptedHead(r, r.Round-1)
	}
	return ""
}

func (s *Service) prepareOperation(r model.Record, id, directory string, review model.ReviewEvidence) (model.Operation, error) {
	now := s.now()
	o := model.Operation{ID: id, Purpose: r.Phase, Round: r.Round, State: "pending", CreatedAt: now, Investigation: directory, Closing: r.FinishRequested}
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
				context.AcceptedHead = acceptedHead(r, 0)
				if context.AcceptedHead == "" {
					return o, errors.New("缺少已接受成果的提交")
				}
			}
		}
		if local {
			context.RoundStart = roundStart(r)
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
		body, err := prompts.MaintenanceExecute(data, local)
		if err != nil {
			return o, err
		}
		input = runnercontract.RunInput{Title: "重构与维护 · " + o.Purpose, NamePrefix: "maintenance", Correlation: correlation, Repository: r.Frozen.Repository, Documents: r.Frozen.Documents, Execution: r.Frozen.Write, Workflow: runnercontract.Workflow{ID: "execute", Version: 1, PromptOverrides: map[string]string{"execute": body}}, ReuseWorktreeFromRunID: r.LatestOwner}
	} else {
		retryJSON := false
		if n := len(r.Operations); n > 0 {
			prior := r.Operations[n-1]
			retryJSON = prior.State == "complete" && prior.Purpose == r.Phase && prior.Round == r.Round &&
				prior.Run != nil && decisionNeedsJSONRepair(prior.Run.ContentResult)
		}
		body, err := prompts.MaintenanceDecision(r.Phase, r.Input.Strategy, retryJSON)
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
		if r.Phase == "first" {
			if directory == "" {
				return o, errors.New("缺少首轮调查目录")
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
