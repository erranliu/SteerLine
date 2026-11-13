package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	roadmapservice "github.com/erranliu/SteerLine/internal/roadmap"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// This controlled public-protocol fixture returns full, schema-shaped results.
// All Git, document preparation, publication and Task writes use production owners.
type promotionRunner struct {
	runs              map[string]runnercontract.Run
	requests          map[string]runnercontract.Request
	t                 *testing.T
	port              *roadmapPort
	run               runnercontract.Run
	request           runnercontract.Request
	formalPredecessor string
	futureTests       bool
	sixTasks          bool
	holdDocuments     bool
}

func (f *promotionRunner) Submit(ctx context.Context, q runnercontract.Request) (runnerclient.Receipt, error) {
	f.request = q
	var in runnercontract.GenerateRunInput
	require(f.t, json.Unmarshal(q.Input, &in))
	f.run = runnercontract.Run{RunID: q.RequestID, Revision: 1, Correlation: in.Correlation, Workflow: in.Workflow,
		InputDigest: roadmap.Digest(q.Input), Status: "completed", StopStatus: "stopped"}
	var result any
	r, err := f.port.store.Roadmap(ctx, f.port.config.Identity(), in.Correlation["steerline_roadmap_parse"])
	require(f.t, err)
	f.run.RepositorySource = appRepositoryManifest(f.t, *in.Generate.Repository)
	for _, material := range in.Generate.Materials {
		location := filepath.Join(f.port.config.Repository, ".runner-fixture-input", q.RequestID, filepath.FromSlash(material.Path))
		f.run.Materials = append(f.run.Materials, runnercontract.Material{Kind: "file", Path: material.Path, Digest: roadmap.Digest([]byte(*material.Content)), SnapshotPath: location, ExecutionLocation: location})
	}
	if r.Purpose != "documents" {
		body := ""
		for _, material := range in.Generate.Materials {
			if material.Path == roadmapservice.SourceInputPath {
				body = *material.Content
			}
		}
		if body == "" {
			f.t.Fatal("original file material absent")
		}
		result = roadmap.Analysis{Coordination: "Reuse the declared owners and confirmed dependency handoffs.", Tasks: []roadmap.Node{
			{Key: "T1", KeyOrigin: "markdown", Title: "Sum function", Goal: "Sum two integers", Scope: "Pure function", Acceptance: "Sum(2,3) equals 5", Evidence: strings.Split(body, "\n")[1], Original: roadmap.Dependencies{}, Proposed: roadmap.Dependencies{}},
			{Key: "T2", KeyOrigin: "markdown", Title: "Difference function", Goal: "Subtract via Sum", Scope: "Pure function", Acceptance: "Difference(5,2) equals 3", Evidence: strings.Split(body, "\n")[2], Original: roadmap.Dependencies{Local: []string{"T1"}}, Proposed: roadmap.Dependencies{Local: []string{"T1"}}},
		}, Questions: []string{}, Overlap: "No existing Tasks"}
	} else {
		r, err := f.port.store.Roadmap(ctx, f.port.config.Identity(), in.Correlation["steerline_roadmap_parse"])
		require(f.t, err)
		text, err := f.port.materials.readCommittedMaterial(ctx, r.CodeCommit, "logic.go")
		require(f.t, err)
		blob := fixtureGit(f.t, f.port.config.Repository, "rev-parse", r.CodeCommit+":logic.go")
		src := &runnercontract.RepositorySource{LocalPath: in.Generate.Repository.LocalPath, Commit: r.CodeCommit, Tree: r.RepositoryTree,
			Files: []runnercontract.RepositoryFile{{Path: "logic.go", Blob: blob, SHA256: roadmap.Digest(text), SizeBytes: len(text)}}}
		for _, guide := range r.Materials {
			body, e := f.port.materials.readCommittedMaterial(ctx, r.CodeCommit, guide.Path)
			require(f.t, e)
			blob := fixtureGit(f.t, f.port.config.Repository, "rev-parse", r.CodeCommit+":"+guide.Path)
			src.Files = append(src.Files, runnercontract.RepositoryFile{Path: guide.Path, Blob: blob, SHA256: roadmap.Digest(body), SizeBytes: len(body)})
		}
		f.run.RepositorySource = src
		out := roadmap.GeneratedDocuments{SourceDigest: r.Source.Digest, ConfirmedDigest: r.ConfirmedDigest, CodeCommit: r.CodeCommit,
			Design: "Reuse package logic; T2 follows T1.", Investigation: []roadmap.InvestigationReference{{Source: "repository", Path: "logic.go", Reason: "Pure logic owner"}}, Questions: []string{}, Documents: roadmap.PlannedDocuments(r)}
		for i := range out.Documents {
			out.Documents[i].Spec.Body = "# Spec\n## 目标\n" + r.Analysis.Tasks[i].Goal + "\n## 范围\n纯函数\n## 验收\n明确输入输出"
			out.Documents[i].Tests = []string{"arithmetic.unit"}
			out.Documents[i].Plan.Body = "# Plan\n## 实施\n扩展 logic.go\n## 测试\n运行 go test ./...，报告保存在独立目录"
			out.Documents[i].Acceptance[0].Body = "# 验收\n## 自动\n待执行\n## 人工\n无需"
		}
		if f.futureTests {
			out.Documents[0].ProvidesTests = []string{"arithmetic.difference"}
			out.Documents[1].Tests = append(out.Documents[1].Tests, "arithmetic.difference")
		}
		key := strings.TrimPrefix(in.Title, "Roadmap Spec / Plan: ")
		selected := []roadmap.Documents{}
		for _, d := range out.Documents {
			if d.Key == key {
				selected = append(selected, d)
			}
		}
		out.Documents = selected
		result = out
	}
	if a, ok := result.(roadmap.Analysis); ok {
		if f.sixTasks {
			for i := 3; i <= 6; i++ {
				key := fmt.Sprintf("T%d", i)
				a.Tasks = append(a.Tasks, roadmap.Node{Key: key, KeyOrigin: "markdown", Title: "Arithmetic " + key, Goal: "Pure arithmetic", Scope: "Pure function", Acceptance: "Unit tests", Evidence: strings.Split(instructionSource(in), "\n")[i], Original: roadmap.Dependencies{Local: []string{"T1"}}, Proposed: roadmap.Dependencies{Local: []string{"T1"}}})
			}
		}

		if f.futureTests {
			a.Tasks[0].ProvidesTests = []string{"arithmetic.difference"}
		}
		for i := range a.Tasks {
			if a.Tasks[i].ProvidesTests == nil {
				a.Tasks[i].ProvidesTests = []string{}
			}
		}
		if f.formalPredecessor != "" {
			found := false
			for _, material := range in.Generate.Materials {
				if material.Path == roadmapservice.GraphPath && strings.Contains(*material.Content, `"id":"`+f.formalPredecessor+`"`) {
					found = true
				}
			}
			if !found {
				f.t.Fatal("formal predecessor absent from frozen graph input")
			}
			for i := range a.Tasks {
				a.Tasks[i].Original.Formal = []string{f.formalPredecessor}
				a.Tasks[i].Proposed.Formal = []string{f.formalPredecessor}
			}
		}
		for i := range a.Tasks {
			if a.Tasks[i].Original.Local == nil {
				a.Tasks[i].Original.Local = []string{}
			}
			if a.Tasks[i].Original.Formal == nil {
				a.Tasks[i].Original.Formal = []string{}
			}
			if a.Tasks[i].Proposed.Local == nil {
				a.Tasks[i].Proposed.Local = []string{}
			}
			if a.Tasks[i].Proposed.Formal == nil {
				a.Tasks[i].Proposed.Formal = []string{}
			}
		}
		result = a
	}
	raw, err := json.Marshal(result)
	require(f.t, err)
	f.run.ContentResult = &runnercontract.ContentResult{Status: "succeeded", JSON: raw, RepositorySource: f.run.RepositorySource, FileMaterials: f.run.Materials,
		Source: runnercontract.Final{Verified: true, NativeOutcome: "succeeded", CommandID: q.RequestID, TurnID: "controlled-turn"}}
	f.run.Final = new(f.run.ContentResult.Source)
	if f.runs == nil {
		f.runs = map[string]runnercontract.Run{}
		f.requests = map[string]runnercontract.Request{}
	}
	if f.holdDocuments && r.Purpose == "documents" {
		f.run.Status, f.run.StopStatus = "running", "active"
	}
	f.run.Execution.AttemptID = "attempt-" + q.RequestID
	f.run.Execution.SessionID = "session-" + q.RequestID
	f.runs[f.run.RunID] = f.run
	f.requests[f.run.RunID] = q
	return runnerclient.Receipt{RunID: f.run.RunID, CommandID: q.RequestID, ReceiptStatus: "accepted"}, nil
}
func (f *promotionRunner) Inspect(_ context.Context, selector, id string) (runnerclient.Inspection, error) {
	r, ok := f.runs[id]
	if !ok {
		return runnerclient.Inspection{}, nil
	}
	return runnerclient.Inspection{Run: &r, Command: &runnerclient.Command{Command: "submit", RequestID: f.requests[id].RequestID, RunID: &r.RunID}}, nil
}
func (f *promotionRunner) Call(context.Context, string, []string, *runnercontract.Request) (runnerclient.Response, error) {
	return runnerclient.Response{}, nil
}

