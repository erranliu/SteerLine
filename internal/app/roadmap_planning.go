package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/prompts"
	roadmaps "github.com/erranliu/SteerLine/internal/roadmap"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

const planningRoute = "/roadmap-planning"
const planningAction = "roadmap_planning_"
const taskCardOperation = "plan_task"

type planningPort struct {
	*roadmapPort
	store                      planningStore
	readAdmissionMapping       func(context.Context, string, string) (map[string]string, error)
	runner                     planningRunner
	admit                      func(context.Context) error
	refreshBaselineWithRewrite func(context.Context) (commit string, rewritten bool, err error)
}

type planningStore interface {
	PlanningTask(context.Context, string, string) (roadmap.PlanningTask, error)
	PlanningTasks(context.Context, string) ([]roadmap.PlanningTask, error)
	PlanningReceipt(context.Context, string, string) ([]byte, error)
	// A nonempty request commits its receipt and task changes together.
	SavePlanning(context.Context, string, string, []byte, ...roadmap.PlanningTask) error
}

type planningRunner interface {
	BuildPin() runnercontract.BuildPin
	Inspect(context.Context, string, string) (runnercontract.Inspection, error)
	Call(context.Context, string, []string, *runnercontract.Request) (runnerclient.Response, error)
}

type planningView struct {
	ID           string                    `json:"id"`
	Title        string                    `json:"title"`
	Kind         string                    `json:"kind,omitempty"`
	Goal         string                    `json:"goal,omitempty"`
	Card         *roadmap.TaskCard         `json:"card,omitempty"`
	CardVersion  uint64                    `json:"card_version,omitempty"`
	PlanningCard bool                      `json:"planning_card,omitempty"`
	DAGTaskID    string                    `json:"dag_task_id,omitempty"`
	Path         string                    `json:"path"`
	Revision     uint64                    `json:"revision"`
	Status       string                    `json:"status"`
	Diagnostic   string                    `json:"diagnostic,omitempty"`
	Body         string                    `json:"body,omitempty"`
	SourceID     string                    `json:"source_id,omitempty"`
	PR           string                    `json:"pr,omitempty"`
	Busy         bool                      `json:"busy"`
	CanMessage   bool                      `json:"can_message"`
	CanRebase    bool                      `json:"can_rebase"`
	Question     *runnercontract.Question  `json:"question,omitempty"`
	Conflict     *roadmap.PlanningConflict `json:"conflict,omitempty"`
	Review       *roadmap.PlanningReview   `json:"review,omitempty"`
}

func viewPlanning(task roadmap.PlanningTask) planningView {
	view := planningView{ID: task.ID, Title: task.Title, Kind: task.Kind, Path: task.Path, Revision: task.Revision, Status: task.Status, Diagnostic: task.Diagnostic, Body: task.Body, SourceID: task.SourceID, Busy: task.Operation != nil, Conflict: task.Conflict, Review: task.Review}
	if task.Kind == roadmap.SingleTaskPlanning {
		view.Goal = task.Goal
		view.Body = ""
		view.CardVersion = task.CardVersion
		view.PlanningCard = task.Operation != nil && task.Operation.Kind == taskCardOperation
		var card roadmap.TaskCard
		if decodeStrictJSON([]byte(task.Body), &card) == nil {
			view.Card = &card
		}
	}
	if task.Publication != nil {
		view.PR = task.Publication.URL
	}
	if task.Run != nil && !view.Busy && task.Merged == "" && !task.StopRequested {
		view.CanMessage = task.Run.Controls.CanMessage && task.Run.WorkspaceWrite != nil && task.Run.WorkspaceWrite.Owner == "runner"
		view.CanRebase = task.Conflict == nil && task.Run.WorkspaceWrite != nil && task.Run.WorkspaceWrite.CanAcquire
	}
	if task.Run != nil && task.Merged == "" && !task.StopRequested && task.Run.Controls.CanReply &&
		(task.Operation == nil || planningOperationCanAnswer(task.Operation)) {
		view.Question = task.Run.Question
	}
	return view
}

func planningOperationCanAnswer(op *roadmap.PlanningOperation) bool {
	return op.Answer == nil && (op.Kind == "resolve" || op.Kind == taskCardOperation)
}

