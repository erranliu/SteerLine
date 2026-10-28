package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/erranliu/SteerLine/internal/prompts"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	roadmapservice "github.com/erranliu/SteerLine/internal/roadmap"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
)

func TestRoadmapPublicationAndAdmissionWithRealGit(t *testing.T) { roadmapPromotionChain(t, false) }

func roadmapPromotionChain(t *testing.T, live bool) {
	c, _ := nativeTaskFixture(t)
	ctx := context.Background()
	fixtureGit(t, c.Repository, "config", "user.name", "Roadmap fixture")
	fixtureGit(t, c.Repository, "config", "user.email", "roadmap@example.invalid")
	fixtureGit(t, c.Repository, "config", "commit.gpgsign", "false")
	// Self-contained rules of the arithmetic fixture project, not SteerLine's
	// own managed-test framework with unavailable linked prerequisites.
	rules := map[string]string{
		"SteerlineFlow/spec/SpecGuide.md":                  "# Spec / Plan 生成规范\n实际读取当前固定版本 logic.go 和 go.mod，按已确认任务与依赖生成完整中文 Spec、Plan、专属验收文件。内部代号不等于正式编号。先整体说明复用、修改所有者和依赖，再逐任务写文档；有关键问题才返回问题。只需读取本文件、testing.md 和 spec-plan-guidelines.md，三份均为本项目完整规范，不引用其他规范。",
		"SteerlineFlow/testing.md":                         "# Arithmetic 测试规范\n项目已有 go.mod。各任务在已有 logic 包新增纯函数与 Go 单元测试，最终验证 go test ./... 和 go vet ./...。计划中写明命令、输入输出断言和证据位置，当前结果均为未验。本项目不使用 SteerLine 仓库的 cmd/test-feedback、L1/L2 或 managed 测试清单，也不需要新增测试基础设施。",
		"SteerlineProjectGuideDoc/spec-plan-guidelines.md": "# Spec / Plan 编写规范\nSpec 写目标、范围、接口行为、非目标和通过条件；Plan 写具体代码修改、既有模块复用、T1 到 T2 交接、单元测试安排、命令和证据。每个任务生成独立验收文件，自动验收描述必须可执行，纯函数无需人工 UI 体验。禁止兼容层、发布系统或额外任务。",
		"go.mod": "module example.invalid/arithmetic\n\ngo 1.24.2\n",
	}
	for name, body := range rules {
		full := filepath.Join(c.Repository, filepath.FromSlash(name))
		require(t, os.MkdirAll(filepath.Dir(full), 0700))
		require(t, os.WriteFile(full, []byte(body), 0600))
	}

	require(t, os.WriteFile(filepath.Join(c.Repository, "logic.go"), []byte("package logic\n// Extend this pure integer arithmetic package.\n"), 0600))
	fixtureGit(t, c.Repository, "add", ".")
	fixtureGit(t, c.Repository, "commit", "-qm", "generation baseline and guidelines")

	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	fixtureGit(t, root, "init", "--bare", "--initial-branch=main", bare)
	fixtureGit(t, c.Repository, "push", bare, "HEAD:refs/heads/main")
	var mu sync.Mutex
	var fact *roadmapPR
	creates, merges := 0, 0
	var server *httptest.Server
	runGit := func(dir string, args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		b, e := cmd.CombinedOutput()
		if e != nil {
			return "", fmt.Errorf("%v: %w: %s", args, e, b)
		}
		return strings.TrimSpace(string(b)), nil
	}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		route := r.URL.Path
		if route == "/api/v1/repos/owner/repo/pulls" && r.Method == "GET" {
			if fact == nil {
				_ = json.NewEncoder(w).Encode([]roadmapPR{})
			} else {
				_ = json.NewEncoder(w).Encode([]roadmapPR{*fact})
			}
			return
		}
		if route == "/api/v1/repos/owner/repo/pulls" && r.Method == "POST" {
			creates++
			var input struct {
				Head string `json:"head"`
				Base string `json:"base"`
			}
			if e := json.NewDecoder(r.Body).Decode(&input); e != nil {
				http.Error(w, e.Error(), 400)
				return
			}
			sha, e := runGit(c.Repository, "rev-parse", input.Head)
			if e != nil {
				http.Error(w, e.Error(), 500)
				return
			}
			f := &roadmapPR{Number: 1, URL: server.URL + "/owner/repo/pulls/1", State: "open"}
			f.Head.Ref = input.Head
			f.Head.SHA = sha
			f.Head.Repo.FullName = "owner/repo"
			f.Base.Ref = input.Base
			fact = f
			// Provider applied the original POST, but the caller lost its receipt.
			http.Error(w, "lost original PR receipt", http.StatusInternalServerError)
			return
		}
		if route == "/api/v1/repos/owner/repo/pulls/1" && r.Method == "GET" && fact != nil {
			_ = json.NewEncoder(w).Encode(fact)
			return
		}
		if route == "/api/v1/repos/owner/repo/pulls/1/merge" && r.Method == "POST" && fact != nil {
			var input map[string]any
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input["head_commit_id"] != fact.Head.SHA || input["do"] != "merge" || input["force_merge"] == true {
				http.Error(w, "unsafe merge", 409)
				return
			}
			merges++
			mergeDir := filepath.Join(root, "provider-merge")
			steps := [][]string{{"clone", bare, mergeDir}, {"-C", mergeDir, "config", "user.name", "Provider"}, {"-C", mergeDir, "config", "user.email", "provider@example.invalid"}, {"-C", mergeDir, "-c", "commit.gpgsign=false", "merge", "--no-ff", "--no-edit", "origin/" + fact.Head.Ref}, {"-C", mergeDir, "push", "origin", "main"}}
			for _, args := range steps {
				if _, e := runGit(root, args...); e != nil {
					http.Error(w, e.Error(), 500)
					return
				}
			}
			sha, e := runGit(mergeDir, "rev-parse", "HEAD")
			if e != nil {
				http.Error(w, e.Error(), 500)
				return
			}
			fact.Merged = true
			fact.State = "closed"
			fact.MergeCommit = sha
			w.WriteHeader(200)
			return
		}
		http.Error(w, "unexpected provider request "+r.Method+" "+route, 404)
	}))
	defer server.Close()
	fakeURL := server.URL + "/owner/repo.git"
	fixtureGit(t, c.Repository, "remote", "set-url", "origin", fakeURL)
	fixtureGit(t, c.Repository, "config", "url."+filepath.ToSlash(bare)+".insteadOf", fakeURL)
	c.PR = executionmodel.PRProviderConfig{Kind: "gitea", BaseURL: server.URL, Repository: "owner/repo"}
	reader, e := OpenNativeTaskProject(ctx, &c)
	require(t, e)
	require(t, reader.Close())
	store, e := OpenNativeTaskCommandStore(ctx, &c)
	require(t, e)
	defer store.Close()
	c.runtimeMaintenance = newProjectMaintenance(c, store)
	port := &roadmapPort{config: c, store: store}
	configurePromotionGeneration(t, port, live)
	var code string
	sourceBody := "# Arithmetic roadmap\nT1: Add a pure Sum(a,b int) int function in package logic; Sum(2,3)=5. Scope is only integer arithmetic and unit tests.\nT2: Add Difference(a,b int) int implemented using Sum(a,-b); Difference(5,2)=3. Depends on T1. Do not implement the functions during planning."
	sourcePath := roadmap.Root + "publish.md"
	sourceFile := filepath.Join(c.Repository, filepath.FromSlash(sourcePath))
	require(t, os.MkdirAll(filepath.Dir(sourceFile), 0700))
	require(t, os.WriteFile(sourceFile, []byte(sourceBody), 0600))
	parseRules, e := prompts.RoadmapParseRules()
	require(t, e)
	docRules, e := prompts.RoadmapDocumentRules()
	require(t, e)
	existingGraph, e := port.graph(ctx)
	require(t, e)
	graphRaw, e := json.Marshal(existingGraph)
	require(t, e)
	for path, body := range map[string][]byte{roadmapservice.GraphPath: graphRaw, roadmapservice.MappingPath: []byte(`{}`), roadmapservice.ParseRulesPath: parseRules, roadmapservice.DocumentRulesPath: docRules} {
		require(t, os.WriteFile(filepath.Join(c.Repository, path), body, 0600))
	}
	fixtureGit(t, c.Repository, "add", ".")
	fixtureGit(t, c.Repository, "commit", "-qm", "prepare original parse materials")
	fixtureGit(t, c.Repository, "push", "origin", "main")
	source, e := port.source(ctx, sourcePath)
	require(t, e)
	parsed := promotionAction(t, port, "roadmap_parse", source.ID, strings.Repeat("a", 32), 1, roadmapservice.ParseInput{RequestID: strings.Repeat("a", 32), Path: sourcePath, Digest: source.Digest, Profile: "fixture"})
	parsed = awaitPromotion(t, port, parsed, "review")
	parent := promotionAction(t, port, "roadmap_confirm", parsed.ID, strings.Repeat("1", 32), parsed.Revision, struct{}{})
	// Explicit normal Git preparation, outside all production commands.
	planning, e := json.Marshal(struct {
		Source    string             `json:"source_digest"`
		Confirmed string             `json:"confirmed_digest"`
		Analysis  *roadmap.Analysis  `json:"analysis"`
		Existing  []roadmap.Existing `json:"existing"`
	}{parent.Source.Digest, parent.ConfirmedDigest, parent.Analysis, parent.Existing})
	require(t, e)
	require(t, os.WriteFile(filepath.Join(c.Repository, roadmapservice.PlanningPath), planning, 0600))
	fixtureGit(t, c.Repository, "add", ".")
	fixtureGit(t, c.Repository, "commit", "-qm", "prepare confirmed planning material")
	fixtureGit(t, c.Repository, "push", "origin", "main")
	code = fixtureGit(t, c.Repository, "rev-parse", "HEAD")
	require(t, os.WriteFile(filepath.Join(c.Repository, "logic.go"), []byte("dirty worktree must not enter the fixed snapshot"), 0600))
	r := promotionAction(t, port, "roadmap_generate_documents", parent.ID, strings.Repeat("2", 32), parent.Revision, struct{}{})
	r = awaitPromotion(t, port, r, "documents-review")
	if r.CodeCommit != code || len(r.Documents) != 2 || len(r.Mapping) != 0 || len(r.Investigation) == 0 {
		t.Fatal("unverified document result", r)
	}
	view, e := port.read(ctx, "/roadmaps/"+r.ID, nil)
	require(t, e)
	preview, _ := json.Marshal(view)
	if !strings.Contains(string(preview), "publication_preview") || !strings.Contains(string(preview), "execution-map.md") {
		t.Fatal("publication preview incomplete")
	}
	// Change the live worktree after freezing: documents still refer to the fixed commit.
	require(t, os.WriteFile(filepath.Join(c.Repository, "logic.go"), []byte("dirty worktree must not enter the fixed snapshot"), 0600))
	pub, e := port.publish(ctx, r, strings.Repeat("b", 32), roadmap.ReviewDigest(r), true)
	if e == nil || pub.Publication.State != "pr_unknown" {
		t.Fatal("missing PR receipt was treated as applied", pub, e)
	}
	pub, e = port.observePublication(ctx, pub)
	require(t, e)
	if pub.State != "awaiting-merge" {
		t.Fatal("original PR was not reconciled", pub)
	}
	// Explicit continuation keeps the original authorization and never creates another PR.
	pub, e = port.resumePublication(ctx, pub)
	require(t, e)
	if pub.State != "awaiting-admission" || pub.Publication.MergedCommit == "" {
		t.Fatal(pub)
	}
	graph, e := store.TaskGraphSnapshot(ctx, c.Identity())
	require(t, e)
	if len(graph.Tasks) != 0 {
		t.Fatal("publication allocated Tasks")
	}
	if fixtureGit(t, c.Repository, "rev-parse", "HEAD") != code {
		t.Fatal("publication changed main worktree HEAD")
	}
	raw, e := os.ReadFile(sourceFile)
	require(t, e)
	if string(raw) != sourceBody {
		t.Fatal("source Markdown changed")
	}
	// This batch is submitted once; replay after a lost HTTP receipt recovers it.
	admitted, e := port.admit(ctx, pub, strings.Repeat("c", 32))
	require(t, e)
	if admitted.State != "admitted" || len(admitted.Mapping) != 2 {
		t.Fatal(admitted)
	}
	graph, e = store.TaskGraphSnapshot(ctx, c.Identity())
	require(t, e)
	if len(graph.Tasks) != 2 || len(graph.Edges) != 1 {
		t.Fatal(graph)
	}
	recovered, e := port.reconcileAdmission(ctx, admitted)
	require(t, e)
	if roadmap.ValueDigest(recovered.Mapping) != roadmap.ValueDigest(admitted.Mapping) {
		t.Fatal("original mapping changed")
	}
	if recovered.Documents[0].Spec.Path != r.Documents[0].Spec.Path {
		t.Fatal("formal allocation renamed documents")
	}
	promotionEvidence(t, "admitted", recovered)
	recovered, e = port.resumeAdmission(ctx, recovered)
	require(t, e)
	mu.Lock()
	defer mu.Unlock()
	if creates != 1 || merges != 1 {
		t.Fatal("duplicate PR effects", creates, merges)
	}
}
