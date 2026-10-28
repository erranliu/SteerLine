package controlled

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	model "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	protocol "github.com/erranliu/SteerLine/internal/testkit/protocol"
)

// RunnerFixture is a scripted public CLI peer. It owns no agent, transport,
// workflow or native recovery state. Calls are evidence of caller side effects.
type RunnerFixture struct {
	WaitEvents []runnercontract.Event                 `json:"wait_events,omitempty"`
	Progress   map[string]runnercontract.ProgressFile `json:"progress"`
	Results    map[string]runnercontract.ResultFile   `json:"results"`
	Providers  map[string]string                      `json:"providers"`
	Runs       map[string]runnercontract.Run          `json:"runs"`
	Requests   map[string]runnercontract.Command      `json:"requests"`
	Bodies     map[string]string                      `json:"bodies"`
	Calls      []string                               `json:"calls"`
}

func RunnerCLI(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) == 3 && args[0] == "capabilities" && args[1] == "--offline" && args[2] == "--json" {
		if path := os.Getenv("STEERLINE_TEST_WEB_OFFLINE_CAPABILITIES"); path != "" {
			raw, err := os.ReadFile(path)
			if err != nil {
				return true, err
			}
			var data map[string]any
			if err = json.Unmarshal(raw, &data); err != nil {
				return true, err
			}
			data["version"] = "caller-fixture-v1"
			data["resource_digest"] = strings.Repeat("a", 64)
			return true, json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "protocol": "steerrunner-cli/v1", "data": data})
		}
	}
	if args[0] == "capabilities" {
		exe, err := os.Executable()
		if err != nil {
			return true, err
		}
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(exe)), "seed", "runner-content-capabilities.json"))
		if err == nil {
			var data map[string]any
			if err = json.Unmarshal(raw, &data); err != nil {
				return true, err
			}
			data["version"] = "caller-fixture-v1"
			data["resource_version"] = 1
			advertiseRepository(data)
			advertiseResult(data)
			data["resource_digest"] = strings.Repeat("a", 64)
			return true, json.NewEncoder(os.Stdout).Encode(map[string]any{"protocol": "steerrunner-cli/v1", "ok": true, "request_id": nil, "data": data, "error": nil})
		}
		if !os.IsNotExist(err) {
			return true, err
		}
		return true, json.NewEncoder(os.Stdout).Encode(map[string]any{"protocol": "steerrunner-cli/v1", "ok": true, "request_id": nil, "data": controlledCapabilities(), "error": nil})
	}
	switch args[0] {
	case "version", "capabilities", "service":
		os.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "caller-fixture-v1")
		return protocol.RunRunnerServiceFixture(), nil
	case "progress-file", "result-file", "submit", "inspect", "open-native-session", "events", "observe", "reply", "resume", "stop":
	default:
		return false, nil
	}
	stateDir := argument(args, "--state-dir")
	if !filepath.IsAbs(stateDir) {
		return true, fmt.Errorf("fixture requires an owned absolute state-dir")
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return true, err
	}
	lockPath := filepath.Join(stateDir, "caller-fixture.lock")
	deadline := time.Now().Add(10 * time.Second)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			f.Close()
			break
		}
		if !os.IsExist(err) || time.Now().After(deadline) {
			return true, err
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer os.Remove(lockPath)
	path := filepath.Join(stateDir, "caller-fixture.json")
	s := RunnerFixture{Runs: map[string]runnercontract.Run{}, Requests: map[string]runnercontract.Command{}, Bodies: map[string]string{}}
	if raw, err := os.ReadFile(path); err == nil {
		if err = json.Unmarshal(raw, &s); err != nil {
			return true, err
		}
	}
	if s.Providers == nil {
		s.Providers = map[string]string{}
	}
	var request runnercontract.Request
	var id any
	if args[0] != "progress-file" && args[0] != "result-file" && args[0] != "inspect" && args[0] != "events" && args[0] != "observe" && args[0] != "open-native-session" {
		if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
			return true, err
		}
		id = request.RequestID
	}
	respond := func(data any, code string) (bool, error) {
		if err := Write(path, s); err != nil {
			return true, err
		}
		if code == "" && request.RequestID != "" {
			providerPath := statePathForDirectory(stateDir)
			if cmd, ok := s.Requests[request.RequestID]; ok && cmd.RunID != nil && s.Providers[*cmd.RunID] != "" {
				providerPath = s.Providers[*cmd.RunID]
			}
			provider, err := Read(providerPath)
			if err != nil {
				return true, err
			}
			if provider.Requests == nil {
				provider.Requests = map[string]string{}
			}
			provider.Requests[request.RequestID] = args[0]
			if err := Write(providerPath, provider); err != nil {
				return true, err
			}
		}
		var failure any
		if code != "" {
			failure = map[string]string{"code": code, "message": code}
		}
		err := json.NewEncoder(os.Stdout).Encode(map[string]any{"protocol": "steerrunner-cli/v1", "ok": code == "", "request_id": id, "data": data, "error": failure})
		if err != nil {
			return true, err
		}
		if code != "" {
			os.Remove(lockPath)
			os.Exit(5)
		}
		return true, nil
	}
	s.Calls = append(s.Calls, strings.Join(args[:1], " "))
	runID := argument(args, "--run-id")
	if args[0] == "result-file" {
		result, ok := s.Results[runID]
		if !ok {
			return respond(nil, "run_not_found")
		}
		return respond(result, "")
	}
	if args[0] == "progress-file" {
		p, ok := s.Progress[runID]
		if !ok {
			return respond(nil, "run_not_found")
		}
		if raw, err := os.ReadFile(filepath.Join(stateDir, runID+"-progress.json")); err == nil {
			if json.Unmarshal(raw, &p) != nil {
				return respond(nil, "invalid_fixture")
			}
		}
		if s.Runs[runID].StopStatus == "stopped" {
			p.CollectionStatus = "closed"
		}
		return respond(p, "")
	}
	if args[0] == "submit" {
		if previous, ok := s.Requests[request.RequestID]; ok {
			if s.Bodies[request.RequestID] != string(request.Input) {
				return respond(nil, "request_conflict")
			}
			return respond(map[string]any{"command_id": previous.CommandID, "run_id": *previous.RunID, "receipt_status": "accepted", "action_status": "completed"}, "")
		}
		var input runnercontract.RunInput
		if err := json.Unmarshal(request.Input, &input); err != nil {
			return true, err
		}
		directory := input.Repository.LocalPath
		if directory == "" {
			directory = stateDir
		}
		providerPath := statePathForDirectory(directory)
		provider, err := Read(providerPath)
		if err != nil {
			return true, err
		}
		if !filepath.IsAbs(provider.LocalRepository) {
			return true, fmt.Errorf("caller fixture repository missing")
		}
		input.Repository.LocalPath = provider.LocalRepository
		runID = "run-" + request.RequestID
		sum := sha256.Sum256(request.Input)
		run := runnercontract.Run{Correlation: input.Correlation, RunID: runID, Revision: 1, Title: input.Title, InputDigest: hex.EncodeToString(sum[:]), Status: "running", Phase: "running", StopStatus: "active", NewInputsAllowed: true, Controls: runnercontract.Controls{CanStop: true}, UpdatedAt: time.Now().UTC(), Execution: runnercontract.Execution{AttemptID: "attempt-" + runID, Generation: "fixture", TurnID: "fixture-turn", SessionID: "session-" + runID, Workspace: input.Repository.LocalPath, Branch: "codex/fixture/" + request.RequestID}}
		run.Workflow = input.Workflow
		if input.Workflow.ID == "generate" {
			var generation runnercontract.GenerateRunInput
			if err := json.Unmarshal(request.Input, &generation); err != nil {
				return true, err
			}
			// Scripted public result, not a native/model success claim.
			selection := []string{}
			noTests := false
			materials := generation.Generate.Materials
			if generation.Generate.Repository != nil {
				var e error
				materials, run.RepositorySource, e = fixtureRepository(*generation.Generate.Repository)
				if e != nil {
					return true, e
				}
			}
			for _, material := range materials {
				if material.Content == nil {
					continue
				}
				if strings.HasSuffix(material.Path, ".plan.md") && (strings.Contains(strings.ToLower(*material.Content), "no automated tests") || strings.Contains(*material.Content, "无需自动测试")) {
					noTests = true
				}
				if material.Path == "SteerlineFlow/testing/test-list.json" {
					var catalog struct {
						Tests []struct {
							ID string `json:"id"`
						} `json:"tests"`
					}
					if e := json.Unmarshal([]byte(*material.Content), &catalog); e != nil {
						return true, e
					}
					for _, d := range catalog.Tests {
						selection = append(selection, d.ID)
					}
				}
			}
			if noTests {
				selection = []string{}
			}
			result, _ := json.Marshal(map[string]any{"tests": selection})
			source := runnercontract.Final{CommandID: "fixture-content-" + request.RequestID, TurnID: "fixture-turn", Verified: true, NativeOutcome: "succeeded", Source: "caller-fixture"}
			run.ContentResult = &runnercontract.ContentResult{Status: "succeeded", JSON: result, RepositorySource: run.RepositorySource, Source: source}
			run.Status = "completed"
			run.StopStatus = "stopped"
			run.Final = &source
			run.NewInputsAllowed = false
		}
		if input.ProgressFile != nil {
			body, err := base64.StdEncoding.Strict().DecodeString(input.ProgressFile.InitialContentBase64)
			if err != nil {
				return true, err
			}
			hash := sha256.Sum256(body)
			now := time.Now().UTC()
			if s.Progress == nil {
				s.Progress = map[string]runnercontract.ProgressFile{}
			}
			encoded := input.ProgressFile.InitialContentBase64
			s.Progress[runID] = runnercontract.ProgressFile{Contract: runnercontract.ProgressContract, RunID: runID, Version: 1, ObservedAt: &now, Encoding: "base64", ContentBase64: &encoded, SizeBytes: len(body), SHA256: hex.EncodeToString(hash[:]), CollectionStatus: "observing"}
		}
		if input.Correlation["environment_activity"] != "" && input.ResultFile != nil {
			result, err := scriptedEnvironment(input, &run)
			if err != nil {
				return true, err
			}
			if s.Results == nil {
				s.Results = map[string]runnercontract.ResultFile{}
			}
			s.Results[runID] = result
		}
		s.Runs[runID] = run
		s.Providers[runID] = providerPath
		cmd := runnercontract.Command{CommandID: "command-" + request.RequestID, RequestID: request.RequestID, RunID: &runID, Command: "submit", ReceiptStatus: "accepted", ActionStatus: "completed"}
		s.Requests[request.RequestID] = cmd
		s.Bodies[request.RequestID] = string(request.Input)
		return respond(map[string]any{"command_id": cmd.CommandID, "run_id": runID, "receipt_status": "accepted", "action_status": "completed"}, "")
	}
	if args[0] == "inspect" || args[0] == "observe" {
		var cmd *runnercontract.Command
		if requestID := argument(args, "--request-id"); requestID != "" {
			v, ok := s.Requests[requestID]
			if !ok {
				return respond(nil, "request_not_found")
			}
			cmd = &v
			runID = *v.RunID
		}
		run, ok := s.Runs[runID]
		if !ok {
			return respond(nil, "run_not_found")
		}
		providerPath := s.Providers[runID]
		if providerPath == "" {
			providerPath = statePathForDirectory(stateDir)
		}
		provider, err := Read(providerPath)
		if err != nil {
			return true, err
		}
		beforeWait, _ := json.Marshal(run.ExecutionWait)
		run.ExecutionWait = provider.PreparationWait.Clone()
		if run.StopStatus == "stopped" {
			run.ExecutionWait = nil
		}
		afterWait, _ := json.Marshal(run.ExecutionWait)
		if string(beforeWait) != string(afterWait) {
			run.Revision++
			run.Status, run.Phase, run.ResumeRequired = "running", "running", false
			run.Controls.CanResume = false
			if run.StopStatus == "stopped" {
				run.Status, run.Phase = "stopped", "stopped"
			}
			if run.ExecutionWait != nil {
				run.Status, run.Step, run.Phase = "queued", "implement.execute", "execution_wait"
				if run.ExecutionWait.Stage == "exhausted" {
					run.Status, run.Phase, run.ResumeRequired, run.Controls.CanResume = "blocked", "operation_rejected", true, true
				}
			}
			s.WaitEvents = append(s.WaitEvents, runnercontract.Event{EventID: fmt.Sprintf("wait-%s-%d", runID, run.Revision), RunID: runID, Revision: run.Revision, Type: "execution_wait_changed", ExecutionWait: afterWait, AttemptID: run.Execution.AttemptID, CreatedAt: time.Now().UTC()})
		}
		if run.StopStatus != "stopped" && !provider.Hold {
			if run.PR == nil {
				pr, err := prepareFixturePR(providerPath, &provider, run.Execution.Workspace, run.Execution.Branch)
				if err != nil {
					return true, err
				}
				b, _ := json.Marshal(pr)
				var fact runnercontract.PRFact
				json.Unmarshal(b, &fact)
				fact.Provider = "command"
				fact.Number, _ = strconv.Atoi(pr.URL[strings.LastIndex(pr.URL, "/")+1:])
				fact.CheckedAt = time.Now().UTC()
				run.PR = &fact
				run.Status = "pr_ready"
				run.Step = "await_merge"
				run.Phase = "pr_ready"
				run.Controls.CanReply = true
				if provider.WebFeedback {
					raw, _ := json.Marshal(map[string]any{"id": "question-" + runID, "text": "修复发布或 PR 元数据", "allow_free_text": true, "options": []any{map[string]string{"id": "publish_fix", "label": "修复发布或 PR 元数据"}}})
					json.Unmarshal(raw, &run.Question)
				}
				run.Revision++
			}
			if hasArgument(args, "--refresh-pr") {
				if err := recordProviderQuery(providerPath); err != nil {
					return true, err
				}
				pr, err := ProviderPR(provider, run.PR.URL)
				if err != nil {
					return true, err
				}
				b, _ := json.Marshal(pr)
				var fact runnercontract.PRFact
				json.Unmarshal(b, &fact)
				fact.Provider = "command"
				fact.Number, _ = strconv.Atoi(pr.URL[strings.LastIndex(pr.URL, "/")+1:])
				fact.CheckedAt = time.Now().UTC()
				run.PR = &fact
			}
		}
		s.Runs[runID] = run
		if args[0] == "observe" {
			return respond(map[string]any{"contract": "steerrunner-run-observation/v1", "run": run, "event_page": s.eventPage(run, argument(args, "--after"))}, "")
		}
		return respond(runnercontract.Inspection{Command: cmd, Run: &run}, "")
	}
	run, ok := s.Runs[runID]
	if !ok {
		return respond(nil, "run_not_found")
	}
	if args[0] == "open-native-session" {
		if argument(args, "--attempt-id") != run.Execution.AttemptID || argument(args, "--session-id") != run.Execution.SessionID || argument(args, "--generation") != run.Execution.Generation {
			return respond(nil, "stale_revision")
		}
		// Caller-only fixture: records the public opening request without
		// owning native resources or starting an interactive agent.
		return respond(map[string]any{"run_id": runID, "attempt_id": run.Execution.AttemptID, "session_id": run.Execution.SessionID, "generation": run.Execution.Generation, "opened": true}, "")
	}
	if args[0] == "events" {
		return respond(s.eventPage(run, argument(args, "--after")), "")
	}
	if previous, ok := s.Requests[request.RequestID]; ok {
		if s.Bodies[request.RequestID] != string(request.Input) {
			return respond(nil, "request_conflict")
		}
		return respond(map[string]any{"command_id": previous.CommandID, "run_id": runID, "receipt_status": "accepted", "action_status": "completed"}, "")
	}
	var input struct {
		ExpectedRevision uint64 `json:"expected_revision"`
	}
	json.Unmarshal(request.Input, &input)
	if input.ExpectedRevision != run.Revision {
		return respond(nil, "stale_revision")
	}
	if args[0] == "reply" {
		run.Question = nil
		run.Revision++
	}
	if args[0] == "stop" {
		if run.ExecutionWait != nil {
			run.ExecutionWait = nil
			s.WaitEvents = append(s.WaitEvents, runnercontract.Event{EventID: fmt.Sprintf("wait-%s-%d", runID, run.Revision+1), RunID: runID, Revision: run.Revision + 1, Type: "execution_wait_changed", ExecutionWait: json.RawMessage("null"), AttemptID: run.Execution.AttemptID, CreatedAt: time.Now().UTC()})
		}
		run.Status, run.Phase = "stopped", "stopped"
		run.StopStatus = "stopped"
		run.NewInputsAllowed = false
		run.Controls = runnercontract.Controls{}
		run.Revision++
	}
	s.Runs[runID] = run
	cmd := runnercontract.Command{CommandID: "command-" + request.RequestID, RequestID: request.RequestID, RunID: &runID, Command: args[0], ReceiptStatus: "accepted", ActionStatus: "completed"}
	s.Requests[request.RequestID] = cmd
	s.Bodies[request.RequestID] = string(request.Input)
	return respond(map[string]any{"command_id": cmd.CommandID, "run_id": runID, "receipt_status": "accepted", "action_status": "completed"}, "")
}