func (p *planningPort) read(ctx context.Context, route string, values url.Values) (any, error) {
	if route == planningRoute {
		if len(values) != 0 {
			return nil, errors.New("unexpected planning query")
		}
		tasks, err := p.store.PlanningTasks(ctx, p.config.Identity())
		if err != nil {
			return nil, err
		}
		list := []planningView{}
		for _, task := range tasks {
			if task.SourceID == "" {
				view := viewPlanning(task)
				view.Body, view.Goal, view.Card, view.Review, view.Question = "", "", nil, nil, nil
				list = append(list, view)
			}
		}
		agent, err := executionconfig.SteerAgentSettings(p.config.RunnerExecution.SteerAgent)
		if err != nil {
			return nil, err
		}
		var reason string
		if !p.config.runtimeCatalog.SupportsManagedConversation() {
			reason = "固定 Runner 缺少 conversation/2 与 worktree 租约"
		} else if _, err := p.config.runtimeCatalog.ConfigurationCapability("compose"); err != nil {
			reason = err.Error()
		}
		return map[string]any{"tasks": list, "steer_agent": agent, "available": reason == "", "unavailable_reason": reason}, nil
	}
	id, tail, _ := strings.Cut(strings.TrimPrefix(route, planningRoute+"/"), "/")
	if !roadmap.IDPattern.MatchString(id) {
		return nil, roadmap.ErrNotFound
	}
	task, err := p.store.PlanningTask(ctx, p.config.Identity(), id)
	if err != nil {
		return nil, err
	}
	if tail == "history" {
		if len(values) > 1 || len(values) == 1 && (len(values["after"]) != 1 || len(values.Get("after")) > 160) {
			return nil, errors.New("invalid planning history cursor")
		}
		if task.Run == nil {
			return map[string]any{"entries": []any{}}, nil
		}
		args := []string{"--run-id", task.Run.RunID, "--limit", "50"}
		if after := values.Get("after"); after != "" {
			args = append(args, "--after", after)
		}
		res, err := p.runner.Call(ctx, "history", args, nil)
		if err != nil {
			return nil, err
		}
		var page struct {
			RunID   string `json:"run_id"`
			Entries []struct {
				RequestCommandID string `json:"request_command_id"`
				Role             string `json:"role"`
				Text             string `json:"text,omitempty"`
				Result           *struct {
					Text string `json:"text,omitempty"`
				} `json:"result,omitempty"`
			} `json:"entries"`
			Next string `json:"next_cursor"`
			More bool   `json:"has_more"`
		}
		if json.Unmarshal(res.Data, &page) != nil || page.RunID != task.Run.RunID {
			return nil, errors.New("原规划对话身份无法核实")
		}
		task, err = p.store.PlanningTask(ctx, p.config.Identity(), id)
		if err != nil {
			return nil, err
		}
		if task.Run == nil || task.Run.RunID != page.RunID {
			return nil, errors.New("原规划对话身份无法核实")
		}
		cursor := values.Get("after")
		index, cursorErr := strconv.Atoi(strings.TrimPrefix(cursor, page.RunID+":"))
		firstPage := cursor == "" || cursorErr == nil && index == 0
		var pending struct{ Text string }
		if op := task.Operation; op != nil && op.Kind == taskCardOperation && op.Command == "message" && op.Request != nil && op.Result == "" {
			_ = json.Unmarshal(op.Request.Input, &pending)
		}
		for i := range page.Entries {
			entry := &page.Entries[i]
			if entry.Role != "user" {
				continue
			}
			if i == 0 && firstPage {
				entry.Text, entry.Result = task.Goal, nil
			} else if slices.Contains(task.PlanningCommands, entry.RequestCommandID) || pending.Text != "" && entry.Text == pending.Text {
				entry.Role, entry.Text, entry.Result = "planning", "", nil
			}
		}
		return page, nil
	}
	if tail == "predecessors" && len(values) == 0 && task.Kind == roadmap.SingleTaskPlanning {
		graph, err := p.taskGraph.TaskGraphSnapshot(ctx, task.Project)
		if err != nil {
			return nil, err
		}
		options := []roadmap.Existing{}
		for _, node := range graph.Tasks {
			if node.Status != "discarded" {
				options = append(options, roadmap.Existing{ID: node.TaskID, Title: node.Title, Status: node.Status})
			}
		}
		return options, nil
	}
	if tail != "" || len(values) != 0 {
		return nil, roadmap.ErrNotFound
	}
	view := viewPlanning(task)
	if task.Kind == roadmap.SingleTaskPlanning {
		mapping, err := p.readAdmissionMapping(ctx, task.Project, task.ID)
		if err != nil {
			return nil, err
		}
		view.DAGTaskID = mapping["T1"]
		if view.DAGTaskID != "" {
			view.CanMessage, view.CanRebase, view.Question = false, false, nil
		}
	}
	return view, nil
}

func (p *planningPort) receipt(ctx context.Context, id string) (*httpcontract.CommandResponse, error) {
	raw, err := p.store.PlanningReceipt(ctx, p.config.Identity(), id)
	if err != nil || len(raw) == 0 {
		return nil, err
	}
	var out httpcontract.CommandResponse
	err = json.Unmarshal(raw, &out)
	return &out, err
}

