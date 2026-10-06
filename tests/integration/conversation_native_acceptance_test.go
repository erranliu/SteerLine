package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productdto "github.com/erranliu/SteerLine/internal/product/api"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	steerline "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func TestCodexHistoryRealFailedAcceptance(t *testing.T) {
	path := os.Getenv("STEERLINE_CODEX_HISTORY_FAILED_BINDING")
	if path == "" {
		t.Skip("set an exact isolated failed-turn binding to run acceptance")
	}
	raw, err := os.ReadFile(path)
	require(t, err)
	var binding codexworker.HistoryBinding
	require(t, json.Unmarshal(raw, &binding))
	if binding.Backend != executionbackend.BackendLocal || binding.Home == "" {
		t.Fatal("failed-turn acceptance requires an isolated local Codex home")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "failed.db"))
	require(t, err)
	defer store.Close()
	service := &isolatedHistoryCommitFixture{domainStore: store, directory: t.TempDir()}
	_, err = service.collectCodexHistoryPage(ctx, binding)
	require(t, err)
	query := productquery.NewConversationService(store, binding.ProjectID)
	filter := productdto.ConversationFilter{ExecutionID: binding.ExecutionID, AttemptID: binding.AttemptID, Source: "codex-app-server"}
	page, err := query.ReadConversation(ctx, filter, "", 100)
	require(t, err)
	failures := 0
	for _, event := range page.Events {
		if event.Type == productdto.ConversationTurnFailed {
			failures++
			if event.Status != "failed" || event.FinalAvailability != productdto.FinalAbsent || event.Raw == nil {
				t.Fatalf("real failed turn lost native status or error reference: %+v", event)
			}
		}
		if event.Type == productdto.ConversationTurnCancelled || event.Type == productdto.ConversationMessageComplete {
			t.Fatalf("real failed turn was confused with cancellation or final: %+v", event)
		}
	}
	if failures != 1 {
		t.Fatalf("real failed turn was not collected once: %+v", page.Events)
	}
	_, err = service.collectCodexHistoryPage(ctx, binding)
	require(t, err)
	replayed, err := query.ReadConversation(ctx, filter, "", 100)
	require(t, err)
	if len(replayed.Events) != len(page.Events) {
		t.Fatalf("real failed-turn replay duplicated events: %d -> %d", len(page.Events), len(replayed.Events))
	}
}

