package controlled

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	em "github.com/erranliu/SteerLine/internal/execution/model"
	ref "github.com/erranliu/SteerLine/internal/refactoring/model"
	rc "github.com/erranliu/SteerLine/internal/runnercontract"
)

func (s *RunnerFixture) submitRefactoring(stateDir string, request rc.Request) (any, string, error) {
	var in rc.RunInput
	if e := json.Unmarshal(request.Input, &in); e != nil {
		return nil, "", e
	}
	id := "run-" + request.RequestID
	now := time.Now().UTC()
	run := rc.Run{RunID: id, Revision: 2, Workflow: in.Workflow, Correlation: in.Correlation, InputDigest: ref.Digest(request.Input), Status: "completed", StopStatus: "stopped", Closed: true, UpdatedAt: now, Final: &rc.Final{Verified: true, NativeOutcome: "succeeded", CommandID: "command-" + id, AttemptID: "attempt-" + id, SessionID: "session-" + id, TurnID: "turn-" + id}}
	run.Execution = rc.Execution{AttemptID: "attempt-" + id, SessionID: "session-" + id, TurnID: "turn-" + id}
	const reportBody = "受控审计报告：原收益保持。冲突审计：双方意图与验证已记录。参考费用未覆盖范围已披露。"
	type contextFacts struct {
		Mode         string
		Requirements string
		Answers      []ref.Answer
		PR           *rc.PRFact
		Report       string
		Feedback     []struct {
			Purpose string
			Result  *ref.Result
		}
	}
	var facts contextFacts
	purpose := strings.TrimPrefix(in.Title, "重构与维护 · ")
	providerPath := statePathForDirectory(stateDir)
	if in.Workflow.ID == "generate" {
		var g rc.GenerateRunInput
		_ = json.Unmarshal(request.Input, &g)
		if purpose == "report" && len(g.Generate.Materials) != 2 || purpose != "report" && len(g.Generate.Materials) != 3 {
			return nil, "invalid_materials", nil
		}
		_ = json.Unmarshal([]byte(*g.Generate.Materials[1].Content), &facts)
		decision := map[string]any{"kind": "execute", "prompt": "实施同方向的有限切片并验证。", "summary": "受控决策，非真实模型判断"}
		switch facts.Mode {
		case "audit":
			if len(facts.Feedback) > 0 && facts.Feedback[0].Result != nil && facts.Feedback[0].Result.Outcome == "blocked" {
				decision["prompt"] = "修复原工作区失败切片并验证。"
			} else {
				decision["kind"] = "accept"
				delete(decision, "prompt")
			}
		case "first":
			if strings.Contains(facts.Requirements, "ask-once") && len(facts.Answers) == 0 {
				decision["kind"] = "ask"
				delete(decision, "prompt")
				decision["question"] = ref.QuestionBody{Prompt: "是否保留公共接口？", Facts: []string{"当前接口有调用方"}, Attempts: []string{}, Options: []ref.QuestionOption{{ID: "keep", Label: "保留公共接口", Impact: "维持兼容"}}, RecommendedOptionID: "keep", RecommendationReason: "兼容现有调用"}
			}
		case "report":
			decision = map[string]any{"kind": "report", "report": reportBody}
			if facts.PR != nil {
				decision["report"] = reportBody + "\r\n" + facts.PR.HeadRevision + "  "
				decision["prompt"] = "按随附版本核对原 PR，将随附正文原样同步。"
			}
		}
		if g.Generate.Repository != nil || (purpose == "first") != (g.Generate.Directory != nil) {
			return nil, "unexpected_source", nil
		}
		run.Execution.Workspace = filepath.Join(stateDir, "runs", id, "content")
		if g.Generate.Directory != nil {
			run.Execution.Workspace = g.Generate.Directory.Path
			head, e := Git(context.Background(), run.Execution.Workspace, "rev-parse", "HEAD")
			if e != nil || head == "" {
				return nil, "invalid_investigation", e
			}
		}
		for _, doc := range g.Generate.Materials {
			path := filepath.Join(stateDir, "materials", id, doc.Path)
			if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
				return nil, "", e
			}
			if e := os.WriteFile(path, []byte(*doc.Content), 0600); e != nil {
				return nil, "", e
			}
			run.Materials = append(run.Materials, rc.Material{Kind: "file", Path: doc.Path, Digest: ref.Digest([]byte(*doc.Content)), SnapshotPath: path, ExecutionLocation: path})
		}
		raw, _ := json.Marshal(decision)
		run.ContentResult = &rc.ContentResult{Status: "succeeded", JSON: raw, Source: *run.Final, FileMaterials: run.Materials, RepositorySource: run.RepositorySource}
	} else {
		providerPath = statePathForDirectory(in.Repository.LocalPath)
		provider, e := Read(providerPath)
		if e != nil {
			return nil, "", e
		}
		worktree := filepath.Join(stateDir, "maintenance", in.Correlation["steerline_refactoring"])
		branch := "codex/maintenance/" + in.Correlation["steerline_refactoring"]
		run.WorkspaceOriginRunID = id
		if in.ReuseWorktreeFromRunID != "" {
			source, ok := s.Runs[in.ReuseWorktreeFromRunID]
			if !ok || source.StopStatus != "stopped" || source.Correlation["steerline_refactoring"] != in.Correlation["steerline_refactoring"] {
				return nil, "workspace_identity_conflict", nil
			}
			run.WorkspaceOriginRunID = source.WorkspaceOriginRunID
			for _, other := range s.Runs {
				if other.ReuseWorktreeFromRunID == source.RunID {
					return nil, "workspace_already_reused", nil
				}
			}
		} else {
			if _, e = Git(context.Background(), provider.LocalRepository, "worktree", "add", "-b", branch, worktree, in.Repository.BaseRevision); e != nil {
				return nil, "", e
			}
		}
		run.ReuseWorktreeFromRunID = in.ReuseWorktreeFromRunID
		run.WorkspaceIdentity = "identity-" + run.WorkspaceOriginRunID
		run.Execution.Workspace = worktree
		run.Execution.Branch = branch
		initial, e := base64.StdEncoding.DecodeString(in.ResultFile.InitialContentBase64)
		if e != nil {
			return nil, "", e
		}
		var seed struct {
			ActionID    string          `json:"action_id"`
			OperationID string          `json:"operation_id"`
			Context     json.RawMessage `json:"context"`
		}
		if e = json.Unmarshal(initial, &seed); e != nil {
			return nil, "", e
		}
		_ = json.Unmarshal(seed.Context, &facts)
		value := ref.Result{ActionID: seed.ActionID, OperationID: seed.OperationID, Outcome: "done", Summary: "受控执行"}
		if purpose == "implement" || purpose == "repair" || purpose == "conflict" {
			if e = os.WriteFile(filepath.Join(worktree, "maintenance-result.txt"), []byte(id+"\n"), 0600); e != nil {
				return nil, "", e
			}
		}
		if purpose == "implement" && in.ReuseWorktreeFromRunID == "" {
			value.Outcome = "blocked"
			run.Final.NativeOutcome = "failed"
			run.Status = "failed"
			run.Diagnostic = "scripted first-round failure with dirty worktree"
		}
		if purpose == "publish" || purpose == "conflict" {
			for _, args := range [][]string{{"add", "--", "maintenance-result.txt"}, {"commit", "--quiet", "-m", "Controlled maintenance delivery"}} {
				if _, e = Git(context.Background(), worktree, args...); e != nil {
					return nil, "", e
				}
			}
			head, e := Git(context.Background(), worktree, "rev-parse", "HEAD")
			if e != nil {
				return nil, "", e
			}
			value.Head = head
			value.Base = in.Repository.BaseRevision
			if _, e = Git(context.Background(), worktree, "push", provider.GitURL, head+":refs/heads/"+branch); e != nil {
				return nil, "", e
			}
			url := fmt.Sprintf("https://github.com/%s/pull/%d", provider.RepositoryName(), len(provider.PRs)+1)
			if facts.PR != nil {
				url = facts.PR.URL
			}
			pr := em.PRInfo{URL: url, Repository: provider.RepositoryName(), HeadRepository: provider.RepositoryName(), BaseBranch: provider.BranchName(), BaseRevision: value.Base, HeadBranch: branch, HeadRevision: head, State: "open"}
			if provider.PRs == nil {
				provider.PRs = map[string]em.PRInfo{}
			}
			provider.PRs[url] = pr
			provider.PR = pr
			if e = Write(providerPath, provider); e != nil {
				return nil, "", e
			}
			value.PR = url
		}
		if purpose == "sync" {
			if facts.PR == nil || facts.Report == "" {
				return nil, "missing_sync_binding", nil
			}
			pr := provider.PRs[facts.PR.URL]
			if pr.HeadRevision != facts.PR.HeadRevision || pr.BaseRevision != facts.PR.BaseRevision {
				return nil, "stale_sync_binding", nil
			}
			pr.Body = facts.Report
			provider.PRs[pr.URL], provider.PR = pr, pr
			if e = Write(providerPath, provider); e != nil {
				return nil, "", e
			}
			value.PR, value.Head, value.Base = pr.URL, pr.HeadRevision, pr.BaseRevision
		}

		raw, _ := json.Marshal(value)
		encoded := base64.StdEncoding.EncodeToString(raw)
		if s.Results == nil {
			s.Results = map[string]rc.ResultFile{}
		}
		s.Results[id] = rc.ResultFile{Contract: rc.ResultContract, RunID: id, Version: 1, Encoding: "base64", ObservedAt: &now, ContentBase64: &encoded, SHA256: ref.Digest(raw), SizeBytes: len(raw), CollectionStatus: "closed"}
	}
	s.Runs[id] = run
	s.Providers[id] = providerPath
	s.Bodies[request.RequestID] = string(request.Input)
	cmd := rc.Command{CommandID: "command-" + request.RequestID, RequestID: request.RequestID, RunID: &id, Command: "submit", ReceiptStatus: "accepted", ActionStatus: "completed"}
	s.Requests[request.RequestID] = cmd
	return map[string]any{"command_id": cmd.CommandID, "run_id": id, "receipt_status": "accepted", "action_status": "completed"}, "", nil
}