func (p *planningPort) command(ctx context.Context, in httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	out := maintenanceResponse(in)
	tasks, info, err := p.prepare(ctx, in)
	if err != nil {
		out.Status, out.Code, out.Diagnostic = "rejected", "planning_rejected", err.Error()
		tasks = nil
	} else {
		out.Status = "applied"
		out.Roadmap, _ = json.Marshal(info)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	durable := context.WithoutCancel(ctx)
	err = p.store.SavePlanning(durable, p.config.Identity(), in.RequestID, raw, tasks...)
	if errors.Is(err, roadmap.ErrPlanningChanged) || errors.Is(err, roadmap.ErrPlanningPathOccupied) {
		out.Status, out.Code, out.Diagnostic, out.Roadmap = "rejected", "planning_rejected", err.Error(), nil
		raw, _ = json.Marshal(out)
		err = p.store.SavePlanning(durable, p.config.Identity(), in.RequestID, raw)
	}
	return out, err
}

func (p *planningPort) prepare(ctx context.Context, in httpcontract.CommandEnvelope) ([]roadmap.PlanningTask, any, error) {
	switch in.Action {
	case planningAction + "create":
		if in.TargetID != in.ProjectID || in.ExpectedVersion != 1 {
			return nil, nil, errors.New("invalid creation target")
		}
		task, err := p.create(ctx, in)
		return []roadmap.PlanningTask{task}, viewPlanning(task), err
	case planningAction + "rebase_all":
		if in.TargetID != in.ProjectID || in.ExpectedVersion != 1 || decodeStrictJSON(in.Payload, &struct{}{}) != nil {
			return nil, nil, errors.New("invalid Rebase batch")
		}
		tasks, err := p.store.PlanningTasks(ctx, p.config.Identity())
		if err != nil {
			return nil, nil, err
		}
		target, err := p.rebaseTarget(ctx)
		if err != nil {
			return nil, nil, err
		}
		var ready []roadmap.PlanningTask
		skipped := map[string]string{}
		for _, task := range tasks {
			if task.SourceID != "" {
				continue
			}
			if !viewPlanning(task).CanRebase {
				skipped[task.ID] = "当前轮次、冲突或其他操作尚未结算"
				continue
			}
			schedulePlanning(&task, "rebase", target)
			ready = append(ready, task)
		}
		return ready, map[string]any{"target": target, "queued": len(ready), "skipped": skipped}, nil
	}
	task, err := p.store.PlanningTask(ctx, p.config.Identity(), in.TargetID)
	if err != nil {
		return nil, nil, err
	}
	answeringOperation := in.Action == planningAction+"answer" && task.Operation != nil && planningOperationCanAnswer(task.Operation)
	if task.Revision != in.ExpectedVersion || task.Operation != nil && !answeringOperation || task.Merged != "" || task.StopRequested || task.Run == nil {
		return nil, nil, errors.New("任务状态已变化或原操作未结算")
	}
	view := viewPlanning(task)
	if task.Kind == roadmap.SingleTaskPlanning {
		mapping, err := p.readAdmissionMapping(ctx, task.Project, task.ID)
		if err != nil {
			return nil, nil, err
		}
		if len(mapping) != 0 {
			return nil, nil, errors.New("任务已正式入图")
		}
	}
	switch in.Action {
	case planningAction + taskCardOperation:
		if task.Kind != roadmap.SingleTaskPlanning || !view.CanMessage || task.Conflict != nil || decodeStrictJSON(in.Payload, &struct{}{}) != nil {
			return nil, nil, errors.New("原会话当前无法规划任务卡")
		}
		if task.Material == "" {
			if err := p.freezePlanningRules(&task); err != nil {
				return nil, nil, err
			}
		}
		instruction, err := prompts.TaskPlanning(task.Material, task.Path, "")
		if err != nil {
			return nil, nil, err
		}
		schedulePlanning(&task, taskCardOperation, "")
		if raw, err := roadmaps.ReadRegular(task.Run.Execution.Workspace, task.Path); err == nil {
			task.Operation.Digest = roadmap.Digest(raw)
		}
		task.Operation.Previous = task.Run.Execution.TurnID
		planControl(task.Operation, "message", task.Run.Revision, map[string]any{"text": instruction})
	case planningAction + "message", planningAction + "resolve":
		var input struct {
			Text string `json:"text"`
		}
		if err := decodeStrictJSON(in.Payload, &input); err != nil {
			return nil, nil, err
		}
		resolve := in.Action == planningAction+"resolve"
		if !view.CanMessage || (task.Conflict != nil) != resolve || len(input.Text) > 65536 {
			return nil, nil, errors.New("原会话当前无法接收此消息")
		}
		if resolve {
			input.Text, err = prompts.RoadmapPlanningConflict(input.Text)
			if err != nil {
				return nil, nil, err
			}
		} else if strings.TrimSpace(input.Text) == "" {
			return nil, nil, errors.New("消息不能为空")
		}
		kind := "message"
		if resolve {
			kind = "resolve"
		}
		schedulePlanning(&task, kind, "")
		task.Operation.Previous = task.Run.Execution.TurnID
		planControl(task.Operation, "message", task.Run.Revision, map[string]any{"text": input.Text})
	case planningAction + "answer":
		var input struct {
			QuestionID string `json:"question_id"`
			Text       string `json:"text"`
		}
		if err := decodeStrictJSON(in.Payload, &input); err != nil {
			return nil, nil, err
		}
		if view.Question == nil || view.Question.ID != input.QuestionID || strings.TrimSpace(input.Text) == "" {
			return nil, nil, errors.New("原问题已变化或答案为空")
		}
		body := map[string]any{"kind": "answer", "question_id": input.QuestionID, "text": input.Text}
		if answeringOperation {
			var answer roadmap.PlanningOperation
			planControl(&answer, "reply", task.Run.Revision, body)
			task.Operation.Answer = answer.Request
			task.Revision++
		} else {
			schedulePlanning(&task, "answer", "")
			planControl(task.Operation, "reply", task.Run.Revision, body)
		}
	case planningAction + "rebase":
		if !view.CanRebase || decodeStrictJSON(in.Payload, &struct{}{}) != nil {
			return nil, nil, errors.New("任务当前无法 Rebase")
		}
		target, err := p.rebaseTarget(ctx)
		if err != nil {
			return nil, nil, err
		}
		schedulePlanning(&task, "rebase", target)
	case planningAction + "abort":
		if task.Conflict == nil || !view.CanMessage || decodeStrictJSON(in.Payload, &struct{}{}) != nil {
			return nil, nil, errors.New("没有可中止的 Rebase")
		}
		schedulePlanning(&task, "abort", task.Conflict.Target)
	case planningAction + "review":
		if task.Kind == roadmap.SingleTaskPlanning || !view.CanRebase || task.Body == "" || decodeStrictJSON(in.Payload, &struct{}{}) != nil {
			return nil, nil, errors.New("当前草稿无法审阅")
		}
		schedulePlanning(&task, "review", "")
	case planningAction + "publish":
		var input struct {
			Digest string   `json:"digest"`
			Paths  []string `json:"paths"`
		}
		if err := decodeStrictJSON(in.Payload, &input); err != nil {
			return nil, nil, err
		}
		if task.Kind == roadmap.SingleTaskPlanning || !view.CanRebase || task.Review == nil || task.Review.Digest != input.Digest || len(input.Paths) == 0 {
			return nil, nil, errors.New("审阅版本已变化")
		}
		schedulePlanning(&task, "publish", "")
		task.Operation.Selected, task.Operation.Digest = input.Paths, input.Digest
	default:
		return nil, nil, errors.New("unknown planning command")
	}
	return []roadmap.PlanningTask{task}, viewPlanning(task), nil
}

func (p *planningPort) rebaseTarget(ctx context.Context) (string, error) {
	target, rewritten, err := p.refreshBaselineWithRewrite(ctx)
	if err != nil {
		return "", err
	}
	if rewritten {
		return "", errors.New("目标分支历史已改写，请先核对项目基线")
	}
	return target, nil
}

func schedulePlanning(task *roadmap.PlanningTask, kind, target string) {
	task.Revision++
	task.Diagnostic = ""
	task.Operation = &roadmap.PlanningOperation{Kind: kind, Stage: "queued", Target: target}
	if kind == "rebase" || kind == "message" || kind == "resolve" {
		task.Review = nil
	}
}

func planControl(op *roadmap.PlanningOperation, command string, revision uint64, body map[string]any) {
	if body == nil {
		body = map[string]any{}
	}
	body["expected_revision"] = revision
	raw, _ := json.Marshal(body)
	op.Command, op.Stage = command, "runner"
	if op.QueuedAt.IsZero() {
		op.QueuedAt = time.Now().UTC()
	}
	op.Request = &runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: roadmaps.Token(), Input: raw}
}