func TestCodexHistoryNativeAcceptance(t *testing.T) {
	path := os.Getenv("STEERLINE_CODEX_HISTORY_NATIVE_BINDINGS")
	if path == "" {
		t.Skip("set exact isolated native probe bindings to run acceptance")
	}
	raw, err := os.ReadFile(path)
	require(t, err)
	var bindings []codexworker.HistoryBinding
	require(t, json.Unmarshal(raw, &bindings))
	if len(bindings) != 2 {
		t.Fatal("acceptance requires local and Orca probe sessions")
	}
	var evidence []map[string]any
	for _, binding := range bindings {
		t.Run(string(binding.Backend), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			dbPath := filepath.Join(t.TempDir(), "history.db")
			store, err := sqlite.OpenSQLiteStore(dbPath)
			require(t, err)
			service := &isolatedHistoryCommitFixture{domainStore: store, directory: t.TempDir()}
			if binding.Backend == executionbackend.BackendOrca {
				orcaExe, lookErr := exec.LookPath("orca")
				require(t, lookErr)
				version, versionErr := exec.CommandContext(ctx, orcaExe, "--version").Output()
				require(t, versionErr)
				service.platformVersion = strings.TrimSpace(string(version))
			}
			settled := false
			for n := 0; n < 8 && !settled; n++ {
				settled, err = service.collectCodexHistoryPage(ctx, binding)
				require(t, err)
			}
			if !settled {
				t.Fatal("native turn paging did not reach the final turn")
			}
			query := productquery.NewConversationService(store, binding.ProjectID)
			filter := productdto.ConversationFilter{ExecutionID: binding.ExecutionID, AttemptID: binding.AttemptID}
			page, err := query.ReadConversation(ctx, filter, "", 1000)
			require(t, err)
			starts, ends, commentary, finals, tools := 0, 0, 0, 0, 0
			digests := []string{}
			for _, e := range page.Events {
				switch e.Type {
				case productdto.ConversationTurnStarted:
					starts++
				case productdto.ConversationTurnCompleted:
					ends++
				case productdto.ConversationToolCompleted:
					tools++
				}
				if e.Type == productdto.ConversationMessageComplete {
					if e.Phase == productdto.MessagePhaseCommentary {
						commentary++
					}
					if e.Phase == productdto.MessagePhaseFinalAnswer {
						finals++
					}
					if e.Body == nil {
						t.Fatal("native full message body missing")
					}
					body, bodyErr := query.ReadMessageBody(ctx, *e.Body, 0, 256<<10)
					require(t, bodyErr)
					if !body.Complete || body.SHA256 != e.Body.SHA256 {
						t.Fatal("native message body/hash mismatch")
					}
					digests = append(digests, body.SHA256)
				}
			}
			if starts != 2 || ends != 2 || commentary < 1 || finals != 2 {
				t.Fatalf("native exact facts missing: starts=%d ends=%d commentary=%d finals=%d", starts, ends, commentary, finals)
			}
			if binding.Backend == executionbackend.BackendOrca && tools < 2 {
				t.Fatal("Orca native tool history missing")
			}
			reports, reportErr := store.ConversationCapabilityReports(ctx, binding.Backend, binding.Protocol, binding.TargetID, 1)
			require(t, reportErr)
			if len(reports) != 1 || reports[0].AgentVersion == "" || reports[0].PlatformVersion == "" || reports[0].PlatformVersion == "windows" || reports[0].ExecutionID != binding.ExecutionID || reports[0].AttemptID != binding.AttemptID || reports[0].SourceIdentity != binding.Identity() {
				t.Fatal("native capability report lacks actual agent version")
			}
			statuses := map[string]executionbackend.CapabilityStatus{}
			for _, cap := range reports[0].Capabilities {
				statuses[cap.Name] = executionbackend.CapabilityStatus(cap.Status)
			}
			for _, name := range []string{"turn_boundaries", "message_phase", "final_content", "stable_item_identity", "incremental_read", "history_replay"} {
				if statuses[name] != executionbackend.CapabilityVerified {
					t.Fatalf("native report did not verify %s: %+v", name, statuses)
				}
			}
			if binding.Backend == executionbackend.BackendOrca && statuses["input_wait"] != executionbackend.CapabilityNotTested {
				t.Fatal("unobserved Orca wait was overclaimed")
			}
			if binding.Backend == executionbackend.BackendLocal && statuses["input_wait"] != executionbackend.CapabilityUnsupported {
				t.Fatal("unsupported local reply was overclaimed")
			}
			checkpoint, err := store.ConversationCheckpoint(ctx, binding.ProjectID, binding.ExecutionID, binding.AttemptID, "codex-app-server", binding.Identity())
			require(t, err)
			require(t, store.Close())
			store, err = sqlite.OpenSQLiteStore(dbPath)
			require(t, err)
			defer store.Close()
			service.domainStore = store
			query = productquery.NewConversationService(store, binding.ProjectID)
			for i := 0; i < 2; i++ {
				_, err = service.collectCodexHistoryPage(ctx, binding)
				require(t, err)
			}
			replayedCheckpoint, checkpointErr := store.ConversationCheckpoint(ctx, binding.ProjectID, binding.ExecutionID, binding.AttemptID, "codex-app-server", binding.Identity())
			require(t, checkpointErr)
			if replayedCheckpoint.LocalSeq != checkpoint.LocalSeq || replayedCheckpoint.Cursor != checkpoint.Cursor {
				t.Fatal("unchanged native history advanced the persisted source position")
			}
			replayed, err := query.ReadConversation(ctx, filter, "", 1000)
			require(t, err)
			if len(replayed.Events) != len(page.Events) {
				t.Fatalf("native reopen replay duplicated events: %d -> %d", len(page.Events), len(replayed.Events))
			}
			wrong := binding
			wrong.Workspace = t.TempDir()
			if _, err = service.collectCodexHistoryPage(ctx, wrong); err == nil {
				t.Fatal("history read crossed workspace identity")
			}
			evidence = append(evidence, map[string]any{"backend": binding.Backend, "protocol": binding.Protocol, "session_sha256": isolatedHistoryDigest(binding.SessionID), "events": len(page.Events), "native_turn_starts": starts, "native_turn_completions": ends, "commentary_messages": commentary, "final_messages": finals, "tool_completions": tools, "body_sha256": digests, "cursor_persisted": checkpoint.Cursor != "", "capabilities_persisted_by_target_and_version": true, "reopen_replay_without_duplicates": true, "reopen_replay_without_cursor_advance": true, "wrong_workspace_rejected": true})
		})
	}
	if out := os.Getenv("STEERLINE_CODEX_HISTORY_NATIVE_EVIDENCE_OUT"); out != "" && !t.Failed() {
		data, err := json.MarshalIndent(map[string]any{"version": 1, "captured_at": time.Now().UTC(), "samples": evidence}, "", "  ")
		require(t, err)
		require(t, steerline.AtomicWrite(out, append(data, '\n')))
	}
}

