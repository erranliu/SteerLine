// Package query owns product reads and pure page projections. Page projection
// never reconstructs the execution actor or grants business write authority.
package query

import (
	"encoding/json"
	"strings"
	"time"

	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	model "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	taskview "github.com/erranliu/SteerLine/internal/product/query/taskgraph"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TaskSummary(t taskmodel.TaskRecord) api.TaskSummary {
	return api.TaskSummary{TaskID: t.TaskID, Title: t.Title, Type: t.Type, Status: t.Status, Version: t.Version, Claimant: t.Claimant}
}

func TaskDetail(t storageports.NativeTaskDetail) api.TaskDetail {
	p := api.TaskDetail{TaskSummary: TaskSummary(t.TaskRecord), StopPending: append([]string(nil), t.StopPending...), ProjectID: t.ProjectID, Priority: t.Priority, SpecPath: t.Contract.SpecRef.Path, Successors: append([]string(nil), t.Successors...), DependencyReady: t.DependencyReady, StartupBlockers: append([]string(nil), t.StartupBlockers...)}
	for _, w := range t.WorkflowRuns {
		p.WorkflowRuns = append(p.WorkflowRuns, workflowSummary(w))
	}
	for _, d := range t.Predecessors {
		p.Predecessors = append(p.Predecessors, dependencySummary(d))
	}
	for _, d := range t.UnmetPredecessors {
		p.UnmetPredecessors = append(p.UnmetPredecessors, dependencySummary(d))
	}
	if d := t.Delivery; d != nil {
		p.Delivery = &api.DeliverySummary{PRURL: d.PRURL, WorkflowRunID: d.SelectedWorkflowRunID, MergedAt: d.MergedAt}
	}
	return p
}

func dependencySummary(d taskmodel.TaskDependencyStatus) api.DependencySummary {
	return api.DependencySummary{TaskID: d.TaskID, Status: d.Status, Delivered: d.Delivered, Blocker: d.Blocker}
}

func TaskPage(p storageports.NativeTaskPage) api.TaskPage {
	view := api.TaskPage{Protocol: p.Protocol, ProjectID: p.ProjectID, AuthorityVersion: p.AuthorityVersion, GraphRevision: p.GraphRevision, DatabaseRevision: p.DatabaseRevision, DatabaseID: p.DatabaseID, SampledAt: p.SampledAt, Capabilities: api.TaskCapabilities{DependencyReadiness: p.Capabilities.DependencyReadiness, DependencyBlockers: p.Capabilities.DependencyBlockers, StartupBlockers: p.Capabilities.StartupBlockers}, NextCursor: p.NextCursor, HasMore: p.HasMore, Tasks: []api.TaskDetail{}}
	for _, t := range p.Tasks {
		view.Tasks = append(view.Tasks, TaskDetail(t))
	}
	return view
}

func TaskDetailByID(p storageports.NativeTaskPage, id string) (*api.TaskDetail, error) {
	for _, item := range p.Tasks {
		if item.TaskID == id {
			v := TaskDetail(item)
			return &v, nil
		}
	}
	return nil, nil
}

func TaskGraphPage(p taskview.Page) api.GraphPage {
	v := api.GraphPage{Protocol: p.Protocol, ProjectID: p.ProjectID, AuthorityVersion: p.AuthorityVersion, GraphRevision: p.GraphRevision, DatabaseRevision: p.DatabaseRevision, DatabaseID: p.DatabaseID, SampledAt: p.SampledAt, NextCursor: p.NextCursor, HasMore: p.HasMore, Boundary: api.GraphBoundary{Truncated: p.Boundary.Truncated, MaxNodes: p.Boundary.MaxNodes, OmittedNodeCount: p.Boundary.OmittedNodeCount, OmittedBoundaryEdgeCount: p.Boundary.OmittedBoundaryEdgeCount, Reasons: append([]string(nil), p.Boundary.Reasons...)}, Nodes: []api.GraphNode{}, Edges: []api.GraphEdge{}}
	for _, n := range p.Nodes {
		node := api.GraphNode{TaskID: n.TaskID, Title: n.Title, Status: n.Status, Priority: n.Priority, DependencyReady: n.DependencyReady, StartupBlockers: append([]string(nil), n.StartupBlockers...)}
		for _, b := range n.Blockers {
			node.Blockers = append(node.Blockers, api.DependencySummary{TaskID: b.TaskID, Status: b.Status, Delivered: b.Delivered, Blocker: b.Blocker})
		}
		if d := n.Delivery; d != nil {
			node.Delivery = &api.DeliverySummary{PRURL: d.PRURL, WorkflowRunID: d.SelectedWorkflowRunID, MergedAt: d.MergedAt}
		}
		v.Nodes = append(v.Nodes, node)
	}
	for _, e := range p.Edges {
		v.Edges = append(v.Edges, api.GraphEdge{Predecessor: e.Predecessor, Successor: e.Successor})
	}
	for _, e := range p.Boundary.Edges {
		v.Boundary.Edges = append(v.Boundary.Edges, api.GraphBoundaryEdge{Predecessor: e.Predecessor, Successor: e.Successor, Reason: e.Reason})
	}
	return v
}

