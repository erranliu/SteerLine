package roadmap

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/erranliu/SteerLine/internal/prompts"
	"os"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func repositoryCatalog(t *testing.T, original *runnerclient.Catalog) *runnerclient.Catalog {
	t.Helper()
	raw, err := os.ReadFile("../runnerclient/testdata/generate-repository-capability.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Generate *runnerclient.GenerateCapability `json:"generate"`
		Input    json.RawMessage                  `json:"generate_input"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	caps := original.Capabilities
	caps.Generate = fixture.Generate
	caps.ConfigurationSchemas.GenerateInput = fixture.Input
	catalog, err := runnerclient.CompileCatalog(caps)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}
func documentSetup(t *testing.T) (*Service, *memoryRoadmaps, *roadmapRunner, model.Record) {
	s, store, f, input := generationSetup(t)
	ctx := context.Background()
	r, err := s.Parse(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(sampleAnalysis())
	f.run.ContentResult = &runnercontract.ContentResult{Status: "succeeded", JSON: raw, Source: runnercontract.Final{Verified: true, NativeOutcome: "succeeded", CommandID: "parse", TurnID: "turn"}}
	f.run.StopStatus = "stopped"
	f.run.Status = "completed"
	r, err = s.Observe(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Confirm(ctx, r.ID, strings.Repeat("c", 32), r.Revision)
	if err != nil {
		t.Fatal(err)
	}
	prepare := s.Prepare
	s.Prepare = func(ctx context.Context, id string) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
		exec, cat, e := prepare(ctx, id)
		return exec, repositoryCatalog(t, cat), e
	}
	s.Repository = t.TempDir()
	s.PrepareDocuments = func(context.Context, model.Record) (model.DocumentContext, error) {
		body := "fixed generation guidelines"
		return model.DocumentContext{Commit: strings.Repeat("a", 40), Tree: strings.Repeat("b", 40), Materials: []runnercontract.Document{{Kind: "inline", Path: "SpecGuide.md", Content: &body}, {Path: "SteerlineFlow/environment-project.json", Content: stringPtr(`{"version":1,"requirements":[],"testing_catalogue":"quality/cases.json"}`)}, {Path: "quality/cases.json", Content: stringPtr(`{"tests":[{"id":"one","name":"One","level":"L1","environment":"isolated","command":"test","check":"pass"}]}`)}}}, nil
	}
	rules, err := prompts.RoadmapDocumentRules()
	if err != nil {
		t.Fatal(err)
	}
	f.files[DocumentRulesPath] = rules
	f.files["SpecGuide.md"] = []byte("fixed generation guidelines")
	f.files["SteerlineFlow/environment-project.json"] = []byte(`{"version":1,"requirements":[],"testing_catalogue":"quality/cases.json"}`)
	f.files["quality/cases.json"] = []byte(`{"tests":[{"id":"one","name":"One","level":"L1","environment":"isolated","command":"test","check":"pass"}]}`)
	f.files["code.go"] = []byte("fixed code\n")
	return s, store, f, r
}
func documentResult(r model.Record) (model.GeneratedDocuments, *runnercontract.RepositorySource) {
	source := &runnercontract.RepositorySource{LocalPath: "", Commit: r.CodeCommit, Tree: r.RepositoryTree,
		Files: []runnercontract.RepositoryFile{{Path: "code.go", Blob: strings.Repeat("d", 40), SHA256: model.Digest([]byte("fixed code\n")), SizeBytes: len("fixed code\n")}}}
	for _, m := range r.Materials {
		source.Files = append(source.Files, runnercontract.RepositoryFile{Path: m.Path, Blob: strings.Repeat("d", 40), SHA256: model.Digest([]byte(*m.Content)), SizeBytes: len(*m.Content)})
	}
	out := model.GeneratedDocuments{SourceDigest: r.Source.Digest, ConfirmedDigest: r.ConfirmedDigest, CodeCommit: r.CodeCommit,
		Design: "Reuse the existing owner; no second workflow.", Investigation: []model.InvestigationReference{{Source: "repository", Path: "code.go", Reason: "Existing implementation"}},
		Questions: []string{}, Documents: model.PlannedDocuments(r)}
	for i := range out.Documents {
		out.Documents[i].Spec.Body = "# Spec\n## 目标\n完整目标\n## 范围\n当前范围\n## 验收\n通过条件"
		out.Documents[i].Tests = []string{"one"}
		out.Documents[i].Plan.Body = "# Plan\n## 实施\n复用现有代码\n## 测试\n明确命令与证据"
		out.Documents[i].Acceptance[0].Body = "# 验收\n## 自动\n待执行\n## 人工\n待安排"
	}
	return out, source
}
func TestRoadmapRepositoryDocumentsLifecycle(t *testing.T) {
	s, store, f, parent := documentSetup(t)
	ctx := context.Background()
	r, err := s.GenerateDocuments(ctx, parent.ID, strings.Repeat("d", 32), parent.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if r.ParentID != parent.ID || r.Purpose != "documents" || r.DocumentsVerified || len(r.Mapping) != 0 {
		t.Fatal(r)
	}
	var input runnercontract.GenerateRunInput
	if err = json.Unmarshal(f.request.Input, &input); err != nil {
		t.Fatal(err)
	}
	if input.Generate.Repository == nil || input.Generate.Repository.Commit != r.CodeCommit || input.Generate.Repository.LocalPath != s.Repository {
		t.Fatal(input)
	}
	if len(input.Generate.Materials) != 2 || !strings.Contains(input.Generate.Instruction, PlanningPath) || strings.Contains(input.Generate.Instruction, parent.Source.Body) {
		t.Fatal("materials expanded instead of referenced")
	}

	if _, err = s.GenerateDocuments(ctx, parent.ID, r.RequestID, parent.Revision); err != nil || f.submits != 2 {
		t.Fatal("duplicate document submission", err, f.submits)
	}
	out, source := documentResult(r)
	source.LocalPath = s.Repository
	readCommitted := s.ReadCommitted
	readCode := false
	s.ReadCommitted = func(ctx context.Context, commit, path string) ([]byte, error) {
		if commit != r.CodeCommit {
			t.Fatalf("investigation read changed commit: %s", commit)
		}
		if path == "code.go" {
			readCode = true
		}
		return readCommitted(ctx, commit, path)
	}
	raw, _ := json.Marshal(out)
	f.run.RepositorySource = source
	f.run.ContentResult = &runnercontract.ContentResult{Status: "succeeded", JSON: raw, RepositorySource: source, Source: runnercontract.Final{Verified: true, NativeOutcome: "succeeded", CommandID: "docs", TurnID: "docs-turn"}}
	f.run.Status = "completed"
	f.run.StopStatus = "running"
	waiting, err := s.Observe(ctx, r.ID)
	if err != nil || waiting.DocumentsVerified {
		t.Fatal("unsettled result adopted", waiting, err)
	}
	f.run.StopStatus = "stopped"
	f.run.Revision++
	done, err := s.Observe(ctx, r.ID)
	if err != nil || done.State != "documents-review" || len(done.Documents) != 2 {
		t.Fatal(done, err)
	}
	if !done.ObservationComplete() || len(done.Mapping) != 0 {
		t.Fatal(done)
	}
	if !readCode || len(done.Investigation) != 1 || done.Investigation[0].SHA256 != model.Digest([]byte("fixed code\n")) {
		t.Fatal("platform did not derive original Git digest", done.Investigation)
	}
	stored, _ := store.Roadmap(ctx, s.Project, r.ID)
	if stored.Materials[0].Content == nil || *stored.Materials[0].Content != "fixed generation guidelines" {
		t.Fatal("guideline snapshot lost")
	}
}
func TestRoadmapDocumentOutputRejectsMismatchesAndSupportsQuestions(t *testing.T) {
	s, _, _, parent := documentSetup(t)
	r, err := s.GenerateDocuments(context.Background(), parent.ID, strings.Repeat("d", 32), parent.Revision)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*model.GeneratedDocuments, *runnercontract.RepositorySource){
		func(o *model.GeneratedDocuments, _ *runnercontract.RepositorySource) { o.Documents = o.Documents[:1] },
		func(o *model.GeneratedDocuments, _ *runnercontract.RepositorySource) {
			o.Documents[0].Spec.Path = "outside.md"
		},
		func(o *model.GeneratedDocuments, _ *runnercontract.RepositorySource) { o.ConfirmedDigest = "stale" },
		func(o *model.GeneratedDocuments, _ *runnercontract.RepositorySource) { o.Documents[0].Plan.Body = "" },
		func(o *model.GeneratedDocuments, _ *runnercontract.RepositorySource) {
			o.Investigation[0].Path = "unknown.go"
		},
		func(_ *model.GeneratedDocuments, src *runnercontract.RepositorySource) {
			src.Commit = strings.Repeat("f", 40)
		},
		func(_ *model.GeneratedDocuments, src *runnercontract.RepositorySource) { src.Files = src.Files[:1] },
		func(_ *model.GeneratedDocuments, src *runnercontract.RepositorySource) {
			src.Files[1].SHA256 = strings.Repeat("f", 64)
		},
	} {
		out, src := documentResult(r)
		mutate(&out, src)
		raw, _ := json.Marshal(out)
		if _, err = model.DecodeDocuments(r, raw, src); err == nil {
			t.Fatal("invalid document result accepted")
		}
	}
	out, src := documentResult(r)
	out.Questions = []string{"任务拆分需要重新确认"}
	out.Documents = []model.Documents{}
	raw, _ := json.Marshal(out)
	blocked, err := model.DecodeDocuments(r, raw, src)
	if err != nil || blocked.State != "documents-blocked" || blocked.DocumentsVerified {
		t.Fatal(blocked, err)
	}
}
func TestRoadmapDocumentsUnknownAndLateStopNeverResubmits(t *testing.T) {
	s, _, f, parent := documentSetup(t)
	ctx := context.Background()
	f.lost = true
	r, err := s.GenerateDocuments(ctx, parent.ID, strings.Repeat("d", 32), parent.Revision)
	if err == nil || r.State != "unknown" {
		t.Fatal(r, err)
	}
	r, err = s.Stop(ctx, r.ID, strings.Repeat("e", 32))
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Observe(ctx, r.ID)
	if err != nil || f.stops != 1 || f.submits != 2 {
		t.Fatal(r, err, f.stops, f.submits)
	}
	f.run.StopStatus = "stopped"
	f.run.Revision++
	r, err = s.Observe(ctx, r.ID)
	if err != nil || !r.ObservationComplete() {
		t.Fatal(r, err)
	}
}
func TestRoadmapDocumentsDoNotSubmitWithoutCapabilityOrGuidelines(t *testing.T) {
	for _, missing := range []string{"capability", "guidelines"} {
		t.Run(missing, func(t *testing.T) {
			s, store, f, parent := documentSetup(t)
			if missing == "guidelines" {
				s.PrepareDocuments = func(context.Context, model.Record) (model.DocumentContext, error) {
					return model.DocumentContext{}, fmt.Errorf("required guideline missing")
				}
			} else {
				prepare := s.Prepare
				s.Prepare = func(ctx context.Context, id string) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
					exec, cat, err := prepare(ctx, id)
					cat.Capabilities.Generate = nil
					return exec, cat, err
				}
			}
			if _, err := s.GenerateDocuments(context.Background(), parent.ID, strings.Repeat("d", 32), parent.Revision); err == nil {
				t.Fatal("missing prerequisite accepted")
			}
			if f.submits != 1 || len(store.rows) != 1 {
				t.Fatal("missing prerequisite created document run")
			}
		})
	}
}

func TestRoadmapDocumentInvestigationSeparatesInputFiles(t *testing.T) {
	s, _, _, parent := documentSetup(t)
	r, err := s.GenerateDocuments(context.Background(), parent.ID, strings.Repeat("d", 32), parent.Revision)
	if err != nil {
		t.Fatal(err)
	}
	var request runnercontract.GenerateRunInput
	if err := json.Unmarshal(r.Request.Input, &request); err != nil {
		t.Fatal(err)
	}
	out, source := documentResult(r)
	input := request.Generate.Materials[0]
	ref := model.InvestigationReference{Source: "input_file", Path: input.Path, Reason: "Original roadmap input"}
	out.Investigation = append(out.Investigation, ref)
	raw, _ := json.Marshal(out)
	decoded, err := model.DecodeDocuments(r, raw, source)
	if err != nil {
		t.Fatal("verified original input citation rejected", err)
	}
	if decoded.Investigation[1].SHA256 != model.Digest([]byte(*input.Content)) {
		t.Fatal("input digest not derived from original request", decoded.Investigation)
	}
	for _, mutate := range []func(*model.GeneratedDocuments){
		func(o *model.GeneratedDocuments) { o.Investigation[1].Source = "repository" },
		func(o *model.GeneratedDocuments) { o.Investigation[1].Source = "" },
		func(o *model.GeneratedDocuments) { o.Investigation[1].Source = "unknown" },
		func(o *model.GeneratedDocuments) { o.Investigation[1].Path = "code.go" },
		func(o *model.GeneratedDocuments) { o.Investigation = o.Investigation[1:] },
	} {
		changed := out
		changed.Investigation = append([]model.InvestigationReference{}, out.Investigation...)
		mutate(&changed)
		raw, _ := json.Marshal(changed)
		if _, err := model.DecodeDocuments(r, raw, source); err == nil {
			t.Fatal("unverified or misclassified investigation adopted")
		}
	}
}

func TestRoadmapInvestigationChecksOriginalGitBytes(t *testing.T) {
	for _, mismatch := range []string{"digest", "size", "missing", "read_error", "changed_bytes"} {
		t.Run(mismatch, func(t *testing.T) {
			s, _, f, parent := documentSetup(t)
			r, err := s.GenerateDocuments(context.Background(), parent.ID, strings.Repeat("d", 32), parent.Revision)
			if err != nil {
				t.Fatal(err)
			}
			out, source := documentResult(r)
			source.LocalPath = s.Repository
			switch mismatch {
			case "digest":
				source.Files[0].SHA256 = strings.Repeat("f", 64)
			case "size":
				source.Files[0].SizeBytes++
			case "missing":
				delete(f.files, "code.go")
			case "read_error":
				s.ReadCommitted = func(context.Context, string, string) ([]byte, error) { return nil, os.ErrPermission }
			case "changed_bytes":
				f.files["code.go"] = []byte("changed code\n")
			}
			raw, _ := json.Marshal(out)
			f.run.RepositorySource = source
			f.run.ContentResult = &runnercontract.ContentResult{Status: "succeeded", JSON: raw, RepositorySource: source, Source: runnercontract.Final{Verified: true, NativeOutcome: "succeeded", CommandID: "docs", TurnID: "docs-turn"}}
			f.run.Status, f.run.StopStatus = "completed", "stopped"
			done, err := s.Observe(context.Background(), r.ID)
			if err != nil || done.State != "invalid" || done.DocumentsVerified {
				t.Fatal("unverified Git bytes adopted", done, err)
			}
		})
	}
}

func stringPtr(s string) *string { return &s }

func TestRoadmapDocumentTestsUseFrozenCatalogue(t *testing.T) {
	service, _, _, parent := documentSetup(t)
	record, err := service.GenerateDocuments(context.Background(), parent.ID, strings.Repeat("d", 32), parent.Revision)
	if err != nil {
		t.Fatal(err)
	}
	documents, _ := documentResult(record)
	record.Documents = documents.Documents
	catalogue, err := TestCatalogue(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range [][]string{nil, {"missing"}, {"one", "one"}, {}} {
		changed := record
		changed.Documents = append([]model.Documents(nil), record.Documents...)
		changed.Documents[0].Tests = selection
		if err := ValidateTestIDs(changed, catalogue); err == nil {
			t.Fatal("invalid test selection accepted", selection)
		}
	}
	record.Documents[0].Tests = []string{}
	record.Documents[0].Plan.Body += "\n无需自动测试：仅文档。"
	if err := ValidateTestIDs(record, catalogue); err != nil {
		t.Fatal(err)
	}
}
