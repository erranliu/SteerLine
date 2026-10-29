package roadmap

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/prompts"
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
	// Guideline bytes remain internal provenance, never model inline materials.
	for _, material := range fixed.Materials {
		if material.Content == nil || strings.TrimSpace(*material.Content) == "" {
			return r, errors.New("固定生成规范内容缺失")
		}
		if _, err := s.fixedFile(ctx, r.CodeCommit, material.Path, []byte(*material.Content), false); err != nil {
			return r, err
		}
	}
	for _, item := range []struct {
		path     string
		expected []byte
	}{{r.Source.Path, []byte(r.Source.Body)}, {PlanningPath, planning}} {
		file, err := s.fixedFile(ctx, r.CodeCommit, item.path, item.expected, false)
		if err != nil {
			return r, err
		}
		r.Materials = append(r.Materials, file)
	}
	rules, err := prompts.RoadmapDocumentRules()
	if err != nil {
		return r, err
	}
	file, err := s.fixedFile(ctx, r.CodeCommit, DocumentRulesPath, rules, true)
	if err != nil {
		return r, err
	}
	r.Materials = append(r.Materials, file)
	instruction, err := prompts.RoadmapDocuments(r.CodeCommit, r.Source.Path)
	if err != nil {
		return r, err
	}
	input := runnercontract.GenerateRunInput{Title: "Roadmap Spec / Plan: " + r.Source.Path,
		Correlation: map[string]string{"steerline_project": s.Project, "steerline_roadmap_parse": request},
		Workflow:    runnercontract.Workflow{ID: "generate", Version: 1}, Execution: execution,
		Generate: runnercontract.GenerateInput{Repository: &runnercontract.GenerateRepository{LocalPath: s.Repository, Commit: r.CodeCommit},
			Instruction: instruction, Materials: []runnercontract.Document{}, OutputSchema: model.DocumentSchema(r)}}
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

func sameRepositoryPath(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(strings.ReplaceAll(a, "\\", "/"), "/"), strings.TrimRight(strings.ReplaceAll(b, "\\", "/"), "/"))
}