func (p *planningPort) create(ctx context.Context, in httpcontract.CommandEnvelope) (roadmap.PlanningTask, error) {
	var input struct {
		Title   string `json:"title"`
		Goal    string `json:"goal"`
		File    string `json:"file"`
		Kind    string `json:"kind"`
		Discuss bool   `json:"discuss,omitempty"`
	}
	if err := decodeStrictJSON(in.Payload, &input); err != nil {
		return roadmap.PlanningTask{}, err
	}
	if len(input.Title) > 240 || len(input.Goal) > 65536 || strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Goal) == "" {
		return roadmap.PlanningTask{}, errors.New("需要规划名称与目标")
	}
	if input.Kind == "roadmap" {
		input.Kind = ""
	}
	if input.Kind == "" && !validPlanningFileName(input.File) || input.Kind == roadmap.SingleTaskPlanning && input.File != "" || input.Kind != "" && input.Kind != roadmap.SingleTaskPlanning {
		return roadmap.PlanningTask{}, errors.New("规划类型或草稿文件名无效")
	}
	if input.Discuss && input.Kind != roadmap.SingleTaskPlanning {
		return roadmap.PlanningTask{}, errors.New("沟通入口仅用于单任务规划")
	}
	id := in.RequestID
	if !roadmap.IDPattern.MatchString(id) {
		return roadmap.PlanningTask{}, errors.New("invalid planning request ID")
	}
	if err := p.admit(ctx); err != nil {
		return roadmap.PlanningTask{}, err
	}
	if !p.config.runtimeCatalog.SupportsManagedConversation() {
		return roadmap.PlanningTask{}, errors.New("固定 Runner 缺少 conversation/2 与 worktree 租约")
	}
	p.config.runtimeNative.RefreshFailures()
	workflow := runnercontract.Workflow{ID: "conversation", Version: 2}
	execution, err := p.config.resolveSteerAgentExecution(ctx, workflow, "full_access")
	if err != nil {
		return roadmap.PlanningTask{}, err
	}
	baseline, err := p.mergeHistory.refreshBaseline(ctx)
	if err != nil {
		return roadmap.PlanningTask{}, err
	}
	path := roadmap.Root + input.File
	directory := strings.TrimSuffix(roadmap.Root, "/")
	if input.Kind == roadmap.SingleTaskPlanning {
		path = roadmap.TaskCardPath(id)
		directory = "SteerlineFlow/task-planning"
	}
	entries, err := p.config.repositoryGit(ctx, "ls-tree", "-r", "--name-only", "-z", baseline, "--", directory)
	if err != nil {
		return roadmap.PlanningTask{}, err
	}
	for _, entry := range strings.Split(string(entries), "\x00") {
		if strings.EqualFold(entry, path) {
			return roadmap.PlanningTask{}, errors.New("目标分支已有同名草稿")
		}
	}
	remote, err := p.config.repositoryGit(ctx, "config", "--get", "remote."+p.config.Remote+".url")
	if err != nil {
		return roadmap.PlanningTask{}, err
	}
	task := roadmap.PlanningTask{ID: id, Project: p.config.Identity(), Title: input.Title, Goal: input.Goal, Kind: input.Kind,
		Path: path, Base: baseline, Revision: 1, Status: "starting", Build: p.runner.BuildPin()}
	instruction, kind := input.Goal, "submit"
	if !input.Discuss {
		if err := p.freezePlanningRules(&task); err != nil {
			return roadmap.PlanningTask{}, err
		}
		if input.Kind == roadmap.SingleTaskPlanning {
			kind = taskCardOperation
			instruction, err = prompts.TaskPlanning(task.Material, path, input.Goal)
		} else {
			instruction, err = prompts.RoadmapPlanning(task.Material, path, input.Goal)
		}
		if err != nil {
			return roadmap.PlanningTask{}, err
		}
	}
	request := runnercontract.ConversationRunInput{Title: input.Title, Correlation: map[string]string{"steerline_planning": id}, Workflow: workflow, Execution: execution,
		Repository:   runnercontract.Repository{URL: strings.TrimSpace(string(remote)), LocalPath: p.config.Repository, Remote: p.config.Remote, BaseBranch: p.config.Branch, BaseRevision: baseline},
		Conversation: runnercontract.ConversationInput{Instruction: instruction, Materials: []runnercontract.Document{}}}
	if err = p.config.runtimeCatalog.ValidateManagedConversation(request); err != nil {
		return roadmap.PlanningTask{}, err
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return roadmap.PlanningTask{}, err
	}
	submit := runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: roadmaps.Token(), Input: raw}
	task.Submission = submit
	task.Operation = &roadmap.PlanningOperation{Kind: kind, Stage: "runner", Command: "submit", Request: &submit, QueuedAt: time.Now().UTC()}
	return task, nil
}

