package roadmap

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// GenerateDocuments creates another content record with the same lifecycle.
// It freezes the confirmed planning snapshot; it cannot allocate a Task number.
func (s *Service) GenerateDocuments(ctx context.Context, parent, request string, expected uint64) (model.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !model.IDPattern.MatchString(request) {
		return model.Record{}, errors.New("invalid request_id")
	}
	if old, err := s.Store.Roadmap(ctx, s.Project, request); err == nil {
		if old.Purpose != "documents" || old.ParentID != parent {
			return old, errors.New("request_conflict")
		}
		return old, nil
	} else if !errors.Is(err, model.ErrNotFound) {
		return model.Record{}, err
	}
	r, err := s.Store.Roadmap(ctx, s.Project, parent)
	if err != nil {
		return r, err
	}
	if r.Revision != expected || r.State != "confirmed" || r.Analysis == nil || r.Purpose == "documents" || !r.ObservationComplete() {
		return r, errors.New("请先核实原解析已结束，并确认当前任务和依赖")
	}
	rows, err := s.Store.Roadmaps(ctx, s.Project)
	if err != nil {
		return r, err
	}
	for _, old := range rows {
		s.track(old)
		if old.Source.ID == r.Source.ID && !old.ObservationComplete() {
			return r, errors.New("原内容请求尚未结算，请先查询或停止")
		}
		if old.Source.ID == r.Source.ID && old.Publication != nil && old.State != "admitted" {
			return r, errors.New("存在在途发布，请先核实原请求")
		}
	}
	source, err := s.Source(ctx, r.Source.Path)
	if err != nil {
		return r, err
	}
	graph, err := s.Graph(ctx)
	if err != nil {
		return r, err
	}
	if source.Digest != r.Source.Digest || model.GraphDigest(graph) != r.GraphDigest {
		return r, errors.New("来源或已有图材料变化，请重新解析确认")
	}
	execution, catalog, err := s.Prepare(ctx, r.Profile)
	if err != nil {
		return r, err
	}
	if reason := catalog.RepositoryGenerationUnavailable(); reason != "" {
		return r, errors.New(reason)
	}
	if s.PrepareDocuments == nil {
		return r, errors.New("固定代码和生成规范不可用")
	}
	fixed, err := s.PrepareDocuments(ctx, r)
	if err != nil {
		return r, err
	}
	r = model.Record{ID: request, RequestID: request, Purpose: "documents", ParentID: parent, Project: s.Project, Revision: 1,
		Source: r.Source, Existing: r.Existing, GraphDigest: r.GraphDigest, Profile: r.Profile, CreatedAt: time.Now().UTC(),
		State: "queued", Runner: s.Build, Submission: "pending", Analysis: r.Analysis, ConfirmedDigest: r.ConfirmedDigest,
		CodeCommit: fixed.Commit, RepositoryTree: fixed.Tree, Materials: fixed.Materials, Documents: []model.Documents{}, Mapping: r.Mapping}
	if len(model.PlannedDocuments(r)) == 0 {
		return r, errors.New("没有尚未入图的新任务")
	}
	planning, _ := json.Marshal(struct {
		Source    string           `json:"source_digest"`
		Confirmed string           `json:"confirmed_digest"`
		Analysis  *model.Analysis  `json:"analysis"`
		Existing  []model.Existing `json:"existing"`
	}{r.Source.Digest, r.ConfirmedDigest, r.Analysis, r.Existing})
	materials := append([]runnercontract.Document(nil), fixed.Materials...)
	materials = append(materials, runnercontract.Document{Kind: "inline", Path: r.Source.Path, Content: &r.Source.Body},
		runnercontract.Document{Kind: "inline", Path: "confirmed-planning.json", Content: stringPointer(string(planning))})
	input := runnercontract.GenerateRunInput{Title: "Roadmap Spec / Plan: " + r.Source.Path,
		Correlation: map[string]string{"steerline_project": s.Project, "steerline_roadmap_parse": request},
		Workflow:    runnercontract.Workflow{ID: "generate", Version: 1}, Execution: execution,
		Generate: runnercontract.GenerateInput{Repository: &runnercontract.GenerateRepository{LocalPath: s.Repository, Commit: r.CodeCommit},
			Instruction: DocumentInstruction, Materials: materials, OutputSchema: model.DocumentSchema(r)}}
	if err = catalog.ValidateRepositoryGenerate(input); err != nil {
		return r, err
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return r, err
	}
	if len(payload) > 8<<20 {
		return r, errors.New("生成请求超过 Runner 完整输入上限")
	}
	r.Request = runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: Token(), Input: payload}
	if err = s.Store.SaveRoadmap(ctx, r, 0); err != nil {
		return r, err
	}
	s.track(r)
	return s.dispatch(ctx, r)
}

const DocumentInstruction = "依据完整内联规范、原 Markdown 和已确认任务/依赖，实际只读调查当前固定代码快照中的相关源码、契约、消费者和测试，再做整批设计并生成所有新任务的完整中文 Spec、Plan 和专属验收材料。先阅读 SpecGuide 与测试规范，引用实际调查文件的仓库相对路径，investigation.sha256 使用固定快照文件内容的 SHA-256。已有正式任务只作引用；保留所有内部代号、任务范围和依赖，不分配新正式编号，不创建 Task/Candidate，不执行实现或测试，不写文件、Git 或 PR。Spec 写清目标、范围、行为契约及验收要求，Plan 写清复用与整合、修改所有者、依赖交接、具体实施和测试命令/环境/通过条件/证据位置；验收材料区分自动与人工且结果均为未验。每份正文应含标题及完整章节，不使用 TODO、占位正文或声称测试已通过。默认简洁设计，不为未来增添兼容、双写或第二套执行链路。发现需要改变任务拆分、范围或依赖，返回 questions 和空 documents，不能暗改已确认规划。路径和文件集严格按 output_schema 返回，正文完整，不输出代码围栏或解释。整个最终 JSON 限 1 MiB，超出时明确失败，不缩略正文伪装完成。"

func sameRepositoryPath(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(strings.ReplaceAll(a, "\\", "/"), "/"), strings.TrimRight(strings.ReplaceAll(b, "\\", "/"), "/"))
}
