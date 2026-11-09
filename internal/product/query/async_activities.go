package query

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

var AsyncActivitySources = []string{"environment_detection", "baseline_check", "task_candidate", "roadmap"}

// Private read shapes contain only fields used by the observation projection.
// Domain qualification/settlement is supplied by the read port, not recreated.
type activityEnvironment struct {
	Project, ID, State, Diagnostic string
	Profile                        string     `json:"profile_id"`
	CreatedAt                      time.Time  `json:"created_at"`
	ObservedAt                     *time.Time `json:"observed_at"`
	ObservationDiagnostic          string     `json:"observation_diagnostic"`
	Revision                       uint64
	Requests                       []struct {
		ID     string `json:"request_id"`
		Action string
	}
	Rounds []struct {
		Run *struct {
			UpdatedAt time.Time `json:"updated_at"`
		}
	}
}
type activityTests struct {
	ParsedVersion uint64 `json:"parsed_version"`
	Completed     map[string]bool
	Diagnostic    string
}
type AsyncActivityOptions struct {
	Filter, Cursor     string
	PublicProjectID    string
	Sources            []string
	Limit              int
	RequestIDs         []string
	afterTime, afterID string
}
type activityCursor struct {
	Project, Instance, Filter, At, ID string
	Seen                              int
}

func activityKey(f ports.AsyncActivityFact) string { return f.Source + "/" + f.ID }
func beforeFact(a, b ports.AsyncActivityFact, desc bool) bool {
	if a.SortAt == b.SortAt {
		return activityKey(a) < activityKey(b)
	}
	if desc {
		return a.SortAt > b.SortAt
	}
	return a.SortAt < b.SortAt
}
func activityTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
func latestTime(a, b *time.Time) *time.Time {
	if a == nil {
		return b
	}
	if b != nil && b.After(*a) {
		return b
	}
	return a
}
func activityTerminal(state string) bool {
	return state == "succeeded" || state == "failed" || state == "cancelled"
}

// ReadAsyncActivities has only a read capability. It never reconciles a command,
// collects Runner facts or advances a domain workflow.
func ReadAsyncActivities(ctx context.Context, reader ports.AsyncActivityReader, project, instance string, o AsyncActivityOptions) (api.AsyncActivities, error) {
	if o.Filter == "" {
		o.Filter = "ongoing"
	}
	if o.Limit == 0 {
		o.Limit = 50
	}
	if o.Filter != "ongoing" && o.Filter != "attention" && o.Filter != "recent" || o.Limit < 1 || o.Limit > 200 || len(o.RequestIDs) > 100 {
		return api.AsyncActivities{}, errors.New("invalid activities query")
	}
	c := activityCursor{Project: project, Instance: instance, Filter: o.Filter}
	if o.Cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(o.Cursor)
		if e != nil || len(raw) > 2048 {
			return api.AsyncActivities{}, errors.New("invalid activity cursor")
		}
		var saved activityCursor
		if json.Unmarshal(raw, &saved) != nil || saved.Project != project || saved.Instance != instance || saved.Filter != o.Filter || saved.Seen < 0 || saved.Seen > 50 {
			return api.AsyncActivities{}, errors.New("activity cursor scope changed")
		}
		c = saved
	}
	out := api.AsyncActivities{ProjectID: project, InstanceID: instance, SampledAt: time.Now().UTC(), Items: []api.AsyncActivity{}, Links: map[string]api.AsyncActivity{}, Counts: map[string]int{"ongoing": 0, "attention": 0, "recent": 0}, CountsKnown: true}
	if o.PublicProjectID != "" {
		out.ProjectID = o.PublicProjectID
	}
	facts := []ports.AsyncActivityFact{}
	var watermark *ports.AsyncActivityFact
	desc := o.Filter == "recent"
	sources := o.Sources
	if len(sources) == 0 {
		sources = AsyncActivitySources
	}
	o.afterTime, o.afterID = c.At, c.ID
	projected := map[string]api.AsyncActivity{}
	for _, sampled := range sampleActivitySources(ctx, reader, project, instance, sources, o, out.SampledAt, false) {
		out.CountsKnown = out.CountsKnown && sampled.countsKnown
		for filter, count := range sampled.counts {
			out.Counts[filter] += count
		}
		facts = append(facts, sampled.rows...)
		for key, a := range sampled.projected {
			projected[key] = a
		}
		out.Sources = append(out.Sources, sampled.source)
		for id, activity := range sampled.links {
			activity.ProjectID = out.ProjectID
			out.Links[id] = activity
		}
		if sampled.more && len(sampled.rows) > 0 {
			last := sampled.rows[len(sampled.rows)-1]
			if watermark == nil || beforeFact(last, *watermark, desc) {
				watermark = &last
			}
		}
	}

	sort.Slice(facts, func(i, j int) bool { return beforeFact(facts[i], facts[j], desc) })
	var last ports.AsyncActivityFact
	matched := 0
	for _, f := range facts {
		if watermark != nil && beforeFact(*watermark, f, desc) {
			break
		}
		a, ok := projected[activityKey(f)]
		if !ok {
			continue
		}
		a.ProjectID = out.ProjectID
		selectRow := MatchesAsyncActivity(a, o.Filter)
		if selectRow {
			matched++
			if len(out.Items) < o.Limit && (o.Filter != "recent" || len(out.Items)+c.Seen < 50) {
				out.Items = append(out.Items, a)
				last = f
			}
		}
	}
	if out.Counts["recent"] > 50 {
		out.Counts["recent"] = 50
	}
	out.HasMore = matched > len(out.Items) || watermark != nil
	if o.Filter == "recent" && len(out.Items)+c.Seen >= 50 {
		out.HasMore = false
	}
	if out.HasMore {
		if matched <= len(out.Items) && watermark != nil {
			last = *watermark
		}
		if last.ID != "" {
			c.At, c.ID = last.SortAt, activityKey(last)
			if o.Filter == "recent" {
				c.Seen += len(out.Items)
			}
			raw, _ := json.Marshal(c)
			out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
		} else {
			out.HasMore = false
		}
	}
	return out, nil
}

