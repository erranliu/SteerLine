package steerline

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const codexHistoryBindingFile = "codex-history.v1.json"

func localCodexHistoryBindingPath(dir, session string) string {
	return filepath.Join(dir, "codex-history."+session+".v1.json")
}

var codexThreadID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type codexHistoryBinding struct {
	Version     int               `json:"v"`
	ProjectID   string            `json:"project_id"`
	TaskID      string            `json:"task_id"`
	WorkflowID  string            `json:"workflow_id"`
	ExecutionID string            `json:"execution_id"`
	AttemptID   string            `json:"attempt_id"`
	Backend     BackendKind       `json:"backend"`
	Protocol    ExecutionProtocol `json:"protocol"`
	TargetID    string            `json:"target_id"`
	ExternalRef string            `json:"external_ref"`
	Workspace   string            `json:"workspace"`
	SessionID   string            `json:"session_id"`
	Command     Command           `json:"command"`
	Home        string            `json:"codex_home,omitempty"`
	Ended       bool              `json:"-"`
}

func (b codexHistoryBinding) validate() error {
	if b.Version != 1 || b.ProjectID == "" || b.ExecutionID == "" || b.AttemptID == "" || b.WorkflowID == "" || !filepath.IsAbs(b.Workspace) || b.Command.Executable == "" || !codexThreadID.MatchString(b.SessionID) {
		return errors.New("Codex conversation binding is incomplete")
	}
	if b.Backend != BackendLocal && b.Backend != BackendOrca {
		return errors.New("Codex history backend is unsupported")
	}
	if b.Backend == BackendLocal && b.Protocol != ProtocolLocalProcess && b.Protocol != ProtocolLocalCodexAppServer || b.Backend == BackendOrca && b.Protocol != ProtocolOrcaTerminalV2 {
		return errors.New("Codex history protocol does not match backend")
	}
	return nil
}
func (b codexHistoryBinding) identity() string {
	return "codex-app-server-v1:" + digestValue(struct {
		Session, Home, Workspace string
		Command                  Command
	}{b.SessionID, b.Home, canonical(b.Workspace), b.Command})
}
func (b codexHistoryBinding) event() ConversationEvent {
	return ConversationEvent{Version: 1, ProjectID: b.ProjectID, TaskID: b.TaskID, WorkflowID: b.WorkflowID, ExecutionID: b.ExecutionID, AttemptID: b.AttemptID, Backend: b.Backend, Protocol: b.Protocol, Agent: "codex", TargetID: b.TargetID, ExternalRef: b.ExternalRef, Source: "codex-app-server", SourceSegment: b.identity(), SourceIdentity: b.identity(), SessionID: b.SessionID, SessionQuality: IdentityNative, TurnQuality: IdentityUnknown, ItemQuality: IdentityUnknown, SourceEventQuality: IdentityDerived, Phase: MessagePhaseUnknown, ReceivedAt: now()}
}
func codexReaderCommand(c Config) Command {
	command := c.Codex
	command.Args = append([]string(nil), command.Args...)
	if c.Profile != "" {
		command.Args = append(command.Args, "--profile", c.Profile)
	}
	return command
}

func codexEnvironmentHome() string {
	if path := os.Getenv("CODEX_HOME"); path != "" {
		return path
	}
	if path, err := os.UserHomeDir(); err == nil {
		return filepath.Join(path, ".codex")
	}
	return ""
}
func nativeCodexHistoryAvailable(ctx context.Context, c Config) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client, err := openCodexHistory(ctx, codexReaderCommand(c), c.Repository, "")
	if err != nil {
		return false
	}
	client.Close()
	return true
}

// The launcher runs inside Orca's terminal environment, which may use a managed
// CODEX_HOME. Only a non-model bootstrap user item is injected to persist the new
// empty session. Every assistant phase below still comes from native history.
type codexSessionLaunch struct {
	Binding     codexHistoryBinding `json:"binding"`
	BindingPath string              `json:"binding_path"`
	Args        []string            `json:"args"`
	Prompt      string              `json:"prompt"`
	Sandbox     string              `json:"sandbox"`
}

