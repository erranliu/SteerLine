package orcatransport

import (
	"context"
	"encoding/json"
	"errors"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeHost struct {
	mu                            sync.Mutex
	root, nativePath, incarnation string
	calls                         [][]string
	lines                         []string
	writeReply, unknown, clipped  bool
	screen                        bool
	unstarted, busy               bool
	draft                         string
	commandMode, launched         bool
	delayLaunchIdentity           bool
}

func (f *fakeHost) StartManaged(*exec.Cmd) (b.StopHandle, error) {
	return nil, errors.New("unexpected process start")
}
func (f *fakeHost) InspectProcess(*b.ProcessIdentity) string { return "unknown" }
func (f *fakeHost) NewProcessGeneration() string             { return "generation" }
func (f *fakeHost) WriteExecutionFile(string, []byte) error {
	return errors.New("terminal path has no file channel")
}
func (f *fakeHost) OutputCommand(_ context.Context, c b.LaunchCommand) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string(nil), c.Args...))
	arg := func(key string) string {
		for i, v := range c.Args {
			if v == key && i+1 < len(c.Args) {
				return c.Args[i+1]
			}
		}
		return ""
	}
	if len(c.Args) < 2 {
		return nil, errors.New("unexpected primitive")
	}
	path := f.root
	if f.nativePath != "" {
		path = f.nativePath
	}
	terminal := map[string]any{"handle": "terminal", "incarnationId": f.incarnation, "worktreeId": "repo::" + path, "worktreePath": path, "branch": "worker", "connected": true, "agentIdentity": "codex", "status": "running"}
	if f.commandMode && (!f.launched || f.delayLaunchIdentity) {
		terminal["agentIdentity"] = ""
	}
	var result any
	switch c.Args[0] + " " + c.Args[1] {
	case "worktree create":
		result = map[string]any{"worktree": map[string]any{"id": "repo::" + path, "repoId": "repo", "path": path, "branch": "worker", "head": arg("--base-branch"), "baseRef": arg("--base-branch"), "displayName": arg("--name")}, "agentTerminalHandle": "terminal"}
	case "terminal show":
		result = map[string]any{"terminal": terminal}
	case "terminal list":
		result = map[string]any{"terminals": []any{terminal}}
	case "terminal wait":
		result = map[string]any{"wait": map[string]any{"satisfied": !f.busy}}
	case "terminal read":
		if f.screen || strings.Contains(strings.Join(c.Args, " "), "--screen") {
			result = map[string]any{"terminal": map[string]any{"handle": "terminal", "source": "screen", "tail": f.lines, "oldestCursor": "0", "nextCursor": "0", "latestCursor": "0", "truncated": true, "limited": f.clipped, "draft": f.draft}}
			break
		}
		cursor := 0
		if arg("--cursor") != "" {
			cursor, _ = strconv.Atoi(arg("--cursor"))
		}
		if cursor > len(f.lines) {
			return nil, errors.New("cursor reset")
		}
		result = map[string]any{"terminal": map[string]any{"handle": "terminal", "source": "stream", "tail": f.lines[cursor:], "oldestCursor": "0", "nextCursor": strconv.Itoa(len(f.lines)), "latestCursor": strconv.Itoa(len(f.lines)), "truncated": f.clipped}}
	case "terminal send":
		if f.unknown {
			return nil, errors.New("lost response")
		}
		text := arg("--text")
		if f.commandMode && !f.launched {
			f.launched = true
			result = map[string]any{"send": map[string]any{"handle": "terminal", "accepted": true, "prompt": map[string]any{"requestId": arg("--retry-request"), "processIncarnation": f.incarnation}}}
			break
		}
		if strings.Contains(text, "SL_RESULT") || strings.Contains(text, "reply_b64") || strings.Contains(text, "nonce=") {
			return nil, errors.New("transport changed the worker prompt")
		}
		f.lines = append(f.lines, "› "+text)
		if f.writeReply {
			f.lines = append(f.lines, `• {"summary":"original result"}`, "  Worked for "+strconv.Itoa(len(f.lines))+"s • 18:50", "› Ask Codex to do anything")
		}

		stages := []string{"input_accepted", "turn_started"}
		if f.unstarted {
			stages = stages[:1]
		}
		result = map[string]any{"send": map[string]any{"handle": "terminal", "accepted": true, "prompt": map[string]any{"requestId": arg("--retry-request"), "processIncarnation": f.incarnation, "stages": stages}}}
	case "terminal close":
		result = map[string]any{"close": map[string]any{"handle": "terminal", "ptyKilled": true}}
	default:
		return nil, errors.New("unexpected resource or orchestration dependency")
	}
	return json.Marshal(map[string]any{"ok": true, "result": result, "_meta": map[string]any{"runtimeId": "runtime"}})
}