func promotionCatalog(t *testing.T) *runnerclient.Catalog {
	t.Helper()
	raw, err := os.ReadFile("../runnerclient/testdata/content-capabilities.json")
	require(t, err)
	var caps runnerclient.Capabilities
	require(t, json.Unmarshal(raw, &caps))
	raw, err = os.ReadFile("../runnerclient/testdata/generate-repository-capability.json")
	require(t, err)
	var extra struct {
		Generate *runnerclient.GenerateCapability `json:"generate"`
		Input    json.RawMessage                  `json:"generate_input"`
	}
	require(t, json.Unmarshal(raw, &extra))
	caps.Generate = extra.Generate
	caps.ConfigurationSchemas.GenerateInput = extra.Input
	catalog, err := runnerclient.CompileCatalog(caps)
	require(t, err)
	return catalog
}
func configurePromotionGeneration(t *testing.T, port *roadmapPort, live bool) {
	t.Helper()
	catalog := promotionCatalog(t)
	var runner roadmapservice.Runner = &promotionRunner{t: t, port: port}
	build := runnercontract.BuildPin{}
	execution := runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"read_only","model":"fixture"}`)}
	if live {
		exe := os.Getenv("STEERRUNNER_EXE")
		if exe == "" {
			t.Fatal("STEERRUNNER_EXE required")
		}
		state := filepath.Join(t.TempDir(), "runner")
		cli := runnerclient.Client{Executable: exe, StateDir: state, Timeout: 45 * time.Second}
		binary, err := os.ReadFile(exe)
		require(t, err)
		cli.SHA256 = roadmap.Digest(binary)
		// Public CLI only; never open Runner state or native logs.
		version, err := cli.Version(context.Background())
		require(t, err)
		info, err := cli.StartService(context.Background(), version)
		if err != nil {
			if info.InstanceID != "" {
				_ = cli.StopService(context.Background(), info.InstanceID, roadmapservice.Token())
			}
			require(t, err)
		}
		cli.InstanceID = info.InstanceID
		cli.SHA256 = info.ExecutableSHA256
		cli.BuildVersion = version.Version
		cli.ResourceDigest = version.ResourceDigest
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()

			req := roadmapservice.Token()
			require(t, cli.StopService(ctx, info.InstanceID, req))
			require(t, cli.VerifyServiceSettlement(ctx, info.InstanceID, req))
			promotionEvidence(t, "runner-settlement", map[string]any{"instance_id": info.InstanceID, "request_id": req, "resources_settled": true, "build": build})
		})
		catalog, err = cli.Catalog(context.Background(), version)
		require(t, err)
		promotionEvidence(t, "runner-version", version)
		promotionEvidence(t, "runner-capabilities", catalog.Capabilities)
		promotionEvidence(t, "runner-build", cli.BuildPin())
		cli = cli.WithCatalog(catalog)
		runner, build = cli, cli.BuildPin()
		codex := os.Getenv("STEERLINE_ROADMAP_CODEX")
		if codex == "" {
			codex = "codex.exe"
		}
		body, _ := json.Marshal(map[string]string{"access_mode": "read_only", "model": "gpt-6.1-sol", "reasoning_effort": "low", "executable": codex})
		execution.Settings = body

	}
	port.config.runtimeCatalog = catalog
	port.service = &roadmapservice.Service{Project: port.config.Identity(), Repository: port.config.Repository, Store: port.store, Runner: runner, Build: build, Queue: runnerbinding.NewContentQueue(5),
		Source: port.source, Graph: port.graph, PrepareDocuments: port.documentContext, ReadCommitted: port.materials.readCommittedMaterial,
		ReadCommittedFiles: port.config.committedDocuments().readCommittedMaterials,
		Prepare: func(context.Context) (runnercontract.ExecutionInput, *runnerclient.Catalog, error) {
			return execution, catalog, nil
		}}
}
func promotionEvidence(t *testing.T, name string, value any) {
	t.Helper()
	root := os.Getenv("STEERLINE_ROADMAP_EVIDENCE")
	if root == "" {
		return
	}
	require(t, os.MkdirAll(root, 0700))
	raw, err := json.MarshalIndent(value, "", "  ")
	require(t, err)
	require(t, os.WriteFile(filepath.Join(root, name+".json"), raw, 0600))
}
func promotionAction(t *testing.T, p *roadmapPort, action, target, request string, version uint64, payload any) roadmap.Record {
	t.Helper()
	raw, _ := json.Marshal(payload)
	value, err := p.action(context.Background(), httpcontract.CommandEnvelope{Action: action, TargetID: target, RequestID: request, ExpectedVersion: version, Payload: raw})
	require(t, err)
	r, ok := value.(roadmap.Record)
	if !ok {
		t.Fatalf("unexpected response %T", value)
	}
	promotionEvidence(t, action, r)
	for _, v := range r.Executions() {
		promotionEvidence(t, action+"-runner-request-"+v.GenerationID, v.Request)
	}
	return r
}
func awaitPromotion(t *testing.T, p *roadmapPort, r roadmap.Record, want string) roadmap.Record {
	t.Helper()
	deadline := time.Now().Add(10 * time.Minute)
	overlapped := false
	for time.Now().Before(deadline) {
		var err error
		r, err = p.service.Observe(context.Background(), r.ID)
		require(t, err)
		sessions := map[string]bool{}
		for _, v := range r.Executions() {
			if v.Projection != nil && v.Projection.Execution.TurnStarted && v.Projection.Status == "running" && v.Projection.StopStatus != "stopped" && v.Projection.Execution.SessionID != "" {
				sessions[v.Projection.Execution.SessionID] = true
			}
		}
		if len(sessions) > 1 && !overlapped {
			overlapped = true
			promotionEvidence(t, "concurrent-native-sessions", map[string]any{"observed_at": time.Now().UTC(), "record": roadmap.StoreValue(r)})
		}
		if r.State == want {
			promotionEvidence(t, want, r)
			promotionEvidence(t, want+"-runner-observation", roadmap.StoreValue(r))
			if os.Getenv("STEERRUNNER_EXE") != "" && want == "documents-review" && !overlapped {
				t.Fatal("native sessions were not observed simultaneously running")
			}
			if cli, ok := p.service.Runner.(runnerclient.Client); ok && want == "documents-review" {
				for _, v := range r.Executions() {
					cursor := ""
					events := []runnercontract.Event{}
					for page := 0; page < 100; page++ {
						observation, err := cli.Observe(context.Background(), v.RunID, cursor)
						require(t, err)
						events = append(events, observation.EventPage.Events...)
						if !observation.EventPage.HasMore {
							break
						}
						cursor = observation.EventPage.NextCursor
						if page == 99 {
							t.Fatal("native event evidence truncated")
						}
					}
					promotionEvidence(t, "events-"+v.GenerationID, events)
				}
			}
			return r
		}
		if r.State == "failed" || r.State == "invalid" || r.State == "blocked" || r.State == "documents-blocked" || r.State == "rejected" {
			promotionEvidence(t, "failed-runner-observation", roadmap.StoreValue(r))
			t.Fatalf("generation %s: %s; questions=%v", r.State, r.Diagnostic, r.Questions)
		}
		time.Sleep(time.Second)
	}
	promotionEvidence(t, "timeout-runner-observation", roadmap.StoreValue(r))
	t.Fatal("content generation timeout")
	return r
}

func instructionSource(in runnercontract.GenerateRunInput) string {
	for _, m := range in.Generate.Materials {
		if m.Path == roadmapservice.SourceInputPath && m.Content != nil {
			return *m.Content
		}
	}
	return ""
}
