package refactoring

import (
	"encoding/base64"
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
	FinalEvidence    *model.Result             `json:"final_evidence,omitempty"`
	EvidenceNeed     string                    `json:"evidence_need,omitempty"`
	Feedback         []contextOperation        `json:"feedback,omitempty"`
	History          []roundSummary            `json:"history,omitempty"`
	HistoryTotals    map[string]int            `json:"history_totals,omitempty"`
	PastAnswers      []model.Answer            `json:"past_answers,omitempty"`
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
		PR: r.PR, FinalEvidence: r.Candidate, EvidenceNeed: r.EvidenceNeed,
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
		if r.Phase == "report" {
			for i := max(0, n-33); i < n-1; i++ {
				answer := r.Answers[i]
				answer.Option.Label = clippedSummary(answer.Option.Label, 256)
				answer.Option.Impact = clippedSummary(answer.Option.Impact, 256)
				answer.Note = clippedSummary(answer.Note, 256)
				f.PastAnswers = append(f.PastAnswers, answer)
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
	historyLimit := 24
	if r.Phase == "report" {
		historyLimit = 256
		f.HistoryTotals = map[string]int{"人工答复": len(r.Answers)}
	}
	for i := len(r.Operations) - 1; i >= 0; i-- {
		o := r.Operations[i]
		if !o.Settled() {
			continue
		}
		if r.Phase == "report" && o.Purpose == "implement" {
			status := o.State
			if successfulExecution(o) {
				status = o.Result.Outcome
			} else if o.Run != nil {
				status = o.Run.Status
			}
			f.HistoryTotals[status]++
			if len(f.History) < historyLimit {
				f.History = append(f.History, roundSummary{ID: o.ID, Round: o.Round, Outcome: status})
			}
		}
		if o.DecisionSummary != "" && (o.AcceptedOperation != "" || r.Phase == "report" && (o.Question != nil || o.Purpose == "first" || o.Purpose == "finish" || o.Purpose == "version_audit" || o.Purpose == "audit")) {
			if len(f.History) >= historyLimit {
				continue
			}
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

func (s *Service) prepareOperation(r model.Record, id, directory string) (model.Operation, error) {
	now := s.now()
	o := model.Operation{ID: id, Purpose: r.Phase, Round: r.Round, State: "pending", CreatedAt: now, Investigation: directory, Closing: r.FinishRequested}
	if r.Phase == "version_audit" {
		o.PRVersion = prVersion(r.PR)
	}
	correlation := map[string]string{"steerline_project": r.Project, "steerline_refactoring": r.ID, "refactoring_operation": id}
	var input any
	if r.Phase == "execute" {
		o.Purpose, o.Prompt = r.WriteKind, r.PendingPrompt
		body, err := prompts.MaintenanceExecute()
		if err != nil {
			return o, err
		}
		context := struct {
			Publication   map[string]string      `json:"publication"`
			ManagedBranch string                 `json:"managed_branch,omitempty"`
			PR            *runnercontract.PRFact `json:"pr,omitempty"`
			Report        string                 `json:"report,omitempty"`
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
		if o.Purpose == "sync" {
			report := pendingReport(r)
			if report == nil {
				return o, errors.New("缺少待同步的报告版本")
			}
			context.Report = report.Body
		}
		if strings.TrimSpace(o.Prompt) == "" {
			return o, errors.New("缺少本次三级指令")
		}
		data, err := json.Marshal(map[string]any{"action_id": r.ID, "operation_id": id, "instruction": o.Prompt, "context": context})
		if err != nil {
			return o, err
		}
		if len(data) > runnercontract.MaxProgressBytes {
			return o, errors.New("维护执行材料超过Runner结果文件限额")
		}
		input = runnercontract.RunInput{Title: "重构与维护 · " + o.Purpose, NamePrefix: "maintenance", Correlation: correlation, Repository: r.Frozen.Repository, Documents: r.Frozen.Documents, Execution: r.Frozen.Write, Workflow: runnercontract.Workflow{ID: "execute", Version: 1, PromptOverrides: map[string]string{"execute": body}}, ReuseWorktreeFromRunID: r.LatestOwner, ResultFile: &runnercontract.ResultFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString(data), RequireInitialization: true}}
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
		f := s.context(r, now)
		data, err := json.Marshal(f)
		if err != nil {
			return o, err
		}
		if len(data)+len(rules)+len(r.Frozen.Strategy) > 1<<20 {
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
			o.ReportCost, o.ReportAsOf = f.Amount, f.AsOf
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