func TestAgentLaunchSingleTerminalAndReadonlyReconciliation(t *testing.T) {
	for _, environment := range []string{"", "paired-worker"} {
		t.Run(environment, func(t *testing.T) {
			l, f, s := fixtureOn(t, environment)
			l.setup.AgentLaunchCommand = "opaque-start --chosen-preferences"
			f.commandMode, f.delayLaunchIdentity = true, true
			r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			candidate, err := l.Next(context.Background(), s)
			if err != nil || candidate.Action != b.LifecycleCreateResource {
				t.Fatal(candidate, err)
			}
			req := request(s, candidate.Action, candidate.Input)
			r, _, err = l.Execute(context.Background(), req)
			if err != nil || r.Disposition != b.OperationAccepted || !r.State.ResourcePending {
				t.Fatal(r, err)
			}
			bind(&s, r)
			req.Snapshot = s
			if _, _, err = l.Execute(context.Background(), req); err == nil {
				t.Fatal("startup command repeated")
			}
			f.delayLaunchIdentity = false
			r, _, err = l.Reconcile(context.Background(), req)
			if err != nil || r.Disposition != b.OperationConfirmed {
				t.Fatal(r, err)
			}
			bind(&s, r)
			f.writeReply = true
			s, input := prepared(t, l, s, strings.Repeat("c", 32))
			r, observer, err := l.Execute(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
			if observer != nil {
				_ = observer.Close(context.Background())
			}
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			if r, _, err = settled(t, l, s); err != nil || r.TurnEvidence == nil {
				t.Fatal(r, err)
			}
			creates, launches, inputs := 0, 0, 0
			for _, call := range f.calls {
				if call[0] == "worktree" && call[1] == "create" {
					creates++
					if strings.Contains(strings.Join(call, " "), "--agent") {
						t.Fatal("native default agent also launched")
					}
				}
				if call[0] == "terminal" && call[1] == "create" {
					t.Fatal("second terminal created")
				}
				if call[0] == "terminal" && call[1] == "send" {
					if strings.Contains(strings.Join(call, " "), "opaque-start") {
						launches++
					} else {
						inputs++
					}
				}
			}
			if creates != 1 || launches != 1 || inputs != 1 {
				t.Fatal(creates, launches, inputs)
			}
		})
	}
}

func TestUnknownAgentLaunchNeverResendsOrAcceptsBusinessInput(t *testing.T) {
	l, f, s := fixture(t)
	l.setup.AgentLaunchCommand = "opaque-start"
	f.commandMode = true
	r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	candidate, _ := l.Next(context.Background(), s)
	req := request(s, candidate.Action, candidate.Input)
	f.unknown = true
	r, _, err = l.Execute(context.Background(), req)
	if err == nil || r.Disposition != b.OperationUnknown {
		t.Fatal(r, err)
	}
	bind(&s, r)
	req.Snapshot = s
	before := len(f.calls)
	_, _, _ = l.Reconcile(context.Background(), req)
	for _, call := range f.calls[before:] {
		if call[1] == "send" || call[1] == "create" {
			t.Fatal("unknown startup mutated resources", call)
		}
	}
	input := b.TurnInput{CommandID: strings.Repeat("c", 32), Prompt: "business"}
	if _, _, err = l.Execute(context.Background(), request(s, b.LifecyclePrepareSession, b.LifecycleInput{Turn: &input})); err == nil {
		t.Fatal("unknown launch allowed business input")
	}
}
func fixture(t *testing.T) (*Lifecycle, *fakeHost, b.LifecycleSnapshot) { return fixtureOn(t, "") }
func fixtureOn(t *testing.T, environment string) (*Lifecycle, *fakeHost, b.LifecycleSnapshot) {
	t.Helper()
	f := &fakeHost{root: t.TempDir(), incarnation: "original-incarnation"}
	if environment != "" {
		f.nativePath = "/srv/owned-worker"
	}
	p := Profile{Executable: "orca.exe", RepositorySelector: "id:repo", WaitTimeout: "1s", Environment: environment}
	raw, _ := json.Marshal(p)
	s := b.LifecycleSnapshot{Ref: b.ExecutionRef{Backend: b.BackendOrca, Protocol: b.ProtocolOrcaTerminalV3, Target: p.Target(), Agent: b.AgentSpec{Name: "codex"}, RunID: strings.Repeat("d", 32), AttemptID: strings.Repeat("b", 32)}, Session: b.SessionContext{ProjectID: "project", ExecutionID: strings.Repeat("a", 32), AttemptID: strings.Repeat("b", 32), WorkflowRunID: strings.Repeat("d", 32), TaskID: "1"}, ConfigurationDigest: "config", ResourceGeneration: "generation", WorkspacePlan: b.WorkspaceIntent{BaseCommit: strings.Repeat("e", 40)}}
	data, err := b.NewImplementationDataEnvelope(b.ImplementationDataEnvelope{CodecVersion: ProfileVersion, Purpose: b.ImplementationConfigPurpose, ImplementationKey: ProfileKey, ProjectID: "project", ProfileID: "profile", AcceptedConfigVersion: "accepted"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	port, err := NewLifecycle(b.LifecycleSetup{Snapshot: s, ProfileData: []b.ImplementationDataEnvelope{data}, ProfileID: "profile", AcceptedConfigVersion: "accepted", ProcessHost: f, RepositoryPath: f.root, RunDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return port.(*Lifecycle), f, s
}
func request(s b.LifecycleSnapshot, action b.LifecycleAction, input b.LifecycleInput) b.LifecycleRequest {
	ref := s.Ref
	return b.LifecycleRequest{Snapshot: s, Input: input, Operation: b.Operation{ID: "original-operation-" + string(action), Action: string(action), PayloadDigest: "digest", Ref: &ref, LifecycleIdentity: &b.LifecycleOperationIdentity{ProjectID: s.Session.ProjectID, ExecutionID: s.Session.ExecutionID, ConfigurationDigest: s.ConfigurationDigest, ResourceGeneration: s.ResourceGeneration, InputBindingDigest: b.LifecycleBindingDigest(s), Ref: ref}}}
}
func bind(s *b.LifecycleSnapshot, r b.LifecycleReceipt) {
	if r.Workspace != nil {
		s.Workspace = b.WorkspaceIntent{Path: r.Workspace.Worktree, Branch: r.Workspace.Branch, BaseCommit: r.Workspace.Base}
		s.Ref.WorkspacePath = s.Workspace.Path
		s.Ref.WorkspaceBranch = s.Workspace.Branch
		s.Ref.BaseCommit = s.Workspace.BaseCommit
	}
	if r.Target != nil {
		s.Ref.Target = *r.Target
	}
	if r.Process != nil {
		s.Ref.External = *r.Process
		s.Session.SessionID = r.State.SessionID
		s.Session.Generation = r.State.SessionGeneration
	}
	if len(r.Implementation) > 0 {
		s.Implementation = r.Implementation
	}
}
func prepared(t *testing.T, l *Lifecycle, s b.LifecycleSnapshot, cmd string) (b.LifecycleSnapshot, b.TurnInput) {
	t.Helper()
	text := "Implement the bound Spec and Plan."
	input := b.TurnInput{CommandID: cmd, Prompt: text}
	s.PendingTurn = &input
	r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareSession, b.LifecycleInput{Turn: &input}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	return s, input
}
func TestTerminalLifecycleReplyReuseAndExactStop(t *testing.T) {
	l, f, s := fixture(t)
	r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	f.writeReply = true
	for _, cmd := range []string{strings.Repeat("c", 32), strings.Repeat("f", 32)} {
		var input b.TurnInput
		s, input = prepared(t, l, s, cmd)
		r, observer, err := l.Execute(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
		if observer != nil {
			_ = observer.Close(context.Background())
		}
		if err != nil || !r.State.InputAccepted || !r.State.TurnStarted {
			t.Fatal("acceptance confused with start", err)
		}
		bind(&s, r)
		r, _, err = settled(t, l, s)
		if err != nil || r.TurnEvidence == nil || !r.TurnEvidence.Completed || r.TurnEvidence.FinalNativeTurnID != "" {
			t.Fatal(r, err)
		}
		bind(&s, r)
	}
	creates, sends := 0, 0
	for _, call := range f.calls {
		if call[0] == "worktree" && call[1] == "create" {
			creates++
		}
		if call[0] == "terminal" && call[1] == "send" {
			sends++
		}
	}
	if creates != 1 || sends != 2 {
		t.Fatal("replaced resource or repeated input", creates, sends)
	}
	r, err = l.StopLifecycle(context.Background(), request(s, b.LifecycleStopExecution, b.LifecycleInput{Stop: &b.StopInput{}}), nil)
	if err != nil || !r.State.WorkerAbsent || r.State.WritersAbsent {
		t.Fatal("overstated stop", r, err)
	}
}
func TestTerminalUnknownSubmissionOnlyObservesOriginal(t *testing.T) {
	l, f, s := fixture(t)
	r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	s, input := prepared(t, l, s, strings.Repeat("c", 32))
	f.unknown = true
	req := request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input})
	r, _, err = l.Execute(context.Background(), req)
	if err == nil || r.Disposition != b.OperationUnknown {
		t.Fatal("unknown promoted")
	}
	bind(&s, r)
	req.Snapshot = s
	before := len(f.calls)
	_, _, _ = l.Reconcile(context.Background(), req)
	for _, call := range f.calls[before:] {
		if call[1] == "send" || call[1] == "create" {
			t.Fatal("reconciliation mutated resource", call)
		}
	}
	if _, _, err = l.Execute(context.Background(), req); err == nil {
		t.Fatal("input repeated")
	}
}
func TestTerminalLocalAndRemoteUseIdenticalReturnAndNativePorts(t *testing.T) {
	for _, environment := range []string{"", "paired-worker"} {
		t.Run(environment, func(t *testing.T) {
			l, f, s := fixtureOn(t, environment)
			r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			f.writeReply = true
			s, input := prepared(t, l, s, strings.Repeat("c", 32))
			r, observer, err := l.Execute(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
			if observer != nil {
				_ = observer.Close(context.Background())
			}
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			r, _, err = settled(t, l, s)
			if err != nil || r.TurnEvidence == nil {
				t.Fatal("missing result", err)
			}
			for _, call := range f.calls {
				found := false
				for i, arg := range call {
					if arg == "--environment" && i+1 < len(call) && call[i+1] == environment {
						found = true
					}
				}
				if found != (environment != "") {
					t.Fatal("routing escaped", call)
				}
			}
		})
	}
}
func TestTerminalClippedOutputCannotComplete(t *testing.T) {
	l, f, s := fixture(t)
	r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	f.writeReply = true
	s, input := prepared(t, l, s, strings.Repeat("c", 32))
	r, observer, err := l.Execute(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
	if observer != nil {
		_ = observer.Close(context.Background())
	}
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	f.clipped = true
	r, _, err = settled(t, l, s)
	if err == nil || r.TurnEvidence != nil {
		t.Fatal("clipped result completed", r.Diagnostic, err)
	}
}

func TestScreenReplyDoesNotRequireOffscreenHistory(t *testing.T) {
	l, f, s := fixture(t)
	f.screen = true
	r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	s, input := prepared(t, l, s, strings.Repeat("c", 32))
	if _, observer, err := l.ObserveLifecycle(context.Background(), s); err != nil || observer != nil {
		t.Fatal("prepared input was observed as a submitted turn", err)
	}
	f.writeReply = true
	r, observer, err := l.Execute(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
	if observer != nil {
		_ = observer.Close(context.Background())
	}
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	r, _, err = settled(t, l, s)
	if err != nil || r.TurnEvidence == nil || r.TurnEvidence.LastMessage != `{"summary":"original result"}` {
		t.Fatal("complete visible frame was lost to offscreen history", r, err)
	}
}

func TestComposerDraftBlocksPreparationAndSubmission(t *testing.T) {
	l, f, s := fixture(t)
	r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	text := "Implement the bound Spec."
	input := b.TurnInput{CommandID: strings.Repeat("c", 32), Prompt: text}
	s.PendingTurn = &input
	f.draft = "native setup input"
	if _, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareSession, b.LifecycleInput{Turn: &input})); err == nil {
		t.Fatal("preparation accepted an occupied composer")
	}
	f.draft = ""
	s, input = prepared(t, l, s, input.CommandID)
	f.draft = "another UI input arrived after preparation"
	if _, _, err := l.Execute(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input})); err == nil {
		t.Fatal("submission appended business input to a native draft")
	}
	for _, call := range f.calls {
		if call[1] == "send" {
			t.Fatal("occupied composer mutated", call)
		}
	}
}

func settled(t *testing.T, l *Lifecycle, s b.LifecycleSnapshot) (b.LifecycleReceipt, b.LifecycleObserver, error) {
	t.Helper()
	r, o, err := l.observeOnce(context.Background(), s)
	if err != nil || r.TurnEvidence != nil || r.Diagnostic != "" {
		return r, o, err
	}
	bind(&s, r)
	time.Sleep(1100 * time.Millisecond)
	return l.observeOnce(context.Background(), s)
}

func TestTerminalReplyRequiresOriginalStartedAndStableIdle(t *testing.T) {
	for _, mode := range []string{"unstarted", "busy", "draft", "changing"} {
		t.Run(mode, func(t *testing.T) {
			l, f, s := fixture(t)
			r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			f.writeReply, f.unstarted = true, mode == "unstarted"
			s, input := prepared(t, l, s, strings.Repeat("c", 32))
			r, observer, err := l.Execute(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
			if observer != nil {
				_ = observer.Close(context.Background())
			}
			if err != nil {
				t.Fatal(err)
			}
			bind(&s, r)
			f.busy = mode == "busy"
			if mode == "draft" {
				f.draft = "another input"
			}
			r, _, err = l.observeOnce(context.Background(), s)
			if err != nil || r.TurnEvidence != nil {
				t.Fatal("premature completion", err)
			}
			bind(&s, r)
			if mode == "changing" {
				f.lines[len(f.lines)-3] = `• {"summary":"changed result"}`
			}
			r, _, err = l.observeOnce(context.Background(), s)
			if err != nil || r.TurnEvidence != nil {
				t.Fatal("unverified completion", err)
			}
		})
	}
}

func TestTerminalCandidateRecoveryNeverReplaysInput(t *testing.T) {
	l, f, s := fixture(t)
	r, _, err := l.Execute(context.Background(), request(s, b.LifecyclePrepareWorkspace, b.LifecycleInput{Workspace: &s.WorkspacePlan}))
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	f.writeReply = true
	s, input := prepared(t, l, s, strings.Repeat("c", 32))
	r, o, err := l.Execute(context.Background(), request(s, b.LifecycleStartTurn, b.LifecycleInput{Turn: &input}))
	if o != nil {
		_ = o.Close(context.Background())
	}
	if err != nil {
		t.Fatal(err)
	}
	bind(&s, r)
	r, _, err = l.observeOnce(context.Background(), s)
	if err != nil || r.TurnEvidence != nil {
		t.Fatal("candidate was not staged", err)
	}
	bind(&s, r)
	time.Sleep(1100 * time.Millisecond)
	// Reconstruct the adapter from only its frozen setup and persisted snapshot.
	setup := l.setup.Clone()
	setup.Snapshot = s.Clone()
	recovered, err := NewLifecycle(setup)
	if err != nil {
		t.Fatal(err)
	}
	r, _, err = recovered.(*Lifecycle).observeOnce(context.Background(), s)
	if err != nil || r.TurnEvidence == nil || r.TurnEvidence.LastMessage != `{"summary":"original result"}` {
		t.Fatal("saved candidate did not resume", err)
	}
	sends := 0
	for _, call := range f.calls {
		if call[0] == "terminal" && call[1] == "send" {
			sends++
		}
	}
	if sends != 1 {
		t.Fatal("recovery repeated input", sends)
	}
}