func DecisionRequest(r model.DecisionRequest) api.DecisionRequest {
	p := api.DecisionRequest{Question: r.Question, Context: r.Context, Recommended: r.Recommended, AllowFreeText: r.AllowFreeText, HasNativeQuestions: len(r.Questions) > 0 || r.QuestionRoute != nil}
	for _, o := range r.Options {
		p.Options = append(p.Options, api.DecisionOption{ID: o.ID, Label: o.Label, Detail: o.Detail})
	}
	return p
}

// CandidateView is the bounded product projection input prepared by the
// application edge. The query package never receives or reconstructs Runtime.
type CandidateView struct {
	FlowRef, StepID, BusinessStatus, StageDiagnostic, ReportToolPolicy string
	StageResults                                                       []model.StageResult

	SessionID, SessionGeneration, CurrentTurnID    string
	TurnNumber                                     uint64
	PRHistory                                      []string
	CandidateCreatedAt                             time.Time
	CandidateEndedAt                               *time.Time
	CandidateTiming, ResourceStatus                string
	RecoveryRequired                               bool
	RecoveryState                                  string
	RecoveryVersion                                uint64
	TaskID, DomainTaskID, WorkflowVersion          string
	ExecutionID, Title, RunID, AttemptID           string
	ControlRevision                                uint64
	Claimant, Host, Backend, Protocol, Agent       string
	Target                                         backend.ExecutionTarget
	TaskStatus, ExecutionStatus, FinishTaskStatus  string
	StartedAt, EndedAt                             *time.Time
	WorktreePath, WorktreeBranch, BaseCommit       *string
	LastEvent, LogPath, ResultPath, Summary, Error *string
	Process                                        *backend.ProcessIdentity
	Abandonment                                    *model.CandidateAbandonment
	EnvironmentBlocker                             *model.WorkerHostBlocker
	ExecutionProfile                               *model.FixedExecutionProfile
	Decision                                       *model.DecisionState
	PR                                             model.PRState
}

// ServiceView contains only service fields rendered by the product API plus
// candidate values projected separately from the execution actor state.
type ServiceView struct {
	Revision               uint64
	UpdatedAt, HeartbeatAt time.Time
	CurrentTask            *string
	SlotOccupied           bool
	Repository             api.RepositorySummary
	Service                api.ServiceSummary
	Control                api.ControlSummary
	Runs                   []*CandidateView
	Deliveries             []model.TaskDeliveryRecord
}