func hasArgument(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

// Fixture preparation is one Git/PR outcome, not a simulated development flow.
func prepareFixturePR(path string, s *State, repo, branch string) (model.PRInfo, error) {
	if existing, ok := s.PRs["https://github.com/"+s.RepositoryName()+"/pull/1"]; ok && existing.HeadBranch == branch {
		return existing, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, args := range [][]string{{"checkout", "-B", branch}, {"commit", "--allow-empty", "-m", "Controlled Runner outcome"}, {"push", s.GitURL, "HEAD:refs/heads/" + branch}} {
		if _, err := Git(ctx, repo, args...); err != nil {
			return model.PRInfo{}, err
		}
	}
	head, err := Git(ctx, repo, "rev-parse", "HEAD")
	if err != nil {
		return model.PRInfo{}, err
	}
	pr := model.PRInfo{URL: fmt.Sprintf("https://github.com/%s/pull/%d", s.RepositoryName(), len(s.PRs)+1), Repository: s.RepositoryName(), HeadRepository: s.RepositoryName(), BaseBranch: s.BranchName(), HeadBranch: branch, HeadRevision: head, State: "open"}
	if s.PRs == nil {
		s.PRs = map[string]model.PRInfo{}
	}
	s.PR = pr
	s.PRs[pr.URL] = pr
	if err := Write(path, s); err != nil {
		return model.PRInfo{}, err
	}
	return pr, nil
}

// This peer already scripts generate results in submit/inspect. Advertise that
// public capability even when the helper is built directly rather than packaged.
func controlledCapabilities() map[string]any {
	caps := protocol.RunnerServiceCapabilities("caller-fixture-v1")
	caps["run_observation"] = map[string]any{"contract": "steerrunner-run-observation/v1", "same_snapshot": true, "max_events": 100}
	caps["configuration_schemas"].(map[string]any)["adapters"].(map[string]any)["orca-terminal"] = map[string]string{"type": "object"}
	caps["workflows"].([]any)[0].(map[string]any)["supported_adapters"] = []string{"codex-app-server", "orca-terminal"}
	development := map[string]any{}
	for key, value := range caps["workflows"].([]any)[0].(map[string]any) {
		development[key] = value
	}
	development["version"] = 2
	caps["workflows"] = append(caps["workflows"].([]any), development)
	caps["workflows"] = append(caps["workflows"].([]any), map[string]any{"id": "generate", "version": 1, "supported_adapters": []string{"codex-app-server"}, "prompt_steps": []string{}})
	caps["configuration_schemas"].(map[string]any)["generate_input"] = map[string]any{"type": "object", "required": []string{"workflow", "execution", "generate"}}
	advertiseRepository(caps)
	advertiseResult(caps)
	return caps
}

// eventPage reads the scripted events under the same fixture lock as the Run.
func (s RunnerFixture) eventPage(run runnercontract.Run, after string) runnercontract.EventPage {
	page := runnercontract.EventPage{NextCursor: "waiting", Events: []runnercontract.Event{}}
	if after != "fixture-end" && len(s.WaitEvents) != 0 {
		start := 0
		if strings.HasPrefix(after, "wait-page-") {
			start, _ = strconv.Atoi(strings.TrimPrefix(after, "wait-page-"))
		}
		for i, event := range s.WaitEvents {
			if i >= start && event.RunID == run.RunID {
				page.Events = append(page.Events, event)
			}
		}
		page.NextCursor = fmt.Sprintf("wait-page-%d", len(s.WaitEvents))
	}
	if after != "fixture-end" && run.PR != nil {
		for i := 1; i <= 3; i++ {
			f := runnercontract.Final{CommandID: fmt.Sprintf("final-%s-%d", run.RunID, i), AttemptID: run.Execution.AttemptID, SessionID: run.Execution.SessionID, TurnID: strconv.Itoa(i), NativeOutcome: "succeeded", Verified: true, Source: "fixture", ItemID: strconv.Itoa(i), Text: []string{"Controlled implementation result", "Controlled report result", "Controlled candidate pushed and PR created"}[i-1]}
			page.Events = append(page.Events, runnercontract.Event{EventID: fmt.Sprintf("event-%s-%d", run.RunID, i), RunID: run.RunID, Revision: run.Revision, Type: "final", CommandID: f.CommandID, AttemptID: f.AttemptID, Final: &f, CreatedAt: time.Now().UTC()})
		}
	}
	if run.PR != nil {
		page.NextCursor = "fixture-end"
	}
	return page
}

// Public repository snapshot behavior for this scripted caller peer only.
func fixtureRepository(repo runnercontract.GenerateRepository) ([]runnercontract.Document, *runnercontract.RepositorySource, error) {
	gitInput := func(input string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo.LocalPath}, args...)...)
		cmd.Stdin = strings.NewReader(input)
		return cmd.Output()
	}
	tree, err := gitInput("", "rev-parse", repo.Commit+"^{tree}")
	if err != nil {
		return nil, nil, err
	}
	source := &runnercontract.RepositorySource{LocalPath: repo.LocalPath, Commit: repo.Commit, Tree: strings.TrimSpace(string(tree))}
	entries, err := gitInput("", "ls-tree", "-r", "-z", repo.Commit)
	if err != nil {
		return nil, nil, err
	}
	type entry struct{ path, blob string }
	objects := []entry{}
	var batch strings.Builder
	for _, raw := range bytes.Split(entries, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		meta, path, ok := strings.Cut(string(raw), "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 || fields[0] != "100644" && fields[0] != "100755" {
			return nil, nil, fmt.Errorf("unsupported fixture source entry")
		}
		objects = append(objects, entry{path, fields[2]})
		batch.WriteString(fields[2] + "\n")
	}
	// Read all original objects in one process. Per-file Git processes held the
	// shared fixture lock long enough for another project's CLI call to expire.
	raw, err := gitInput(batch.String(), "cat-file", "--batch")
	if err != nil {
		return nil, nil, err
	}
	reader := bufio.NewReader(bytes.NewReader(raw))
	materials := []runnercontract.Document{}
	for _, object := range objects {
		header, err := reader.ReadString('\n')
		if err != nil {
			return nil, nil, err
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[0] != object.blob || fields[1] != "blob" {
			return nil, nil, fmt.Errorf("invalid fixture batch object")
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || size > len(raw) {
			return nil, nil, fmt.Errorf("invalid fixture batch size")
		}
		body := make([]byte, size)
		if _, err = io.ReadFull(reader, body); err != nil {
			return nil, nil, err
		}
		end, err := reader.ReadByte()
		if err != nil || end != '\n' {
			return nil, nil, fmt.Errorf("invalid fixture batch terminator")
		}
		hash := sha256.Sum256(body)
		source.Files = append(source.Files, runnercontract.RepositoryFile{Path: object.path, Blob: object.blob, SHA256: hex.EncodeToString(hash[:]), SizeBytes: len(body)})
		text := string(body)
		materials = append(materials, runnercontract.Document{Path: object.path, Content: &text, Kind: "inline"})
	}
	return materials, source, nil
}
func advertiseRepository(caps map[string]any) {
	caps["generate"] = map[string]any{"workflow": "generate/1", "result": "inspect/history: content_result.json", "source": "repository_source: commit/tree/files/blob/sha256/size_bytes", "max_result_utf8_bytes": 1048576, "output_schema": "caller_defined_json_schema_2020_12", "repository": map[string]any{"commit": "full_commit_object_id", "input_property": "generate.repository", "source": "existing_local_git_objects", "snapshot": "private_regular_files_without_git", "max_files": 4096, "max_bytes": 67108864, "max_manifest_bytes": 1048576}}
}