type isolatedHistoryCommitFixture struct {
	domainStore                *sqlite.SQLiteStore
	directory, platformVersion string
}

func isolatedHistoryDigest(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

type isolatedHistoryStop struct{ process *os.Process }

func (p isolatedHistoryStop) Identity() executionbackend.ProcessIdentity {
	return executionbackend.ProcessIdentity{Kind: "fixture", PID: p.process.Pid, Created: "native-read-only-probe"}
}

func (p isolatedHistoryStop) Kill() { _ = p.process.Kill() }

func (p isolatedHistoryStop) Close() {}

func (f *isolatedHistoryCommitFixture) collectCodexHistoryPage(ctx context.Context, b codexworker.HistoryBinding) (bool, error) {
	path := filepath.Join(f.directory, isolatedHistoryDigest(b)+".json")
	if err := codexworker.WriteHistoryBinding(path, b, steerline.AtomicWrite); err != nil {
		return false, err
	}
	checkpoint, cpErr := f.domainStore.ConversationCheckpoint(ctx, b.ProjectID, b.ExecutionID, b.AttemptID, "codex-app-server", b.Identity())
	source := conversationtypes.ConversationHistorySource{Identity: b.Identity() + ":" + b.AttemptID, BindingPath: path, Workspace: b.Workspace, Event: b.Event()}
	batch, err := codexworker.NewConversationAdapter().ReadHistory(ctx, conversationtypes.ConversationHistoryReadRequest{Source: source, Cursor: checkpoint.Cursor, HasCheckpoint: cpErr == nil, PlatformVersion: f.platformVersion}, func(cmd *exec.Cmd) (executionbackend.StopHandle, error) {
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return isolatedHistoryStop{cmd.Process}, nil
	})
	if err != nil {
		return false, err
	}
	if len(batch.Writes) > 0 || batch.Cursor != checkpoint.Cursor {
		_, err = f.domainStore.CommitConversation(ctx, batch.Writes, executionmodel.ConversationCheckpoint{ProjectID: b.ProjectID, ExecutionID: b.ExecutionID, AttemptID: b.AttemptID, Source: "codex-app-server", SourceSegment: b.Identity(), SourceIdentity: b.Identity(), Generation: b.SessionID, Cursor: batch.Cursor, LocalSeq: batch.Sequence})
		if err != nil {
			return false, err
		}
	}
	if batch.Capabilities != nil {
		if err := f.domainStore.SaveConversationCapabilityReport(ctx, *batch.Capabilities); err != nil {
			return false, err
		}
	}
	return !batch.More && batch.Settled, nil
}

func TestCodexHistoryRealSourceReplacement(t *testing.T) {
	path := os.Getenv("STEERLINE_CODEX_HISTORY_REPLACEMENT_BINDINGS")
	if path == "" {
		t.Skip("set exact isolated source-replacement bindings to run acceptance")
	}
	raw, err := os.ReadFile(path)
	require(t, err)
	var bindings []codexworker.HistoryBinding
	require(t, json.Unmarshal(raw, &bindings))
	if len(bindings) != 2 || bindings[0].Backend != executionbackend.BackendLocal || bindings[1].Backend != executionbackend.BackendLocal || bindings[0].ProjectID != bindings[1].ProjectID || bindings[0].ExecutionID != bindings[1].ExecutionID || bindings[0].AttemptID != bindings[1].AttemptID || !strings.EqualFold(filepath.Clean(bindings[0].Workspace), filepath.Clean(bindings[1].Workspace)) || bindings[0].SessionID == bindings[1].SessionID {
		t.Fatal("replacement acceptance needs two distinct real sessions bound to one local attempt and workspace")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "replacement.db"))
	require(t, err)
	defer store.Close()
	service := &isolatedHistoryCommitFixture{domainStore: store, directory: t.TempDir()}
	for _, binding := range bindings {
		settled := false
		for i := 0; i < 8 && !settled; i++ {
			settled, err = service.collectCodexHistoryPage(ctx, binding)
			require(t, err)
		}
		if !settled {
			t.Fatalf("real source %s did not reach its final page", binding.SessionID)
		}
	}
	query := productquery.NewConversationService(store, bindings[0].ProjectID)
	filter := productdto.ConversationFilter{ExecutionID: bindings[0].ExecutionID, AttemptID: bindings[0].AttemptID, Source: "codex-app-server"}
	page, err := query.ReadConversation(ctx, filter, "", 100)
	require(t, err)
	sessions, gaps, finals := map[string]int{}, map[string]int{}, map[string]int{}
	for _, event := range page.Events {
		switch {
		case event.Type == productdto.ConversationSession:
			sessions[event.SessionID]++
		case event.Type == productdto.ConversationGap && event.Status == "source_changed":
			gaps[event.SessionID]++
		case event.Type == productdto.ConversationMessageComplete && event.Phase == productdto.MessagePhaseFinalAnswer:
			finals[event.SessionID]++
			if event.Body == nil {
				t.Fatal("real replacement lost a final body")
			}
			body, readErr := query.ReadMessageBody(ctx, *event.Body, 0, 256<<10)
			require(t, readErr)
			if !body.Complete || body.SHA256 != event.Body.SHA256 {
				t.Fatal("real replacement changed the final body")
			}
		}
	}
	for _, binding := range bindings {
		if sessions[binding.SessionID] != 1 || gaps[binding.SessionID] != 1 || finals[binding.SessionID] < 1 {
			t.Fatalf("real source %s was hidden or crossed: sessions=%v gaps=%v finals=%v", binding.SessionID, sessions, gaps, finals)
		}
		cp, cpErr := store.ConversationCheckpoint(ctx, binding.ProjectID, binding.ExecutionID, binding.AttemptID, "codex-app-server", binding.Identity())
		require(t, cpErr)
		if cp.Cursor == "" {
			t.Fatalf("real source %s has no own checkpoint", binding.SessionID)
		}
	}
	t.Logf("real replacement: events=%d, old_finals=%d, new_finals=%d, source_changed=2, independent_checkpoints=2", len(page.Events), finals[bindings[0].SessionID], finals[bindings[1].SessionID])
	before := len(page.Events)
	for _, binding := range bindings {
		_, err = service.collectCodexHistoryPage(ctx, binding)
		require(t, err)
	}
	page, err = query.ReadConversation(ctx, filter, "", 100)
	require(t, err)
	if len(page.Events) != before {
		t.Fatalf("real source replacement replay duplicated events: %d -> %d", before, len(page.Events))
	}
}