func RunCodexSessionLauncher(ctx context.Context, requestPath string) error {
	raw, err := os.ReadFile(requestPath)
	if err != nil {
		return err
	}
	var request codexSessionLaunch
	if err = json.Unmarshal(raw, &request); err != nil {
		return err
	}
	b := request.Binding
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if b.Version != 1 || b.Backend != BackendOrca || b.Protocol != ProtocolOrcaTerminalV2 || b.SessionID != "" || b.Command.Executable == "" || canonical(cwd) != canonical(b.Workspace) || !filepath.IsAbs(requestPath) || canonical(filepath.Dir(request.BindingPath)) != canonical(filepath.Dir(requestPath)) || filepath.Base(request.BindingPath) != codexHistoryBindingFile {
		return errors.New("invalid Codex session launch binding")
	}
	if _, err = os.Stat(request.BindingPath); !os.IsNotExist(err) {
		return errors.New("Codex session binding already exists; refusing another launch")
	}
	b.Home = codexEnvironmentHome()
	setupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client, err := openCodexHistory(setupCtx, b.Command, cwd, b.Home)
	if err != nil {
		return err
	}
	params := map[string]any{"cwd": cwd, "ephemeral": false}
	if request.Sandbox != "" {
		params["sandbox"] = request.Sandbox
	}
	result, err := client.call(setupCtx, "thread/start", params)
	if err != nil {
		client.Close()
		return err
	}
	var started struct {
		Thread codexHistoryThread `json:"thread"`
	}
	if err = json.Unmarshal(result, &started); err != nil {
		client.Close()
		return err
	}
	b.SessionID = started.Thread.ID
	if err = b.validate(); err != nil {
		client.Close()
		return err
	}
	_, err = client.call(setupCtx, "thread/inject_items", map[string]any{"threadId": b.SessionID, "items": []any{map[string]any{"type": "message", "role": "user", "content": []any{map[string]string{"type": "input_text", "text": "SteerLine prepared this session for execution " + b.ExecutionID + ", attempt " + b.AttemptID + ". Await the task instruction."}}}}})
	client.Close()
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	if err = AtomicWrite(request.BindingPath, append(encoded, '\n')); err != nil {
		return err
	}
	args := append(append([]string{}, b.Command.Args...), "resume", "--include-non-interactive", b.SessionID)
	args = append(args, request.Args...)
	args = append(args, request.Prompt)
	cmd := exec.Command(b.Command.Executable, args...)
	cmd.Dir = cwd
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s *Service) bindLocalCodexHistory(run *Run, source WorkerEvent) error {
	if source.Source != "codex-jsonl" || !codexThreadID.MatchString(source.Session) || run.Backend != string(BackendLocal) {
		return nil
	}
	logPath := s.localConversationLogPath(run, source.AttemptID)
	if run.WorkflowVersion == WorkerFlowVersion {
		logPath = s.workerFlowSourcePath(run, source.SourceIdentity)
	}
	if logPath == "" {
		return errors.New("Codex session lacks its attempt source path")
	}
	var marker ConversationSourceMarker
	if raw, err := os.ReadFile(filepath.Join(filepath.Dir(logPath), ConversationSourceMarkerName)); err != nil {
		return err
	} else if err = json.Unmarshal(raw, &marker); err != nil {
		return err
	}
	if marker.validate() != nil || marker.SourceIdentity != source.SourceIdentity || marker.ProjectID != s.Config.identity || marker.WorkflowID != run.RunID || marker.ExecutionID != run.ExecutionID || marker.AttemptID != source.AttemptID || marker.Backend != BackendLocal || marker.Protocol != ProtocolLocalProcess || marker.TargetID != run.Target.ID {
		return errors.New("Codex session does not match its source marker")
	}
	command := marker.ReaderCommand
	if command.Executable == "" {
		fixed := s.projectExecutionConfig()
		if run.WorkflowVersion == WorkerFlowVersion {
			var err error
			fixed, err = s.executionConfigForRun(run)
			if err != nil {
				return err
			}
		}
		command = codexReaderCommand(fixed)
	}
	b := codexHistoryBinding{Version: 1, ProjectID: marker.ProjectID, TaskID: run.DomainTaskID, WorkflowID: marker.WorkflowID, ExecutionID: marker.ExecutionID, AttemptID: marker.AttemptID, Backend: BackendLocal, Protocol: ProtocolLocalProcess, TargetID: marker.TargetID, ExternalRef: conversationExternalRef(run.Process), Workspace: deref(run.WorktreePath), SessionID: source.Session, Command: command, Home: marker.CodexHome}
	path := localCodexHistoryBindingPath(filepath.Dir(logPath), b.SessionID)
	return writeCodexHistoryBinding(path, b)
}

