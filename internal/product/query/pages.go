// Package query owns product reads and pure page projections. Page projection
// never reconstructs the execution actor or grants business write authority.
package query

import (
	"encoding/json"
	"fmt"
	"strings"

	backend "github.com/erranliu/SteerLine/internal/execution/backend"
	model "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TaskSummary(t taskmodel.TaskRecord) api.TaskSummary {
	return api.TaskSummary{TaskID: t.TaskID, Title: t.Title, Type: t.Type, Status: t.Status, Version: t.Version, Claimant: t.Claimant}
}

func TaskDetail(t contracts.NativeTaskDetail) api.TaskDetail {
	p := api.TaskDetail{TaskSummary: TaskSummary(t.TaskRecord), ProjectID: t.ProjectID, Priority: t.Priority, SpecPath: t.Contract.SpecRef.Path, Successors: append([]string(nil), t.Successors...), DependencyReady: t.DependencyReady, StartupBlockers: append([]string(nil), t.StartupBlockers...)}
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

func TaskPage(p contracts.NativeTaskPage) api.TaskPage {
	view := api.TaskPage{Protocol: p.Protocol, ProjectID: p.ProjectID, AuthorityVersion: p.AuthorityVersion, GraphRevision: p.GraphRevision, DatabaseRevision: p.DatabaseRevision, DatabaseID: p.DatabaseID, SampledAt: p.SampledAt, Capabilities: api.TaskCapabilities{DependencyReadiness: p.Capabilities.DependencyReadiness, DependencyBlockers: p.Capabilities.DependencyBlockers, StartupBlockers: p.Capabilities.StartupBlockers}, NextCursor: p.NextCursor, HasMore: p.HasMore, Tasks: []api.TaskDetail{}}
	for _, t := range p.Tasks {
		view.Tasks = append(view.Tasks, TaskDetail(t))
	}
	return view
}

func DecisionRequest(r model.DecisionRequest) api.DecisionRequest {
	p := api.DecisionRequest{Question: r.Question, Context: r.Context, Recommended: r.Recommended, AllowFreeText: r.AllowFreeText}
	for _, o := range r.Options {
		p.Options = append(p.Options, api.DecisionOption{ID: o.ID, Label: o.Label, Detail: o.Detail})
	}
	return p
}

func Candidate(r *contracts.Run) *api.CandidateSummary {
	if r == nil {
		return nil
	}
	p := &api.CandidateSummary{SessionID: r.SessionID, SessionGeneration: r.SessionGeneration, CurrentTurnID: r.CurrentTurnID, TurnNumber: r.TurnNumber, PRHistory: append([]string(nil), r.PRHistory...), CandidateCreatedAt: r.CandidateCreatedAt, CandidateEndedAt: copyValue(r.CandidateEndedAt), CandidateTiming: r.CandidateTiming, ResourceStatus: r.ResourceStatus, RecoveryRequired: r.RecoveryRequired, RecoveryState: r.RecoveryState, RecoveryVersion: r.RecoveryVersion, TaskID: r.TaskID, DomainTaskID: r.DomainTaskID, WorkflowVersion: r.WorkflowVersion, ExecutionID: r.ExecutionID, Title: r.Title, RunID: r.RunID, AttemptID: r.AttemptID, ControlRevision: r.ControlRevision, Claimant: r.Claimant, Host: r.Host, Backend: r.Backend, Protocol: r.Protocol, Agent: r.Agent, Target: api.TargetSummary{ID: r.Target.ID}, TaskStatus: r.TaskStatus, ExecutionStatus: r.ExecutionStatus, FinishTaskStatus: r.FinishTaskStatus, StartedAt: copyValue(r.StartedAt), EndedAt: copyValue(r.EndedAt), WorktreePath: copyValue(r.WorktreePath), WorktreeBranch: copyValue(r.WorktreeBranch), BaseCommit: copyValue(r.BaseCommit), LastEvent: copyValue(r.LastEvent), LogPath: copyValue(r.LogPath), ResultPath: copyValue(r.ResultPath), Summary: copyValue(r.Summary), Error: copyValue(r.Error), PR: api.PRSummary{URL: copyValue(r.PR.URL), HeadRevision: copyValue(r.PR.HeadRevision), MergeCommit: copyValue(r.PR.MergeCommit), QueryError: copyValue(r.PR.QueryError), State: r.PR.State, MergedAt: copyValue(r.PR.MergedAt), QueriedAt: copyValue(r.PR.QueriedAt)}}
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

func ServicePage(rt *contracts.Runtime) *api.ServicePage {
	if rt == nil {
		return nil
	}
	p := &api.ServicePage{Revision: rt.Revision, UpdatedAt: rt.UpdatedAt, HeartbeatAt: rt.HeartbeatAt, CurrentTask: copyValue(rt.CurrentTask), SlotOccupied: rt.SlotOccupied, Repository: api.RepositorySummary{Path: rt.Repository.Path, Remote: rt.Repository.Remote, Branch: rt.Repository.Branch, Identity: rt.Repository.Identity}, Service: api.ServiceSummary{InstanceID: rt.Service.InstanceID, Status: rt.Service.Status, ConfigDigest: rt.Service.ConfigDigest, Claimant: rt.Service.Claimant, Mode: rt.Service.Mode, PID: rt.Service.PID, Error: copyValue(rt.Service.Error), StartupID: copyValue(rt.Service.StartupID), Ready: rt.Service.Ready, ControlProtocol: rt.Service.ControlProtocol, Capabilities: append([]string(nil), rt.Service.Capabilities...)}, Control: api.ControlSummary{DispatchMode: rt.Control.DispatchMode, DispatchPaused: rt.Control.DispatchPaused, Revision: rt.Control.Revision}}
	if o := rt.Control.Operation; o != nil {
		p.Control.Operation = &api.PendingControl{RequestID: o.RequestID, Action: o.Action, TaskID: o.TaskID, Phase: o.Phase, RunID: o.RunID, AttemptID: o.AttemptID, DecisionID: o.DecisionID}
	}
	for _, b := range rt.Control.Blockers {
		p.Control.Blockers = append(p.Control.Blockers, api.ControlBlocker{RequestID: b.RequestID, Action: b.Action, TaskID: b.TaskID, RunID: b.RunID, AttemptID: b.AttemptID, DecisionID: b.DecisionID, Reason: b.Reason})
	}
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
	p := api.WorkflowSummary{WorkflowRunID: w.WorkflowRunID, TaskID: w.TaskID, CreatedAt: w.CreatedAt}
	if w.ExecutionProfile != nil {
		v := Profile(*w.ExecutionProfile)
		p.ExecutionProfile = &v
	}
	return p
}

func History(d contracts.DomainSnapshot) api.HistorySummary {
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
			}
		}
		p.Decisions = append(p.Decisions, v)
	}
	return p
}