func (p *planningPort) freezePlanningRules(task *roadmap.PlanningTask) error {
	readRules := prompts.RoadmapPlanningRules
	if task.Kind == roadmap.SingleTaskPlanning {
		readRules = prompts.TaskPlanningRules
	}
	rules, err := readRules()
	if err != nil || len(rules) == 0 {
		return errors.New("规划规则文件缺失")
	}
	material, err := safePlanningPath(p.config.RunDir, "roadmap-planning/"+task.ID+"/rules.md")
	if err != nil {
		return err
	}
	if err := storePlanningMaterial(material, rules); err != nil {
		return err
	}
	task.Material, task.MaterialSHA = material, roadmap.Digest(rules)
	return nil
}

func storePlanningMaterial(filePath string, rules []byte) error {
	dir, name := filepath.Dir(filePath), filepath.Base(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if _, err := safePlanningPath(dir, name); err != nil {
		return err
	}
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		// The material can survive an interrupted first save of the planning record.
		raw, readErr := roadmaps.ReadRegular(dir, name)
		if readErr != nil || !bytes.Equal(raw, rules) {
			return errors.New("固定规划规则文件不可读或已变化")
		}
		return nil
	}
	if err != nil {
		return err
	}
	_, writeErr := file.Write(rules)
	if err = errors.Join(writeErr, file.Close()); err != nil {
		return errors.Join(err, os.Remove(filePath))
	}
	return nil
}

func (p *planningPort) run(ctx context.Context) {
	if queue := p.config.runtimeGenerations; queue != nil {
		if tasks, err := p.store.PlanningTasks(ctx, p.config.Identity()); err == nil {
			for _, task := range tasks {
				if task.ModelSlot {
					queue.UpdateContent(task.Project, "planning:"+task.ID, time.Time{}, true, false)
				}
			}
		}
	}
	var workers sync.WaitGroup
	defer workers.Wait()
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	active := map[string]bool{}
	completed := make(chan string, 8)
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-completed:
			delete(active, id)
		case <-ticker.C:
			tasks, err := p.store.PlanningTasks(ctx, p.config.Identity())
			if err != nil {
				continue
			}
			for _, task := range tasks {
				if len(active) >= cap(completed) {
					break
				}
				if active[task.ID] || (task.Operation == nil && (task.Run == nil || task.Run.StopStatus == "stopped") && (task.Merged == "" || task.SourceID != "")) {
					continue
				}
				active[task.ID] = true
				workers.Go(func() {
					defer func() { completed <- task.ID }()
					p.tick(ctx, task)
				})
			}
		}
	}
}

func (p *planningPort) save(ctx context.Context, task *roadmap.PlanningTask) error {
	return savePlanningTransition(ctx, p.store, task)
}