func (s *Service) localConversationLogPath(run *Run, attemptID string) string {
	path := deref(run.LogPath)
	for _, attempt := range s.domain.Attempts {
		if attempt.AttemptID == attemptID && attempt.ExecutionID == run.ExecutionID && attempt.EventPath != "" {
			return attempt.EventPath
		}
	}
	return path
}

func (s *Service) workerFlowSourcePath(run *Run, sourceIdentity string) string {
	if run == nil || run.WorkflowVersion != WorkerFlowVersion || sourceIdentity == "" {
		return ""
	}
	root := filepath.Join(s.Config.RunDir, "runs", run.RunID, "turns")
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) > 4096 {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() || !stableExecutionID(entry.Name()) {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		data, err := os.ReadFile(filepath.Join(dir, ConversationSourceMarkerName))
		if err != nil {
			continue
		}
		var marker ConversationSourceMarker
		if json.Unmarshal(data, &marker) == nil && marker.validate() == nil && marker.SourceIdentity == sourceIdentity &&
			marker.ProjectID == s.Config.identity && marker.WorkflowID == run.RunID && marker.ExecutionID == run.ExecutionID && marker.AttemptID == run.AttemptID {
			return filepath.Join(dir, "events.jsonl")
		}
	}
	return ""
}

func writeCodexHistoryBinding(path string, b codexHistoryBinding) error {
	if old, err := os.ReadFile(path); err == nil {
		var previous codexHistoryBinding
		if json.Unmarshal(old, &previous) != nil || previous.identity() != b.identity() || previous.ProjectID != b.ProjectID || previous.WorkflowID != b.WorkflowID || previous.ExecutionID != b.ExecutionID || previous.AttemptID != b.AttemptID || previous.Backend != b.Backend || previous.Protocol != b.Protocol || previous.TargetID != b.TargetID {
			return errors.New("Codex session changed within the same attempt")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := b.validate(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(path, append(raw, '\n'))
}

func (s *Service) codexHistorySources() []codexHistoryBinding {
	sources := []codexHistoryBinding{}
	seen := map[string]bool{}
	for _, run := range s.runs {
		if run == nil || run.Agent != "codex" {
			continue
		}
		add := func(path, attempt string, ended bool) {
			dir := filepath.Dir(path)
			if path == "" || seen[dir] || !inside(filepath.Join(s.Config.RunDir, "runs", run.RunID), path) {
				return
			}
			seen[dir] = true
			entries, err := os.ReadDir(dir)
			if err != nil {
				if !os.IsNotExist(err) {
					s.reportInvalidCodexBinding(run, attempt, path, err)
				}
				return
			}
			if len(entries) > 2048 {
				s.reportInvalidCodexBinding(run, attempt, dir, errors.New("too many files to scan for bounded Codex history bindings"))
				return
			}
			for _, entry := range entries {
				name := entry.Name()
				if name != codexHistoryBindingFile {
					if !strings.HasPrefix(name, "codex-history.") || !strings.HasSuffix(name, ".v1.json") {
						continue
					}
					session := strings.TrimSuffix(strings.TrimPrefix(name, "codex-history."), ".v1.json")
					if !codexThreadID.MatchString(session) {
						continue
					}
				}
				bindingPath := filepath.Join(dir, name)
				if !entry.Type().IsRegular() {
					s.reportInvalidCodexBinding(run, attempt, bindingPath, errors.New("binding is not a regular file"))
					continue
				}
				raw, readErr := os.ReadFile(bindingPath)
				if readErr != nil {
					s.reportInvalidCodexBinding(run, attempt, bindingPath, readErr)
					continue
				}
				var b codexHistoryBinding
				if json.Unmarshal(raw, &b) != nil || b.validate() != nil || b.ProjectID != s.Config.identity || b.WorkflowID != run.RunID || b.ExecutionID != run.ExecutionID || b.AttemptID != attempt || b.TargetID != run.Target.ID || b.Backend != BackendKind(run.Backend) || b.Protocol != ExecutionProtocol(run.Protocol) || canonical(b.Workspace) != canonical(deref(run.WorktreePath)) || b.Backend == BackendOrca && name != codexHistoryBindingFile || b.Backend == BackendLocal && name != filepath.Base(localCodexHistoryBindingPath(dir, b.SessionID)) {
					s.reportInvalidCodexBinding(run, attempt, bindingPath, errors.New("binding identity differs from recorded execution"))
					continue
				}
				if b.Backend == BackendOrca && b.ExternalRef == "" {
					continue
				} // Launcher has not yet completed the terminal binding.
				b.Ended = ended
				sources = append(sources, b)
			}
		}

		for _, attempt := range s.domain.Attempts {
			if attempt.ExecutionID == run.ExecutionID {
				ended := attempt.AttemptID != run.AttemptID || run.EndedAt != nil
				if run.WorkflowVersion == WorkerFlowVersion && attempt.AttemptID == run.AttemptID {
					ended = run.TaskStatus == "closed" || run.TaskStatus == "discarded"
				}
				add(attempt.EventPath, attempt.AttemptID, ended)
			}
		}
	}
	return sources
}

func (s *Service) reportInvalidCodexBinding(run *Run, attempt, path string, cause error) {
	err := fmt.Errorf("Codex history binding for %s/%s: %w", run.RunID, attempt, cause)
	s.setError(err)
	if s.domainStore == nil {
		return
	}
	e := ConversationEvent{Version: ConversationVersion, ProjectID: s.Config.identity, TaskID: run.DomainTaskID, WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: attempt, Backend: BackendKind(run.Backend), Protocol: ExecutionProtocol(run.Protocol), Agent: run.Agent, TargetID: run.Target.ID, Source: "codex-app-server", SourceSegment: "invalid-binding-v1:" + attempt, SessionQuality: IdentityUnknown, TurnQuality: IdentityUnknown, ItemQuality: IdentityUnknown, SourceEventQuality: IdentityDerived, SourceEventID: "binding:" + digestValue(struct{ Path, Reason string }{path, cause.Error()}), Type: ConversationGap, Phase: MessagePhaseUnknown, Status: "source_unavailable", Diagnostic: err.Error(), ReceivedAt: now()}
	_, _ = s.domainStore.CommitConversation(context.Background(), []ConversationWrite{{Event: e}}, ConversationCheckpoint{})
}

type codexHistoryCursor struct {
	Cursor      string             `json:"cursor"`
	Sequence    uint64             `json:"sequence"`
	LastTurnID  string             `json:"last_turn_id,omitempty"`
	LastDigest  string             `json:"last_digest,omitempty"`
	Pending     []codexPendingTurn `json:"pending,omitempty"`
	NextPending int                `json:"next_pending,omitempty"`
	ReadPending bool               `json:"read_pending,omitempty"`
}

type codexPendingTurn struct {
	ID     string `json:"id"`
	Cursor string `json:"cursor"`
	Digest string `json:"digest"`
}

func (s *Service) collectCodexHistoryPage(ctx context.Context, b codexHistoryBinding) (bool, error) {
	if err := b.validate(); err != nil {
		return false, err
	}
	cp, err := s.domainStore.ConversationCheckpoint(ctx, b.ProjectID, b.ExecutionID, b.AttemptID, "codex-app-server", b.identity())
	hadCheckpoint := err == nil
	var cursor codexHistoryCursor
	previousCursor := ""
	if err == nil {
		if len(cp.Cursor) > MaxEventPageBytes {
			return false, errors.New("persisted Codex source cursor exceeds the read budget")
		}
		previousCursor = cp.Cursor
		if err = json.Unmarshal([]byte(cp.Cursor), &cursor); err != nil {
			return false, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	readerDir := b.Workspace
	if info, statErr := os.Stat(readerDir); statErr != nil || !info.IsDir() {
		readerDir = s.Config.Repository
		if info, statErr = os.Stat(readerDir); statErr != nil || !info.IsDir() {
			readerDir = s.Config.RunDir
			if info, statErr = os.Stat(readerDir); statErr != nil || !info.IsDir() {
				// An empty Dir lets the public read-only client use the service's
				// existing cwd; thread/read still checks the original workspace.
				readerDir = ""
			}
		}
	}
	client, err := openCodexHistory(ctx, b.Command, readerDir, b.Home)
	if err != nil {
		return false, err
	}
	defer client.Close()
	if err = client.readMetadata(ctx, b.SessionID, b.Workspace); err != nil {
		return false, err
	}
	readCursor := cursor.Cursor
	pendingIndex := -1
	if len(cursor.Pending) > 0 && cursor.ReadPending {
		pendingIndex = cursor.NextPending % len(cursor.Pending)
		readCursor = cursor.Pending[pendingIndex].Cursor
		cursor.NextPending++
		cursor.ReadPending = false
	} else if len(cursor.Pending) > 0 {
		cursor.ReadPending = true
	}
	page, err := client.readPage(ctx, b.SessionID, readCursor)
	if err != nil {
		return false, err
	}
	if pendingIndex >= 0 && (len(page.Data) != 1 || page.Data[0].ID != cursor.Pending[pendingIndex].ID) {
		return false, errors.New("Codex pending turn cursor changed its native identity")
	}
	base := b.event()
	var writes []ConversationWrite
	if cursor.Sequence == 0 && len(page.Data) > 0 {
		e := base
		e.Type = ConversationGap
		e.Status = "source_changed"
		e.SourceEventID = "native-source-selected"
		e.Diagnostic = "Verified Codex history is the canonical conversation source; raw source projections remain available through an explicit source filter"
		e.Attributes = map[string]any{"supersedes_sources": []string{"codex-jsonl", "orca-terminal-v2", "orca-reply-receipt"}}
		writes = append(writes, ConversationWrite{Event: e})
		e = base
		e.Type = ConversationSession
		e.SourceEventID = "session:" + b.SessionID
		writes = append(writes, ConversationWrite{Event: e})
	}
	for _, turn := range page.Data {
		digest := digestValue(turn)
		unchanged := pendingIndex >= 0 && cursor.Pending[pendingIndex].Digest == digest || pendingIndex < 0 && cursor.LastTurnID == turn.ID && cursor.LastDigest == digest
		if !unchanged {
			mapped, mapErr := mapCodexHistoryTurn(base, turn, cursor.Sequence+uint64(len(writes))+1)
			if mapErr != nil {
				return false, mapErr
			}
			writes = append(writes, mapped...)
		}
		hasFinal := false
		for _, raw := range turn.Items {
			var item struct {
				Type  string  `json:"type"`
				Phase string  `json:"phase"`
				Text  *string `json:"text"`
			}
			if err := json.Unmarshal(raw, &item); err != nil {
				return false, err
			}
			if item.Type == "agentMessage" && item.Phase == string(MessagePhaseFinalAnswer) && item.Text != nil {
				hasFinal = true
				break
			}
		}
		terminal := turn.CompletedAt != nil && (turn.Status == "completed" || turn.Status == "failed" || turn.Status == "interrupted")
		if pendingIndex >= 0 && hasFinal {
			cursor.Pending = append(cursor.Pending[:pendingIndex], cursor.Pending[pendingIndex+1:]...)
			if len(cursor.Pending) == 0 {
				cursor.NextPending, cursor.ReadPending = 0, false
			}
		} else if pendingIndex >= 0 {
			cursor.Pending[pendingIndex].Digest = digest
		} else if terminal && !hasFinal {
			if page.BackwardsCursor == nil || *page.BackwardsCursor == "" {
				return false, errors.New("Codex omitted the anchor needed to reconcile a final-less turn")
			}
			known := false
			for _, pending := range cursor.Pending {
				if pending.ID == turn.ID {
					known = true
					break
				}
			}
			if !known {
				if len(cursor.Pending) >= 1024 {
					return false, errors.New("Codex history has too many unresolved turns for a bounded cursor")
				}
				cursor.Pending = append(cursor.Pending, codexPendingTurn{ID: turn.ID, Cursor: *page.BackwardsCursor, Digest: digest})
			}
		}
		if pendingIndex < 0 {
			cursor.LastTurnID, cursor.LastDigest = turn.ID, digest
		}
	}
	if pendingIndex < 0 {
		if page.NextCursor != nil && *page.NextCursor != "" {
			cursor.Cursor = *page.NextCursor
		} else if page.BackwardsCursor != nil {
			cursor.Cursor = *page.BackwardsCursor
		}
	}
	cursor.Sequence += uint64(len(writes))
	raw, _ := json.Marshal(cursor)
	if len(raw) > MaxEventPageBytes {
		return false, errors.New("Codex source cursor exceeds the persisted budget")
	}
	cp = ConversationCheckpoint{ProjectID: b.ProjectID, ExecutionID: b.ExecutionID, AttemptID: b.AttemptID, Source: base.Source, SourceSegment: b.identity(), SourceIdentity: b.identity(), Generation: b.SessionID, Cursor: string(raw), LocalSeq: cursor.Sequence}
	// Prefix batches are replayable by native event identity. The source cursor
	// advances only with the last committed batch, after every referenced body.
	for len(writes) > 128 {
		if _, err = s.domainStore.CommitConversation(ctx, writes[:128], ConversationCheckpoint{}); err != nil {
			return false, err
		}
		writes = writes[128:]
	}
	if len(writes) > 0 || cp.Cursor != previousCursor {
		_, err = s.domainStore.CommitConversation(ctx, writes, cp)
	}
	if err == nil {
		err = s.recordCodexHistoryCapabilities(ctx, b, client, page, hadCheckpoint)
	}
	settled := pendingIndex < 0 && len(cursor.Pending) == 0 && len(page.Data) > 0 && page.Data[0].CompletedAt != nil && (page.Data[0].Status == "completed" || page.Data[0].Status == "failed" || page.Data[0].Status == "interrupted")
	return page.NextCursor == nil && settled, err
}

func (s *Service) recordCodexHistoryCapabilities(ctx context.Context, b codexHistoryBinding, client *codexHistoryClient, page codexHistoryPage, hadCheckpoint bool) error {
	values := map[string]ConversationCapability{
		"turn_boundaries":      {Name: "turn_boundaries", Status: CapabilityNotTested, Evidence: "No completed native turn observed yet"},
		"message_phase":        {Name: "message_phase", Status: CapabilityNotTested, Evidence: "No phase-bearing assistant message observed yet"},
		"final_content":        {Name: "final_content", Status: CapabilityNotTested, Evidence: "No complete native final message observed yet"},
		"stable_item_identity": {Name: "stable_item_identity", Status: CapabilityNotTested, Evidence: "No native item observed yet"},
		"incremental_read":     {Name: "incremental_read", Status: CapabilityNotTested, Evidence: "A saved turn cursor has not been resumed yet"},
		"history_replay":       {Name: "history_replay", Status: CapabilityNotTested, Evidence: "A persisted checkpoint has not been replayed yet"},
		"input_wait":           {Name: "input_wait", Status: CapabilityNotTested, Evidence: "A host input-wait object has not been observed"},
	}
	if b.Backend == BackendLocal {
		values["input_wait"] = ConversationCapability{Name: "input_wait", Status: CapabilityUnsupported, Evidence: "local_process Reply is unsupported by the execution backend"}
	}
	for _, turn := range page.Data {
		if turn.CompletedAt != nil {
			values["turn_boundaries"] = ConversationCapability{Name: "turn_boundaries", Status: CapabilityVerified, Evidence: "Native turn ID, terminal status and completedAt were observed through full turn history"}
		}
		if len(turn.Items) > 0 {
			values["stable_item_identity"] = ConversationCapability{Name: "stable_item_identity", Status: CapabilityVerified, Evidence: "Native session, turn and item IDs were observed through full turn history"}
		}
		for _, raw := range turn.Items {
			var item struct {
				Type  string  `json:"type"`
				Phase string  `json:"phase"`
				Text  *string `json:"text"`
			}
			if err := json.Unmarshal(raw, &item); err != nil {
				return err
			}
			if item.Type == "agentMessage" && (item.Phase == string(MessagePhaseCommentary) || item.Phase == string(MessagePhaseFinalAnswer)) {
				values["message_phase"] = ConversationCapability{Name: "message_phase", Status: CapabilityVerified, Evidence: "Native agentMessage.phase was observed through full turn history"}
				if item.Phase == string(MessagePhaseFinalAnswer) && turn.CompletedAt != nil && item.Text != nil {
					values["final_content"] = ConversationCapability{Name: "final_content", Status: CapabilityVerified, Evidence: "A completed native final_answer item has a persisted full body reference"}
				}
			}
		}
	}
	if hadCheckpoint && (page.NextCursor != nil || page.BackwardsCursor != nil) {
		values["incremental_read"] = ConversationCapability{Name: "incremental_read", Status: CapabilityVerified, Evidence: "A persisted opaque turn cursor was resumed with a bounded full-item page"}
	}
	if hadCheckpoint && len(page.Data) > 0 {
		values["history_replay"] = ConversationCapability{Name: "history_replay", Status: CapabilityVerified, Evidence: "A new public app-server process replayed native history from the persisted source checkpoint"}
	}
	version := clip(client.userAgent, 256)
	reports, err := s.domainStore.ConversationCapabilityReports(ctx, b.Backend, b.Protocol, b.TargetID, 100)
	if err != nil {
		return err
	}
	var previous *ConversationCapabilityReport
	for i := range reports {
		if reports[i].ExecutionID == b.ExecutionID && reports[i].AttemptID == b.AttemptID && reports[i].SourceIdentity == b.identity() {
			previous = &reports[i]
			break
		}
	}
	if previous != nil && previous.AgentVersion == version {
		for _, old := range previous.Capabilities {
			current, ok := values[old.Name]
			if ok && current.Status == CapabilityNotTested && old.Status != CapabilityNotTested {
				values[old.Name] = old
			}
			if ok && old.Status == CapabilityVerified && current.Status != CapabilityVerified {
				values[old.Name] = old
			}
		}
	}
	names := []string{"turn_boundaries", "message_phase", "final_content", "stable_item_identity", "incremental_read", "history_replay", "input_wait"}
	platformVersion := version
	if b.Backend == BackendOrca {
		config, configErr := s.executionConfigForWorkflowID(b.WorkflowID)
		if configErr != nil {
			return configErr
		}
		if config.Orca.Executable == "" {
			return errors.New("Orca capability report lacks a configured version probe")
		}
		response, probeErr := (orcaClient{cfg: config}).call(ctx, positiveDuration(config.CommandTimeout, 30*time.Second), "--version")
		if probeErr != nil {
			return fmt.Errorf("Orca capability version probe failed: %w", probeErr)
		}
		platformVersion = strings.TrimSpace(string(response))
		if platformVersion == "" {
			return errors.New("Orca capability version probe returned no version")
		}
	}
	report := ConversationCapabilityReport{Version: 1, Backend: b.Backend, Protocol: b.Protocol, Agent: "codex", AgentVersion: version, PlatformVersion: platformVersion, Target: ExecutionTarget{ID: b.TargetID}, ExecutionID: b.ExecutionID, AttemptID: b.AttemptID, Source: "codex-app-server", SourceIdentity: b.identity(), CheckedAt: now(), Capabilities: make([]ConversationCapability, 0, len(names))}
	for _, name := range names {
		report.Capabilities = append(report.Capabilities, values[name])
	}
	if previous != nil && previous.AgentVersion == report.AgentVersion && previous.PlatformVersion == report.PlatformVersion {
		unchanged := true
		for i, cap := range report.Capabilities {
			if i >= len(previous.Capabilities) || previous.Capabilities[i] != cap {
				unchanged = false
				break
			}
		}
		if unchanged {
			return nil
		}
	}
	return s.domainStore.SaveConversationCapabilityReport(ctx, report)
}

func (s *Service) runCodexHistoryCollector(ctx context.Context, requests <-chan []codexHistoryBinding, errorsOut chan<- error) {
	finished := map[string]bool{}
	activeIndex, historicalIndex := 0, 0
	for {
		select {
		case <-ctx.Done():
			return
		case sources := <-requests:
			active, historical := []codexHistoryBinding{}, []codexHistoryBinding{}
			for _, source := range sources {
				if source.Ended {
					historical = append(historical, source)
				} else {
					active = append(active, source)
				}
			}
			for i, j := 0, len(historical)-1; i < j; i, j = i+1, j-1 {
				historical[i], historical[j] = historical[j], historical[i]
			}
			pick := func(group []codexHistoryBinding, next *int) (codexHistoryBinding, bool) {
				for i := 0; i < len(group); i++ {
					index := (*next + i) % len(group)
					source := group[index]
					if !finished[source.identity()+":"+source.AttemptID] {
						*next = index + 1
						return source, true
					}
				}
				return codexHistoryBinding{}, false
			}
			selected := make([]codexHistoryBinding, 0, 2)
			if source, ok := pick(active, &activeIndex); ok {
				selected = append(selected, source)
			}
			if source, ok := pick(historical, &historicalIndex); ok {
				selected = append(selected, source)
			}
			for _, source := range selected {
				key := source.identity() + ":" + source.AttemptID
				readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
				settled, err := s.collectCodexHistoryPage(readCtx, source)
				cancel()
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					e := source.event()
					e.Type = ConversationGap
					e.Status = "source_unavailable"
					e.SourceEventID = "read-error:" + digestValue(err.Error())
					e.Diagnostic = err.Error()
					_, _ = s.domainStore.CommitConversation(ctx, []ConversationWrite{{Event: e}}, ConversationCheckpoint{})
					select {
					case errorsOut <- fmt.Errorf("Codex conversation history: %w", err):
					default:
					}
				} else if source.Ended && settled {
					finished[key] = true
				}
			}
		}
	}
}