func Profile(f model.FixedExecutionProfile) api.ExecutionProfileSummary {
	p := api.ExecutionProfileSummary{ID: f.ID, Name: f.Name, Host: f.Host, Agent: f.Agent, AcceptedConfigVersion: f.AcceptedConfigVersion, ImplementationDisplay: append([]string(nil), f.ImplementationDisplay...)}
	p.Details = []string{"发布责任：" + f.PublicationPolicy.Publisher, "冻结发布目标：" + f.Publication.Provider.Kind + " · " + f.Publication.Provider.BaseURL + "/" + f.Publication.Provider.Repository + " · base " + f.Publication.BaseBranch, "Git remote：" + f.Publication.Remote + " · " + f.Publication.RemoteURL, "发布工具：" + f.Publication.Git.Executable + " · query " + f.Publication.Provider.Query.Executable + " · close " + f.Publication.Provider.Close.Executable, "认证引用：" + strings.Join(f.Publication.AuthenticationRefs, ", ")}
	for _, tool := range []struct {
		name    string
		command backend.Command
	}{{"Git", f.Publication.Git}, {"Query", f.Publication.Provider.Query}, {"Close", f.Publication.Provider.Close}} {
		description := strings.TrimSpace(tool.command.Executable + " " + strings.Join(tool.command.Args, " "))
		if tool.command.Executable == "" {
			description = "无宿主命令"
		}
		p.Details = append(p.Details, "冻结 "+tool.name+"："+description)
	}
	p.Details = append(p.Details, fmt.Sprintf("项目要求：读取 %s · 写入 %s · Git %t · 网络 %s · 越权 %s", f.ExecutionPolicy.Filesystem.ReadScope, f.ExecutionPolicy.Filesystem.WriteScope, f.ExecutionPolicy.Filesystem.GitMetadataWrite, f.ExecutionPolicy.Network, f.ExecutionPolicy.Escalation), "受理版本："+f.AcceptedConfigVersion+" · 来源："+f.PolicySource, "附加能力："+strings.Join(f.ExecutionRequirements.Capabilities, ", "))
	for _, tool := range f.ExecutionRequirements.Tools {
		version := tool.ExactVersion
		if version == "" {
			version = "存在即可"
		}
		p.Details = append(p.Details, "必需工具："+tool.ID+" · "+version)
	}
	p.Details = append(p.Details, requirementsDetails(f.RequirementsEvaluation, "领取时需求检查")...)
	return p
}

func requirementsDetails(e *backend.RequirementsEvaluation, label string) []string {
	if e == nil {
		return nil
	}
	lines := []string{label + "：" + e.CheckedAt.Format("2006-01-02 15:04:05Z07:00") + " · " + e.Target.ID + "（工具/引用存在性，不证明沙箱权限或远端授权）"}
	if e.Capabilities != nil {
		for _, cap := range e.Capabilities.Capabilities {
			lines = append(lines, "能力："+cap.Name+" · "+string(cap.Status)+" · 版本 "+e.Capabilities.PlatformVersion)
		}
	}
	for _, values := range [][]backend.RequirementEvidence{e.Tools, e.Credentials} {
		for _, item := range values {
			lines = append(lines, "需求证据："+item.Site+"/"+item.ID+" · "+string(item.Status)+" · "+item.Version+" · "+item.Source)
		}
	}
	return lines
}

func executionDetails(e model.ExecutionRecord) []string {
	lines := requirementsDetails(e.CurrentRequirementsEvaluation, "当前需求检查")
	if e.CurrentRequirementsError != nil {
		lines = append(lines, "需求阻断："+*e.CurrentRequirementsError)
	}
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