func savePlanningTransition(ctx context.Context, store planningStore, task *roadmap.PlanningTask) error {
	next := *task
	next.Revision++
	if err := store.SavePlanning(context.WithoutCancel(ctx), task.Project, "", nil, next); err != nil {
		return err
	}
	task.Revision = next.Revision
	return nil
}

func (p *planningPort) tick(ctx context.Context, task roadmap.PlanningTask) {
	if (task.Operation != nil || task.Run != nil && task.Run.StopStatus != "stopped") && !runnerbinding.SameBuild(task.Build, p.runner.BuildPin()) {
		const diagnostic = "原规划会话需要原 Runner 构建继续对账"
		previousSource := task.SourceID
		if task.Merged != "" && task.SourceID == "" {
			_ = p.handoffPlanning(ctx, &task)
		}
		if task.Diagnostic != diagnostic || task.SourceID != previousSource {
			task.Diagnostic = diagnostic
			_ = p.save(ctx, &task)
		}
		return
	}
	before, _ := json.Marshal(task)
	err := p.advance(ctx, &task)
	if err != nil && ctx.Err() == nil {
		task.Diagnostic = err.Error()
	}
	releaseSlot := task.ModelSlot && task.Operation == nil && (task.Run == nil || task.Run.Controls.CanMessage || task.Run.StopStatus == "stopped" ||
		task.Kind == roadmap.SingleTaskPlanning && task.Run.Status == "blocked" && task.Run.Controls.CanResume)
	if releaseSlot {
		task.ModelSlot = false
	}
	after, _ := json.Marshal(task)
	if string(after) != string(before) {
		if p.save(ctx, &task) == nil && (releaseSlot || task.Operation == nil && !task.ModelSlot) {
			if queue := p.config.runtimeGenerations; queue != nil {
				queue.UpdateContent(task.Project, "planning:"+task.ID, time.Time{}, false, true)
			}
		}
	}
}

func verifyPlanningRun(task *roadmap.PlanningTask, run *runnercontract.Run) error {
	if run == nil || run.Correlation["steerline_planning"] != task.ID || run.Workflow.ID != "conversation" || run.Workflow.Version != 2 {
		return errors.New("原规划 Run 绑定无法核实")
	}
	if task.Run != nil {
		original := task.Run
		if original.RunID != run.RunID || (original.InputDigest != "" && original.InputDigest != run.InputDigest) ||
			(original.WorkspaceOriginRunID != "" && original.WorkspaceOriginRunID != run.WorkspaceOriginRunID) ||
			(original.Execution.SessionID != "" && original.Execution.SessionID != run.Execution.SessionID) ||
			(original.WorkspaceIdentity != "" && (original.WorkspaceIdentity != run.WorkspaceIdentity || original.Execution.Workspace != run.Execution.Workspace || original.Execution.Branch != run.Execution.Branch)) {
			return errors.New("原会话或 worktree 身份已变化")
		}
	}
	return nil
}

func (p *planningPort) advance(ctx context.Context, task *roadmap.PlanningTask) error {
	if task.Run != nil && task.Run.StopStatus != "stopped" {
		current, err := p.runner.Inspect(ctx, "run-id", task.Run.RunID)
		if err == nil {
			err = verifyPlanningRun(task, current.Run)
		}
		if err != nil {
			if task.Merged != "" && task.SourceID == "" {
				return errors.Join(err, p.handoffPlanning(ctx, task))
			}
			return err
		}
		task.Run = current.Run
	}
	if task.Publication != nil && task.Publication.URL != "" && task.Merged == "" && !task.StopRequested {
		if err := p.observePlanningPR(ctx, task); err != nil && (task.Operation == nil || task.Operation.Stage == "push_unknown") {
			return err
		}
	}
	if task.Operation != nil {
		if task.Merged != "" && task.Operation.Kind == "close" {
			if err := p.handoffPlanning(ctx, task); err != nil {
				task.Diagnostic = err.Error()
			}
		}
		return p.advancePlanningOperation(ctx, task)
	}
	if task.Merged != "" {
		schedulePlanningClose(task)
		return p.handoffPlanning(ctx, task)
	}
	if task.Kind == roadmap.SingleTaskPlanning && !task.StopRequested {
		mapping, err := p.readAdmissionMapping(ctx, task.Project, task.ID)
		if err != nil {
			return err
		}
		task.StopRequested = len(mapping) != 0
	}
	if task.StopRequested {
		schedulePlanningClose(task)
		return nil
	}
	if task.Run != nil && task.Run.Controls.CanMessage && task.Conflict == nil {
		if task.Kind != roadmap.SingleTaskPlanning {
			if raw, err := roadmaps.ReadRegular(task.Run.Execution.Workspace, task.Path); err == nil {
				task.Body = string(raw)
			}
		}
		task.Status = "ready"
		if task.Publication != nil && task.Publication.URL != "" {
			task.Status = "awaiting_merge"
		}
	} else if task.Run != nil && task.Conflict == nil {
		task.Status = task.Run.Status
	}
	return nil
}