func Candidate(r *CandidateView) *api.CandidateSummary {
	if r == nil {
		return nil
	}
	p := &api.CandidateSummary{FlowRef: r.FlowRef, StepID: r.StepID, BusinessStatus: r.BusinessStatus, StageDiagnostic: r.StageDiagnostic, ReportToolPolicy: r.ReportToolPolicy, SessionID: r.SessionID, SessionGeneration: r.SessionGeneration, CurrentTurnID: r.CurrentTurnID, TurnNumber: r.TurnNumber, PRHistory: append([]string(nil), r.PRHistory...), CandidateCreatedAt: r.CandidateCreatedAt, CandidateEndedAt: copyValue(r.CandidateEndedAt), CandidateTiming: r.CandidateTiming, ResourceStatus: r.ResourceStatus, RecoveryRequired: r.RecoveryRequired, RecoveryState: r.RecoveryState, RecoveryVersion: r.RecoveryVersion, TaskID: r.TaskID, DomainTaskID: r.DomainTaskID, WorkflowVersion: r.WorkflowVersion, ExecutionID: r.ExecutionID, Title: r.Title, RunID: r.RunID, AttemptID: r.AttemptID, ControlRevision: r.ControlRevision, Claimant: r.Claimant, Host: r.Host, Backend: r.Backend, Protocol: r.Protocol, Agent: r.Agent, Target: api.TargetSummary{ID: r.Target.ID, WorktreeID: r.Target.WorktreeID, Branch: r.Target.Branch}, TaskStatus: r.TaskStatus, ExecutionStatus: r.ExecutionStatus, FinishTaskStatus: r.FinishTaskStatus, StartedAt: copyValue(r.StartedAt), EndedAt: copyValue(r.EndedAt), WorktreePath: copyValue(r.WorktreePath), WorktreeBranch: copyValue(r.WorktreeBranch), BaseCommit: copyValue(r.BaseCommit), LastEvent: copyValue(r.LastEvent), LogPath: copyValue(r.LogPath), ResultPath: copyValue(r.ResultPath), Summary: copyValue(r.Summary), Error: copyValue(r.Error), PR: api.PRSummary{URL: copyValue(r.PR.URL), HeadRevision: copyValue(r.PR.HeadRevision), MergeCommit: copyValue(r.PR.MergeCommit), QueryError: copyValue(r.PR.QueryError), State: r.PR.State, MergedAt: copyValue(r.PR.MergedAt), QueriedAt: copyValue(r.PR.QueriedAt)}}
	for _, result := range r.StageResults {
		p.StageResults = append(p.StageResults, api.StageResultSummary{StepID: result.StepID, CommandID: result.CommandID, TurnID: result.TurnID, InputVersion: result.InputVersion, Succeeded: result.Succeeded, Final: result.Final, FinalSource: result.FinalSource, Failure: result.Failure, Outcome: result.Outcome})
	}
	if r.Process != nil {
		p.Process = &api.WorkerLocation{Terminal: r.Process.Terminal}
	}
	if r.Abandonment != nil {
		p.Abandonment = &api.AbandonmentSummary{Reason: r.Abandonment.Reason, RequestedAt: r.Abandonment.At}
	}
	if r.EnvironmentBlocker != nil {
		p.EnvironmentBlocker = environmentBlocker(r.EnvironmentBlocker)
	}
	if r.ExecutionProfile != nil {
		v := Profile(*r.ExecutionProfile)
		p.ExecutionProfile = &v
	}
	if d := r.Decision; d != nil {
		p.Decision = &api.DecisionSummary{ID: d.ID, AttemptID: d.AttemptID, Status: d.Status, Version: d.Version, Request: DecisionRequest(d.Request), OptionID: d.OptionID, Note: d.Note, AnsweredBy: d.AnsweredBy, AnsweredAt: copyValue(d.AnsweredAt), AppliedAt: copyValue(d.AppliedAt)}
	}
	return p
}

func environmentBlocker(b *model.WorkerHostBlocker) *api.EnvironmentBlocker {
	if b == nil {
		return nil
	}
	return &api.EnvironmentBlocker{ID: b.ID, TurnID: b.TurnID, Version: b.Version, Fact: api.EnvironmentFact{Reason: b.Fact.Reason, Detail: b.Fact.Detail, Confidence: b.Fact.Confidence, Source: b.Fact.Source}}
}

func ServicePage(rt *ServiceView) *api.ServicePage {
	if rt == nil {
		return nil
	}
	p := &api.ServicePage{Revision: rt.Revision, UpdatedAt: rt.UpdatedAt, HeartbeatAt: rt.HeartbeatAt, CurrentTask: copyValue(rt.CurrentTask), SlotOccupied: rt.SlotOccupied, Repository: rt.Repository, Service: rt.Service, Control: rt.Control}
	p.Service.Capabilities = append([]string(nil), rt.Service.Capabilities...)
	p.Service.Error = copyValue(rt.Service.Error)
	p.Service.StartupID = copyValue(rt.Service.StartupID)
	if o := rt.Control.Operation; o != nil {
		operation := *o
		p.Control.Operation = &operation
	}
	p.Control.Blockers = append([]api.ControlBlocker(nil), rt.Control.Blockers...)
	for _, r := range rt.Runs {
		candidate := Candidate(r)
		if candidate != nil {
			for _, delivery := range rt.Deliveries {
				if delivery.TaskID == candidate.TaskID {
					candidate.DeliveryOutcome = "loser"
					if delivery.SelectedWorkflowRunID == candidate.RunID {
						candidate.DeliveryOutcome = "winner"
					}
				}
			}
		}
		p.Runs = append(p.Runs, candidate)
	}
	return p
}

