package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	refactoring "github.com/erranliu/SteerLine/internal/refactoring/model"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	advisorycontract "github.com/erranliu/SteerLine/internal/steeragent/suggestion/contract"
	advisory "github.com/erranliu/SteerLine/internal/steeragentbridge/suggestion"
)

type activityConversationSource func(context.Context, taskStorage, string, string) (activityConversations, error)

var activityConversationSources = map[string]activityConversationSource{
	"task_candidate":        candidateConversationTargets,
	"environment_detection": environmentConversationTargets,
	"roadmap":               roadmapConversationTargets,
	"main_agent_advisory":   advisoryConversationTargets,
	"refactoring":           refactoringConversationTargets,
	"baseline_check":        baselineConversationTargets,
}

var errConversationSource = errors.New("activity conversation source unavailable")

func candidateConversationTargets(ctx context.Context, store taskStorage, project, id string) (activityConversations, error) {
	reader, ok := store.(interface {
		Candidate(context.Context, string, string) (runnerstate.Candidate, error)
	})
	if !ok {
		return activityConversations{}, errConversationSource
	}
	candidate, err := reader.Candidate(ctx, project, id)
	if err != nil {
		return activityConversations{}, err
	}
	target := conversationTarget(id, "候选执行", candidate.RunID, candidate.Projection, candidate.RunnerBuild.StateDir)
	return activityConversations{Targets: []activityConversationTarget{target}, Diagnostic: "这里只列出已核实会话绑定的执行；其他轮次的已保存内容可在已记录交互中查看。"}, nil
}

func environmentConversationTargets(ctx context.Context, store taskStorage, project, id string) (activityConversations, error) {
	reader, ok := store.(interface {
		Environment(context.Context, string, string) (env.Record, error)
	})
	if !ok {
		return activityConversations{}, errConversationSource
	}
	record, err := reader.Environment(ctx, project, id)
	if errors.Is(err, env.ErrNotFound) {
		err = sql.ErrNoRows
	}
	if err != nil {
		return activityConversations{}, err
	}
	var list activityConversations
	for _, round := range record.Rounds {
		key := fmt.Sprint(round.Number)
		list.Targets = append(list.Targets, conversationTarget(key, "检测轮次 "+key, round.RunID, round.Run, record.Build.StateDir))
	}
	return list, nil
}

func roadmapConversationTargets(ctx context.Context, store taskStorage, project, id string) (activityConversations, error) {
	reader, ok := store.(interface {
		Roadmap(context.Context, string, string) (roadmap.Record, error)
	})
	if !ok {
		return activityConversations{}, errConversationSource
	}
	record, err := reader.Roadmap(ctx, project, id)
	if errors.Is(err, roadmap.ErrNotFound) {
		err = sql.ErrNoRows
	}
	if err != nil {
		return activityConversations{}, err
	}
	if record.Purpose != "documents" {
		target := conversationTarget(record.ID, "草稿解析", record.RunID, record.Projection, record.Runner.StateDir)
		return activityConversations{Targets: []activityConversationTarget{target}}, nil
	}
	var list activityConversations
	for i, generation := range record.Generations {
		label := fmt.Sprintf("文档生成 %d · %s", i+1, generation.Key)
		list.Targets = append(list.Targets, conversationTarget(generation.ID, label, generation.RunID, generation.Projection, record.Runner.StateDir))
	}
	if record.Publication != nil {
		list.Operation = &activityConversationOperation{
			Label: "发布文档", RequestID: record.Publication.RequestID,
			Status: record.Publication.State, Diagnostic: record.Publication.Diagnostic,
		}
	}
	if record.AdmissionRequest != "" {
		list.Operation = &activityConversationOperation{
			Label: "正式入图", RequestID: record.AdmissionRequest,
			Status: record.State, Diagnostic: record.Diagnostic,
		}
	}
	if list.Operation != nil {
		list.Diagnostic = "可读会话来自文档生成。当前操作结果单独显示。"
	}
	return list, nil
}

func advisoryConversationTargets(ctx context.Context, store taskStorage, project, id string) (activityConversations, error) {
	reader, ok := store.(interface {
		LoadAdvisory(context.Context, string, string) ([]byte, uint64, error)
	})
	if !ok {
		return activityConversations{}, errConversationSource
	}
	raw, revision, err := reader.LoadAdvisory(ctx, project, id)
	if errors.Is(err, advisorycontract.ErrNotFound) {
		err = sql.ErrNoRows
	}
	if err != nil {
		return activityConversations{}, err
	}
	var record advisory.Record
	if err := json.Unmarshal(raw, &record); err != nil {
		return activityConversations{}, err
	}
	if record.Project != project || record.ID != id || record.Revision != revision {
		return activityConversations{}, errors.New("advisory conversation binding differs")
	}
	target := conversationTarget(id, "生成续作提示词建议", record.RunID, record.Projection, record.Runner.StateDir)
	return activityConversations{Targets: []activityConversationTarget{target}}, nil
}

func refactoringConversationTargets(ctx context.Context, store taskStorage, project, id string) (activityConversations, error) {
	reader, ok := store.(interface {
		Refactoring(context.Context, string, string) (refactoring.Record, error)
	})
	if !ok {
		return activityConversations{}, errConversationSource
	}
	record, err := reader.Refactoring(ctx, project, id)
	if errors.Is(err, refactoring.ErrNotFound) {
		err = sql.ErrNoRows
	}
	if err != nil {
		return activityConversations{}, err
	}
	var list activityConversations
	for i, operation := range record.Operations {
		label := fmt.Sprintf("操作 %d · %s", i+1, refactoringConversationLabel(operation.Purpose))
		list.Targets = append(list.Targets, conversationTarget(operation.ID, label, operation.RunID, operation.Run, record.Frozen.Runner.StateDir))
	}
	return list, nil
}

func baselineConversationTargets(ctx context.Context, store taskStorage, project, id string) (activityConversations, error) {
	record, found, err := store.LookupCommand(ctx, project, id)
	if err != nil {
		return activityConversations{}, err
	}
	var payload struct {
		Action string `json:"action"`
	}
	if !found || record.Kind != "project_maintenance" || json.Unmarshal(record.Payload, &payload) != nil || payload.Action != "maintenance_check" {
		return activityConversations{}, sql.ErrNoRows
	}
	var result struct {
		Diagnostic string `json:"diagnostic"`
	}
	_ = json.Unmarshal(record.Result, &result)
	return activityConversations{
		Diagnostic: "本次主分支检查未产生模型会话。",
		Operation: &activityConversationOperation{
			Label:      "检查主分支",
			RequestID:  id,
			Status:     record.Status,
			Diagnostic: result.Diagnostic,
		},
	}, nil
}

func refactoringConversationLabel(purpose string) string {
	switch purpose {
	case "select":
		return "选择方向"
	case "execute":
		return "实施"
	case "audit":
		return "审查"
	case "finish":
		return "收尾"
	case "publish":
		return "发布"
	case "conflict":
		return "处理冲突"
	default:
		return purpose
	}
}