func (p *planningPort) runPlanningControl(ctx context.Context, task *roadmap.PlanningTask) (bool, error) {
	op := task.Operation
	if op.Request == nil {
		return false, errors.New("缺少原 Runner 请求")
	}
	if op.Answer != nil && !task.StopRequested && task.Merged == "" {
		result, err := p.runner.Inspect(ctx, "request-id", op.Answer.RequestID)
		if err != nil {
			var failure *runnerclient.Failure
			if !errors.As(err, &failure) || failure.Code != "request_not_found" {
				return false, err
			}
			_, err = p.runner.Call(ctx, "reply", []string{"--run-id", task.Run.RunID}, op.Answer)
			if errors.As(err, &failure) && !failure.Unknown && failure.Exit == 5 {
				op.Answer = nil
			}
			return false, err
		}
		if err = verifyPlanningRun(task, result.Run); err != nil {
			return false, err
		}
		task.Run = result.Run
		op.Answer = nil
	}
	result, err := p.runner.Inspect(ctx, "request-id", op.Request.RequestID)
	if err != nil {
		var failure *runnerclient.Failure
		if !errors.As(err, &failure) || failure.Code != "request_not_found" {
			return false, err
		}
		if (task.Merged != "" || task.StopRequested) && op.Command != "workspace release" && op.Command != "close" && op.Command != "stop" {
			task.Operation = nil
			if task.Run == nil {
				task.Status = "closed"
			}
			return true, nil
		}
		if op.Kind == taskCardOperation || op.Command == "submit" && task.Kind != roadmap.SingleTaskPlanning {
			_, pathErr := safePlanningPath(filepath.Dir(task.Material), filepath.Base(task.Material))
			raw, err := roadmaps.ReadRegular(filepath.Dir(task.Material), filepath.Base(task.Material))
			if pathErr != nil || err != nil || roadmap.Digest(raw) != task.MaterialSHA {
				if op.Kind == taskCardOperation {
					task.Operation = nil
					if op.Command == "submit" {
						task.Status = "failed"
					}
				}
				return false, errors.New("固定规划规则文件不可读或已变化")
			}
		}
		var args []string
		if op.Command != "submit" {
			if task.Run == nil {
				return false, errors.New("原 Run 不存在")
			}
			args = []string{"--run-id", task.Run.RunID}
		}
		if op.Kind == "resolve" {
			if task.Conflict == nil || task.Merged != "" {
				task.Operation = nil
				return false, errors.New("原 Rebase 已结束")
			}
			branch, target, start, _, readErr := p.rebaseFacts(ctx, task)
			if readErr != nil || branch != "refs/heads/"+task.Run.Execution.Branch || target != task.Conflict.Target || start != task.Conflict.Checkpoint {
				task.Operation = nil
				return false, errors.New("当前 Git 状态与原 Rebase 不符")
			}
		}
		if (op.Command == "submit" || op.Command == "message") && !task.ModelSlot {
			if queue := p.config.runtimeGenerations; queue != nil {
				key := "planning:" + task.ID
				queue.UpdateContent(task.Project, key, op.QueuedAt, false, false)
				if !queue.AcquireContent(task.Project, key) {
					return false, nil
				}
				task.ModelSlot = true
				if err := p.save(ctx, task); err != nil {
					task.ModelSlot = false
					queue.UpdateContent(task.Project, key, time.Time{}, false, true)
					return false, err
				}
			}
		}
		if _, err = p.runner.Call(ctx, op.Command, args, op.Request); err != nil {
			if errors.As(err, &failure) && !failure.Unknown {
				if failure.Code == "stale_revision" && (op.Command == "workspace acquire" || op.Command == "workspace release") {
					return false, p.refreshPlanningLeaseRequest(ctx, task)
				} else if op.Lease == "" && (failure.Exit == 2 || failure.Exit == 3 || failure.Exit == 5 || op.Command == "submit" && failure.Code == "environment_unavailable") {
					task.Operation = nil
					if op.Command == "submit" {
						task.Status = "failed"
					}
				}
			}
			return false, err
		}
		return false, nil
	}
	if err = verifyPlanningRun(task, result.Run); err != nil {
		if result.Run == nil && op.Kind == taskCardOperation && result.Command != nil && result.Command.SubmitRejected() && result.Command.RequestID == op.Request.RequestID {
			task.Operation = nil
			task.Status = "failed"
			return false, result.Command.Error
		}
		return false, err
	}
	if result.Command == nil || result.Command.Command != op.Command {
		return false, errors.New("原 Runner 操作不匹配")
	}
	task.Run = result.Run
	if op.Kind == taskCardOperation {
		if op.Command == "message" && result.Command.CommandID != "" && !slices.Contains(task.PlanningCommands, result.Command.CommandID) {
			task.PlanningCommands = append(task.PlanningCommands, result.Command.CommandID)
			op.Result = result.Command.CommandID
		}
		if task.StopRequested {
			task.Operation = nil
			return true, nil
		}
		return finishTaskCardPlanning(task, result.Command)
	}
	switch op.Command {
	case "workspace acquire":
		if result.Command.ActionStatus != "completed" || task.Run.WorkspaceWrite == nil || task.Run.WorkspaceWrite.Owner != "steerline" || task.Run.WorkspaceWrite.LeaseID != result.Command.CommandID {
			return false, errors.New("当前租约与原获取回执不同")
		}
		op.Lease, op.Stage = result.Command.CommandID, "leased"
	case "workspace release":
		if result.Command.ActionStatus != "completed" || task.Run.WorkspaceWrite == nil || task.Run.WorkspaceWrite.Owner != "runner" {
			return false, errors.New("原租约归还尚未核实")
		}
		if op.Kind == "publish" && op.Result == "prepared" && task.Merged == "" && !task.StopRequested {
			op.Stage = "publish_ready"
		} else {
			task.Operation = nil
		}
		task.Diagnostic = op.Diagnostic
		return true, nil
	case "message":
		if op.Kind == "resolve" && !task.StopRequested && task.Merged == "" {
			if !task.Run.Controls.CanMessage || task.Run.Execution.TurnID == op.Previous {
				if result.Command.ActionStatus == "failed" {
					task.Operation = nil
					return false, errors.New("原冲突处理轮次失败")
				}
				return false, nil
			}
			if err = p.checkPlanningRebase(ctx, task, task.Conflict.Target); err != nil {
				task.Operation = nil
				return false, err
			}
		}
		task.Operation = nil
	case "close", "stop":
		if task.Run.StopStatus != "stopped" {
			return false, nil
		}
		task.Operation = nil
		if task.Merged == "" {
			task.Status = "closed"
		}
	default:
		task.Operation = nil
	}
	task.Diagnostic = ""
	return true, nil
}