// ProjectAsyncActivity converts committed facts without inventing progress or
// treating read availability as a business outcome.
func ProjectAsyncActivity(project, instance string, f ports.AsyncActivityFact, now time.Time) (api.AsyncActivity, error) {
	a := api.AsyncActivity{ID: base64.RawURLEncoding.EncodeToString([]byte(project + "\x00" + f.Source + "\x00" + f.ID)), ProjectID: project, Source: f.Source, SourceID: f.ID, RequestIDs: []string{}, State: "waiting", Observation: "unverified"}
	var related struct {
		Observation *struct {
			ObservedAt *time.Time `json:"observed_at"`
			Diagnostic string     `json:"diagnostic"`
		} `json:"observation"`
		Settled          bool           `json:"settled"`
		DomainDiagnostic string         `json:"domain_diagnostic"`
		Blocked          bool           `json:"blocked"`
		Current          int            `json:"current"`
		Requests         []string       `json:"requests"`
		TaskStatus       string         `json:"task_status"`
		Title            string         `json:"title"`
		Winner           string         `json:"winner"`
		Progress         *activityTests `json:"progress"`
	}
	if json.Unmarshal(f.Related, &related) != nil {
		return a, errors.New("invalid activity related facts")
	}
	switch f.Source {
	case "environment_detection":
		var r activityEnvironment
		if e := json.Unmarshal(f.Payload, &r); e != nil {
			return a, e
		}
		if r.Project != project || r.ID != f.ID {
			return a, errors.New("environment activity identity differs")
		}
		a.Kind = "detection"
		a.Title = "检测开发环境 · " + r.Profile
		a.Revision = r.Revision
		a.CreatedAt = activityTime(r.CreatedAt)
		a.ObservedAt = r.ObservedAt
		a.Diagnostic = related.DomainDiagnostic
		if r.ObservationDiagnostic != "" {
			a.Observation = "unavailable"
		}
		a.Details = api.AsyncActivityDetails{Kind: "environment", Target: r.ID}
		for _, q := range r.Requests {
			if q.Action == "start" {
				a.RequestIDs = append(a.RequestIDs, q.ID)
			}
		}
		switch r.State {
		case "queued":
			a.State = "queued"
			a.Phase = "等待检测"
		case "running":
			a.State = "running"
			a.Phase = "检测中"
		case "stopping":
			a.Phase = "停止收尾中"
		case "ready":
			if related.Settled {
				a.State = "succeeded"
				a.Phase = "检测通过"
			}
		case "failed":
			if related.Settled {
				a.State = "failed"
				a.Phase = "检测失败"
			}
		case "cancelled":
			if related.Settled {
				a.State = "cancelled"
				a.Phase = "检测已取消"
			}
		default:
			a.Phase = "检测状态待核实"
		}
		for _, round := range r.Rounds {
			if round.Run != nil {
				if activityTerminal(a.State) {
					a.FinishedAt = latestTime(a.FinishedAt, activityTime(round.Run.UpdatedAt))
				}
			}
		}
		if related.Blocked {
			a.State = "waiting"
			a.Phase = "检测阻断"
		}
		a.NeedsAttention = related.Current == 1 && (related.Blocked || a.State == "failed" || r.State == "unverified")
	case "baseline_check":
		var r struct {
			ID        string    `json:"command_id"`
			Status    string    `json:"status"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
			Payload   struct {
				Instance string `json:"instance_id"`
			}
			Result struct {
				Diagnostic string `json:"diagnostic"`
			}
		}
		if e := json.Unmarshal(f.Payload, &r); e != nil {
			return a, e
		}
		a.Kind = "detection"
		a.Title = "检查主分支"
		a.CreatedAt = activityTime(r.CreatedAt)
		a.RequestIDs = []string{f.ID}
		a.Details = api.AsyncActivityDetails{Kind: "maintenance", Target: f.ID}
		a.Diagnostic = r.Result.Diagnostic
		switch r.Status {
		case "applied":
			a.State = "succeeded"
			a.Phase = "检查完成"
		case "rejected":
			a.State = "failed"
			a.Phase = "检查失败"
		case "pending":
			if r.Payload.Instance == instance {
				a.State = "running"
				a.Phase = "正在检查主分支"
			} else {
				a.Phase = "原检查结果待核实"
			}
		default:
			a.Phase = "检查结果待核实"
		}
		if activityTerminal(a.State) {
			a.FinishedAt = activityTime(r.UpdatedAt)
		}
		a.NeedsAttention = related.Current == 1 && (a.State == "failed" || a.State == "waiting" && r.Payload.Instance != instance)
	case "task_candidate":
		var c runnerstate.Candidate
		if e := json.Unmarshal(f.Payload, &c); e != nil {
			return a, e
		}
		if c.Project != project || c.ID != f.ID {
			return a, errors.New("candidate identity differs")
		}
		a.Kind = "task_execution"
		a.Title = fmt.Sprintf("任务 #%s · 候选 %d", c.TaskID, c.Number)
		if related.Title != "" {
			a.Summary = related.Title
		}
		a.Revision = c.Revision
		a.CreatedAt = activityTime(c.CreatedAt)
		a.RequestIDs = []string{c.ID}
		a.Diagnostic = c.Diagnostic
		a.Details = api.AsyncActivityDetails{Kind: "candidate", Target: c.ID, TaskID: c.TaskID}
		active := c.Participation == "active" && !c.StopRequested && related.Winner == "" && related.TaskStatus != "closed" && related.TaskStatus != "completed" && related.TaskStatus != "discarded"
		switch c.SubmitState {
		case "pending":
			a.State = "queued"
			a.Phase = "候选等待提交"
		case "rejected":
			a.State = "failed"
			a.Phase = "启动被拒绝"
		case "cancelled":
			a.State = "cancelled"
			a.Phase = "候选未启动"
		default:
			a.Phase = "提交结果待核实"
		}
		if r := c.Projection; r != nil {
			a.Diagnostic = r.Diagnostic
			switch r.Status {
			case "queued":
				a.State = "queued"
				a.Phase = "执行排队中"
			case "running":
				a.State = "running"
				a.Phase = activityStage(r.Step)
			case "waiting_input":
				a.State = "waiting"
				a.Phase = "等待回答"
			case "waiting_external", "pr_ready":
				a.State = "waiting"
				a.Phase = "等待外部结果"
				if r.Status == "pr_ready" || r.Step == "await_merge" {
					a.Phase = "等待合并"
				}
			case "blocked":
				a.State = "waiting"
				a.Phase = "执行阻断"
			case "failed":
				a.State = "failed"
				a.Phase = "候选执行失败"
			case "completed", "closed":
				a.State = "succeeded"
				a.Phase = "候选执行结束"
			case "stopped":
				a.State = "cancelled"
				a.Phase = "候选已停止"
			default:
				a.State = "waiting"
				a.Phase = "执行状态待核实"
			}
			if active && r.ResumeRequired {
				a.State = "waiting"
				a.Phase = "需要显式恢复"
			}
			a.NeedsAttention = active && (r.Question != nil || r.ResumeRequired || r.Status == "failed" || r.Status == "blocked")
			if active && r.Question != nil && r.Status == "waiting_input" {
				a.Phase = "等待回答"
			}
			if related.Winner == c.ID {
				a.State = "succeeded"
				a.Phase = "任务已交付"
			} else if c.Participation != "active" {
				a.State = "cancelled"
				a.Phase = "候选已结束参与"
			}
			if c.StopRequested && r.StopStatus != "stopped" {
				a.Summary = a.Phase
				a.State = "waiting"
				a.Phase = "停止收尾待核实"
			}
			if activityTerminal(a.State) {
				a.FinishedAt = activityTime(r.UpdatedAt)
			}
		}

		pendingEffects := false
		for _, action := range c.Actions {
			if action.State == "unknown" || action.State == "accepted" && action.ActionStatus != "completed" && action.ActionStatus != "failed" && action.ActionStatus != "rejected" && action.ActionStatus != "cancelled" {
				pendingEffects = true
			}
		}
		if pendingEffects {
			a.Summary = a.Phase
			a.State = "waiting"
			a.Phase = "原操作结果待核实"
			a.FinishedAt = nil
		}
		if p := related.Progress; p != nil && p.ParsedVersion == 0 && p.Diagnostic != "" {
			a.Diagnostic = strings.TrimSpace(a.Diagnostic + " · 测试进度：" + p.Diagnostic)
		}
		if p := related.Progress; p != nil && p.ParsedVersion > 0 {
			total, passed := len(p.Completed), 0
			for _, v := range p.Completed {
				if v {
					passed++
				}
			}
			a.Progress = &api.AsyncActivityProgress{Label: "测试已通过", Total: total, Succeeded: passed, Pending: total - passed, Diagnostic: p.Diagnostic}
		}
	case "roadmap":
		var err error
		a, err = projectRoadmapActivity(a, f, instance, related.Settled, related.Current == 1)
		if err != nil {
			return a, err
		}
		a.RequestIDs = related.Requests
	default:
		return a, errors.New("unknown activity source")
	}
	if related.Observation != nil {
		a.ObservedAt = related.Observation.ObservedAt
		if related.Observation.Diagnostic != "" {
			a.Observation = "unavailable"
			a.Diagnostic = strings.TrimSpace(a.Diagnostic + " · 进度观察：" + related.Observation.Diagnostic)
		}
	}
	if a.ObservedAt != nil && a.Observation != "unavailable" {
		a.Observation = "current"
		if now.Sub(*a.ObservedAt) > time.Minute {
			a.Observation = "stale"
		}
	}
	if activityTerminal(a.State) {
		a.Observation = "current"
	}
	if a.Phase == "" {
		a.Phase = "状态待核实"
	}
	return a, nil
}
func activityStage(stage string) string {
	switch stage {
	case "implement":
		return "实施中"
	case "audit", "review":
		return "审查中"
	case "repair":
		return "修复中"
	case "publish":
		return "发布中"
	case "report":
		return "整理报告"
	default:
		return "执行中"
	}
}

func ParseAsyncActivityRequestIDs(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

// MatchesAsyncActivity is the final product predicate. SQL may only prune a
// conservative superset; it does not own the activity classification.
func MatchesAsyncActivity(a api.AsyncActivity, filter string) bool {
	return filter == "ongoing" && !activityTerminal(a.State) || filter == "attention" && a.NeedsAttention || filter == "recent" && activityTerminal(a.State)
}
