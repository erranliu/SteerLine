package refactoring

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
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
	ActionID         string                    `json:"action_id"`
	Mode             string                    `json:"mode"`
	Strategy         string                    `json:"strategy"`
	Requirements     string                    `json:"requirements"`
	DirectionPrompt  string                    `json:"direction_prompt"`
	Repository       runnercontract.Repository `json:"repository"`
	Round            int                       `json:"round"`
	RoundStart       string                    `json:"round_start,omitempty"`
	Budget           model.Budget              `json:"budget"`
	Finish           bool                      `json:"finish"`
	ContinueConflict bool                      `json:"continue_conflict,omitempty"`
	CanStartRound    bool                      `json:"can_start_round"`
	Amount           *string                   `json:"amount,omitempty"`
	CostComplete     bool                      `json:"cost_complete"`
	AsOf             time.Time                 `json:"as_of"`
	ElapsedSeconds   float64                   `json:"elapsed_seconds"`
	Diagnostic       string                    `json:"diagnostic,omitempty"`
	PR               *runnercontract.PRFact    `json:"pr,omitempty"`
	Feedback         []contextOperation        `json:"feedback,omitempty"`
	History          []roundSummary            `json:"history,omitempty"`
	Answers          []model.Answer            `json:"answers,omitempty"`
	Question         *model.Question           `json:"answered_question,omitempty"`
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

func (s *Service) context(r model.Record, now time.Time) decisionContext {
	amount, complete := r.CostTotal()
	f := decisionContext{
		ActionID: r.ID, Mode: r.Phase, Strategy: r.Input.Strategy, Requirements: r.Input.Requirements,
		DirectionPrompt: r.Frozen.Direction.Prompt, Repository: r.Frozen.Repository, Round: r.Round, Budget: r.Input.Budget,
		Finish: r.FinishRequested, CanStartRound: (r.Phase == "first" || r.Phase == "select") && !r.FinishRequested && !r.BudgetExhausted(now) && (r.Input.Budget.Dollars == "" || complete),
		ContinueConflict: r.Phase == "finish" && r.WriteKind == "conflict",
		Amount:           amount, CostComplete: complete, AsOf: now, ElapsedSeconds: r.Elapsed(now).Seconds(), Diagnostic: r.Diagnostic,
		PR: r.PR,
	}
	f.Repository.LocalPath = ""
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
		if o.Content(r).WorkflowID == "execute" && o.Run != nil && executions < 2 {
			item := contextOperation{ID: o.ID, Round: o.Round, Purpose: o.Purpose, Prompt: o.Prompt, Result: o.Result, Question: o.Run.Question, WorkspaceGit: o.WorkspaceGit, Status: o.Run.Status, Diagnostic: clippedSummary(o.Run.Diagnostic, 2048)}
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

func roundStart(r model.Record) string {
	if r.Round == 1 {
		return r.Frozen.Repository.BaseRevision
	}
	var accepted string
	for i := len(r.Operations) - 1; i >= 0; i-- {
		o := r.Operations[i]
		if o.Round != r.Round-1 {
			continue
		}
		if accepted == "" {
			accepted = o.AcceptedOperation
		} else if o.ID == accepted && o.WorkspaceGit != nil && o.WorkspaceGit.Verified &&
			o.WorkspaceGit.Clean != nil && *o.WorkspaceGit.Clean &&
			o.WorkspaceGit.Commit != nil && model.ValidRevision(*o.WorkspaceGit.Commit) {
			return *o.WorkspaceGit.Commit
		}
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
		context := struct {
			Publication   map[string]string      `json:"publication"`
			ManagedBranch string                 `json:"managed_branch,omitempty"`
			PR            *runnercontract.PRFact `json:"pr,omitempty"`
			RoundStart    string                 `json:"round_start,omitempty"`
		}{Publication: r.Frozen.Publication, PR: r.PR}
		if o.Purpose == "repair" {
			context.RoundStart = roundStart(r)
		}
		for i := len(r.Operations) - 1; i >= 0; i-- {
			if run := r.Operations[i].Run; run != nil && run.RunID == r.LatestOwner {
				context.ManagedBranch = run.Execution.Branch
				break
			}
		}
		if strings.TrimSpace(o.Prompt) == "" {
			return o, errors.New("缺少本次三级指令")
		}
		data, err := json.Marshal(map[string]any{"action_id": r.ID, "operation_id": id, "instruction": o.Prompt, "context": context})
		if err != nil {
			return o, err
		}
		body, err := prompts.MaintenanceExecute(data)
		if err != nil {
			return o, err
		}
		input = runnercontract.RunInput{Title: "重构与维护 · " + o.Purpose, NamePrefix: "maintenance", Correlation: correlation, Repository: r.Frozen.Repository, Documents: r.Frozen.Documents, Execution: r.Frozen.Write, Workflow: runnercontract.Workflow{ID: "execute", Version: 1, PromptOverrides: map[string]string{"execute": body}}, ReuseWorktreeFromRunID: r.LatestOwner}
	} else {
		body, err := decision.Instruction()
		if err != nil {
			return o, err
		}
		rules := r.Frozen.FollowRules
		schema := decision.Schema(r.Phase)
		switch r.Phase {
		case "first":
			rules = r.Frozen.FirstRules
		case "report":
			rules, schema = r.Frozen.ReportRules, decision.ReportSchema()
		}
		if rules == "" || r.Phase != "report" && r.Frozen.Strategy == "" || r.Frozen.Direction.Prompt == "" {
			return o, errors.New("缺少冻结维护提示词材料")
		}
		var facts any
		if r.Phase == "report" {
			facts = struct {
				Mode            string                 `json:"mode"`
				PR              *runnercontract.PRFact `json:"pr"`
				DirectionPrompt string                 `json:"direction_prompt"`
				Requirements    string                 `json:"requirements"`
				Checks          string                 `json:"checks"`
			}{"report", r.PR, r.Frozen.Direction.Prompt, r.Input.Requirements, review.Checks}
		} else {
			facts = s.context(r, now)
		}
		data, err := json.Marshal(facts)
		if err != nil {
			return o, err
		}
		if len(data)+len(rules)+len(r.Frozen.Strategy)+len(review.Diff) > 1<<20 {
			return o, errors.New("本阶段必要的维护上下文超过限额，需缩小证据范围")
		}
		contextBody := string(data)
		g := runnercontract.GenerateInput{Instruction: body, OutputSchema: schema, Materials: []runnercontract.Document{
			{Kind: "file", Path: "maintenance/rules.md", Content: &rules},
			{Kind: "file", Path: "maintenance/context.json", Content: &contextBody},
		}}
		if r.Phase != "report" {
			g.Materials = append(g.Materials, runnercontract.Document{Kind: "file", Path: "maintenance/strategy.md", Content: &r.Frozen.Strategy})
		} else {
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