func (p *planningPort) refreshPlanningLeaseRequest(ctx context.Context, task *roadmap.PlanningTask) error {
	op := task.Operation
	current, err := p.runner.Inspect(ctx, "run-id", task.Run.RunID)
	if err != nil {
		return err
	}
	if err := verifyPlanningRun(task, current.Run); err != nil {
		return err
	}
	task.Run = current.Run
	write := task.Run.WorkspaceWrite
	if op.Command == "workspace acquire" {
		if write == nil || write.Owner != "runner" || !write.CanAcquire || task.StopRequested || task.Merged != "" {
			task.Operation = nil
			return errors.New("当前任务无法重新取得 worktree 租约")
		}
		planControl(op, op.Command, task.Run.Revision, nil)
		return nil
	}
	if write == nil || write.Owner != "steerline" || write.LeaseID != op.Lease {
		return errors.New("原 worktree 租约尚未核实")
	}
	planControl(op, op.Command, task.Run.Revision, map[string]any{"lease_id": op.Lease})
	return nil
}

func (p *planningPort) advancePlanningOperation(ctx context.Context, task *roadmap.PlanningTask) error {
	op := task.Operation
	switch op.Stage {
	case "runner":
		_, err := p.runPlanningControl(ctx, task)
		return err
	case "queued":
		if task.Run == nil || task.Run.WorkspaceWrite == nil || !task.Run.WorkspaceWrite.CanAcquire || task.Merged != "" || task.StopRequested {
			task.Operation = nil
			return errors.New("当前任务无法取得 worktree 租约")
		}
		planControl(op, "workspace acquire", task.Run.Revision, nil)
	case "leased", "git_running":
		if task.Run == nil || task.Run.WorkspaceWrite == nil || task.Run.WorkspaceWrite.Owner != "steerline" || task.Run.WorkspaceWrite.LeaseID != op.Lease || task.Run.WorkspaceOriginRunID != task.Run.RunID || task.Run.WorkspaceIdentity == "" {
			return errors.New("原 worktree 租约尚未核实")
		}
		return p.applyPlanningLease(ctx, task)
	case "publish_ready", "push_unknown", "pr_ready", "pr_unknown":
		return p.publishPlanning(ctx, task)
	default:
		return fmt.Errorf("未知规划操作阶段 %q", op.Stage)
	}
	return nil
}

func schedulePlanningClose(task *roadmap.PlanningTask) {
	if task.Operation != nil || task.Run == nil || task.Run.StopStatus == "stopped" || task.Run.WorkspaceWrite == nil || task.Run.WorkspaceWrite.Owner != "runner" {
		return
	}
	command := "close"
	if !task.Run.Controls.CanClose {
		if task.Kind != roadmap.SingleTaskPlanning || !task.StopRequested || !task.Run.Controls.CanStop {
			return
		}
		command = "stop"
	}
	task.Operation = &roadmap.PlanningOperation{Kind: "close"}
	planControl(task.Operation, command, task.Run.Revision, nil)
}

// The project observer is drained before this runs. Runner only accepts
// close after the original lease has been returned.
func (p *planningPort) close(ctx context.Context) error {
	tasks, err := p.store.PlanningTasks(ctx, p.config.Identity())
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if (task.Run == nil && task.Operation == nil) || task.Run != nil && task.Run.StopStatus == "stopped" {
			continue
		}
		task.StopRequested = true
		if err = p.save(ctx, &task); err != nil {
			return err
		}
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		tasks, err := p.store.PlanningTasks(ctx, p.config.Identity())
		if err != nil {
			return err
		}
		pending := false
		for _, task := range tasks {
			if task.StopRequested && (task.Run != nil && task.Run.StopStatus != "stopped" || task.Operation != nil) {
				pending = true
				p.tick(ctx, task)
			}
		}
		if !pending {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