func workflowSummary(w model.WorkflowRunRecord) api.WorkflowSummary {
	p := api.WorkflowSummary{FlowRef: w.FlowRef, StepID: w.StepID, Status: w.Status, WorkflowRunID: w.WorkflowRunID, TaskID: w.TaskID, CreatedAt: w.CreatedAt}
	if w.ExecutionProfile != nil {
		v := Profile(*w.ExecutionProfile)
		p.ExecutionProfile = &v
	}
	return p
}

func History(d storageports.ExecutionSnapshot) api.HistorySummary {
	p := api.HistorySummary{}
	for _, w := range d.WorkflowRuns {
		p.WorkflowRuns = append(p.WorkflowRuns, workflowSummary(w))
	}
	for _, a := range d.Attempts {
		p.Attempts = append(p.Attempts, api.AttemptSummary{ExecutionID: a.ExecutionID, AttemptID: a.AttemptID, AttemptNumber: a.AttemptNumber, CreatedAt: a.CreatedAt})
	}
	for _, e := range d.Executions {
		p.Executions = append(p.Executions, api.ExecutionSummary{ExecutionID: e.ExecutionID, VerifiedAt: e.Workspace.VerifiedAt, Details: executionDetails(e)})
	}
	for _, d := range d.Decisions {
		v := api.DecisionHistory{WorkflowRunID: d.WorkflowRunID, DecisionID: d.DecisionID, ExecutionID: d.ExecutionID, AttemptID: d.AttemptID, Status: d.Status, Version: d.Version, CreatedAt: d.CreatedAt}
		var r model.DecisionRequest
		if err := json.Unmarshal(d.Request, &r); err != nil {
			v.Diagnostic = "问题记录格式未知；原记录保留，未猜测或导入。"
		} else {
			request := DecisionRequest(r)
			v.Request = &request
		}
		if len(d.Answer) > 0 {
			var a api.DecisionAnswer
			if err := json.Unmarshal(d.Answer, &a); err == nil {
				v.Answer = &a
			} else {
				v.AnswerDiagnostic = "原答案记录格式未知；原记录保留，未猜测或重放。"
			}
		}
		p.Decisions = append(p.Decisions, v)
	}
	return p
}

func Profile(f model.FixedExecutionProfile) api.ExecutionProfileSummary {
	p := api.ExecutionProfileSummary{ID: f.ID, Name: f.Name, Host: f.Host, Agent: f.Agent, Digest: f.Digest, AcceptedConfigVersion: f.AcceptedConfigVersion, ImplementationDisplay: append([]string(nil), f.ImplementationDisplay...)}
	p.Details = []string{"发布责任：" + f.PublicationPolicy.Publisher, "冻结发布目标：" + f.Publication.Provider.Kind + " · " + f.Publication.Provider.BaseURL + "/" + f.Publication.Provider.Repository + " · base " + f.Publication.BaseBranch, "Git remote：" + f.Publication.Remote + " · " + f.Publication.RemoteURL, "发布工具：" + f.Publication.Git.Executable + " · query " + f.Publication.Provider.Query.Executable, "认证引用：" + strings.Join(f.Publication.AuthenticationRefs, ", ")}
	for _, tool := range []struct {
		name    string
		command backend.Command
	}{{"Git", f.Publication.Git}, {"Query", f.Publication.Provider.Query}} {
		description := strings.TrimSpace(tool.command.Executable + " " + strings.Join(tool.command.Args, " "))
		if tool.command.Executable == "" {
			description = "无宿主命令"
		}
		p.Details = append(p.Details, "冻结 "+tool.name+"："+description)
	}
	p.Details = append(p.Details, "受理版本："+f.AcceptedConfigVersion)
	return p
}

func executionDetails(e model.ExecutionRecord) []string {
	lines := []string{}
	status := "资源与工具未核验"
	if e.PolicyResources != nil {
		status = "原生设置已核验；工具未核验"
		lines = append(lines, "已绑定写根："+strings.Join(e.PolicyResources.WriteRoots, ", "), "临时写根：无（隐式临时目录已排除）")
	}
	if e.Environment != nil && e.Environment.Ready {
		status = "原会话工具与准备结算已核验"
	}
	lines = append(lines, "权限核验："+status)
	if e.Environment != nil {
		status := "准备未核实"
		if e.Environment.Ready {
			status = "准备结算已核实"
		}
		lines = append(lines, "环境："+status)
		if b := e.Environment.Blocker; b != nil {
			lines = append(lines, "宿主阻断："+b.Fact.Reason+" · "+b.Fact.Confidence+" · "+b.Fact.Source)
		}
	}
	return lines
}

func copyValue[T any](v *T) *T {
	if v == nil {
		return nil
	}
	result := *v
	return &result
}
