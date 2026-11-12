package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/query"
	refactoring "github.com/erranliu/SteerLine/internal/refactoring/model"
	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	advisory "github.com/erranliu/SteerLine/internal/steeragentbridge/suggestion"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

type conversationCandidateStore struct {
	taskStorage
	candidate runnerstate.Candidate
	reads     int
}

func (s *conversationCandidateStore) Candidate(_ context.Context, project, id string) (runnerstate.Candidate, error) {
	s.reads++
	if project != "p" || id != "candidate" {
		return runnerstate.Candidate{}, errors.New("wrong candidate binding")
	}
	return s.candidate, nil
}

func TestActivityConversationRequiresOriginalCandidateAndAdvertisedRunner(t *testing.T) {
	cli := &runnerclient.Client{InstanceID: "instance", StateDir: "state"}
	store := &conversationCandidateStore{candidate: runnerstate.Candidate{
		RunID: "run", RunnerBuild: cli.BuildPin(), Projection: conversationRun("run", "attempt"),
	}}
	port := &runtimeProjectPort{config: Config{identity: "p", runtimeRunner: cli}, store: store}
	target := conversationTarget("candidate", "", "run", store.candidate.Projection, cli.StateDir)
	for _, tc := range []struct {
		identity, code string
		query          url.Values
	}{
		{"other\x00task_candidate\x00candidate", "activity_not_found", nil},
		{"p\x00unknown\x00candidate", "activity_not_found", nil},
		{"p\x00task_candidate\x00candidate", "invalid_reading_query", url.Values{"run_id": {"other"}}},
		{"p\x00task_candidate\x00candidate", "invalid_reading_query", url.Values{"cursor": {"other"}}},
		{"p\x00task_candidate\x00candidate", "conversation_unsupported", url.Values{"session": {target.ID}}},
	} {
		route := "/activities/" + base64.RawURLEncoding.EncodeToString([]byte(tc.identity)) + "/conversation"
		_, err := port.Read(context.Background(), route, tc.query)
		var clientError *httpclient.ClientError
		if !errors.As(err, &clientError) || clientError.Code != tc.code {
			t.Fatalf("expected %s, got %v", tc.code, err)
		}
	}
	if store.reads != 1 {
		t.Fatalf("invalid requests reached the candidate store: %d reads", store.reads)
	}
}

func TestActivityConversationReadsOnePageFromPinnedRunner(t *testing.T) {
	exe, err := os.Executable()
	require(t, err)
	raw, err := os.ReadFile(exe)
	require(t, err)
	digest := sha256.Sum256(raw)
	state := t.TempDir()
	cli := &runnerclient.Client{
		Executable: exe, StateDir: state, InstanceID: "runner-instance", BuildVersion: "fixture",
		SHA256: hex.EncodeToString(digest[:]), ResourceDigest: strings.Repeat("b", 64),
		FullBindingCommands: []string{runnerclient.NativeConversationHistory},
	}
	originalBuild := cli.BuildPin()
	originalBuild.Version = "earlier-build"
	originalBuild.SHA256 = strings.Repeat("a", 64)
	store := &conversationCandidateStore{candidate: runnerstate.Candidate{
		RunID: "run", RunnerBuild: originalBuild, Projection: conversationRun("run", "attempt"),
	}}
	port := &runtimeProjectPort{config: Config{identity: "p", runtimeRunner: cli}, store: store}
	id := base64.RawURLEncoding.EncodeToString([]byte("p\x00task_candidate\x00candidate"))
	route := "/activities/" + id + "/conversation"
	calls := filepath.Join(state, "calls.txt")
	t.Setenv("STEERLINE_USAGE_QUERY_CALLS", calls)
	publish := func(session string) {
		page := runnerclient.NativeConversationPage{
			NativeSessionTarget: runnerclient.NativeSessionTarget{RunID: "run", AttemptID: "attempt", SessionID: session, Generation: "generation"},
			Messages:            []runnerclient.NativeConversationMessage{{ID: "older", Role: "assistant", Text: "Earlier assistant text"}},
		}
		fixture, err := json.Marshal(map[string]any{runnerclient.NativeConversationHistory: page})
		require(t, err)
		t.Setenv("STEERLINE_USAGE_QUERY_FIXTURE", string(fixture))
	}
	publish("session")
	manifest, err := port.Read(context.Background(), route, nil)
	require(t, err)
	targets := manifest.(activityConversations).Targets
	if len(targets) != 1 || targets[0].State != "available" {
		t.Fatalf("unexpected targets: %+v", targets)
	}
	if _, err := os.Stat(calls); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("listing targets called Runner: %v", err)
	}
	session := targets[0].ID
	value, err := port.Read(context.Background(), route, url.Values{"session": {session}, "cursor": {"older-page"}})
	require(t, err)
	payload, err := json.Marshal(value)
	require(t, err)
	if !strings.Contains(string(payload), id) || !strings.Contains(string(payload), "Earlier assistant text") {
		t.Fatalf("original conversation page not returned: %s", payload)
	}
	invocation, err := os.ReadFile(calls)
	require(t, err)
	for _, option := range []string{"--expected-instance-id\nrunner-instance", "--run-id\nrun", "--attempt-id\nattempt", "--session-id\nsession", "--generation\ngeneration", "--after\nolder-page"} {
		if !strings.Contains(string(invocation), option) {
			t.Fatalf("missing original CLI binding: %s", option)
		}
	}
	if strings.Count(string(invocation), runnerclient.NativeConversationHistory+"\n") != 1 || strings.Contains(string(invocation), "--input") {
		t.Fatal("reading a page performed extra calls or sent input")
	}
	publish("another-session")
	if _, err := port.Read(context.Background(), route, url.Values{"session": {session}}); err == nil {
		t.Fatal("accepted a page for a different session")
	}
	store.candidate.RunnerBuild.StateDir = filepath.Join(state, "other-namespace")
	_, err = port.Read(context.Background(), route, url.Values{"session": {session}})
	var clientError *httpclient.ClientError
	if !errors.As(err, &clientError) || clientError.Code != "reading_unavailable" {
		t.Fatalf("accepted an unrelated Runner namespace: %v", err)
	}
}

func conversationRun(id, attempt string) *runnercontract.Run {
	return &runnercontract.Run{RunID: id, Execution: runnercontract.Execution{
		AttemptID: attempt, SessionID: "session", Generation: "generation",
	}}
}

type conversationSourceStore struct {
	conversationCandidateStore
	pin runnercontract.BuildPin
}

func (s *conversationSourceStore) Environment(_ context.Context, project, id string) (env.Record, error) {
	if project != "p" || id != "environment" {
		return env.Record{}, env.ErrNotFound
	}
	return env.Record{ID: id, Project: project, Build: s.pin, Rounds: []env.Round{
		{Number: 1, RunID: "env-1", Run: conversationRun("env-1", "first")},
		{Number: 2, RunID: "env-2", Run: conversationRun("env-2", "second")},
	}}, nil
}

func (s *conversationSourceStore) Roadmap(_ context.Context, project, id string) (roadmap.Record, error) {
	if project != "p" || id != "documents" {
		return roadmap.Record{}, roadmap.ErrNotFound
	}
	return roadmap.Record{ID: id, Project: project, Runner: s.pin, Purpose: "documents",
		State: "admitted", Publication: &roadmap.Publication{RequestID: "publish", State: "merged"}, AdmissionRequest: "admit", Generations: []roadmap.Generation{
			{ID: "gen-1", Key: "spec", RunID: "roadmap-1", Projection: conversationRun("roadmap-1", "first")},
			{ID: "gen-2", Key: "spec", RunID: "roadmap-2", Projection: conversationRun("roadmap-2", "second")},
		}}, nil
}

func (s *conversationSourceStore) LoadAdvisory(_ context.Context, project, id string) ([]byte, uint64, error) {
	if project != "p" || id != "advisory" {
		return nil, 0, sql.ErrNoRows
	}
	raw, err := json.Marshal(advisory.Record{ID: id, Project: project, Revision: 1, RunID: "advice", Projection: conversationRun("advice", "first"), Runner: s.pin})
	return raw, 1, err
}

func (s *conversationSourceStore) Refactoring(_ context.Context, project, id string) (refactoring.Record, error) {
	if project != "p" || id != "refactoring" {
		return refactoring.Record{}, refactoring.ErrNotFound
	}
	return refactoring.Record{ID: id, Project: project, Frozen: refactoring.Frozen{Runner: s.pin}, Operations: []refactoring.Operation{
		{ID: "operation-1", Purpose: "execute", RunID: "refactor-1", Run: conversationRun("refactor-1", "first")},
		{ID: "operation-2", Purpose: "audit", RunID: "refactor-2"},
	}}, nil
}

func (s *conversationSourceStore) LookupCommand(_ context.Context, project, id string) (ports.CommandRecord, bool, error) {
	if project != "p" || id != "baseline" {
		return ports.CommandRecord{}, false, nil
	}
	return ports.CommandRecord{Kind: "project_maintenance", Status: "applied",
		Payload: []byte(`{"action":"maintenance_check"}`), Result: []byte(`{"diagnostic":"已检查"}`)}, true, nil
}

func TestActivityConversationSourcesAndTargetIsolation(t *testing.T) {
	cli := &runnerclient.Client{InstanceID: "fixture", StateDir: "state", FullBindingCommands: []string{runnerclient.NativeConversationHistory}}
	store := &conversationSourceStore{pin: cli.BuildPin()}
	store.candidate = runnerstate.Candidate{RunID: "candidate-run", Projection: conversationRun("candidate-run", "first"), RunnerBuild: store.pin}
	port := &runtimeProjectPort{config: Config{identity: "p", runtimeRunner: cli}, store: store}
	tests := map[string]struct {
		id   string
		runs []string
	}{
		"task_candidate":        {"candidate", []string{"candidate-run"}},
		"environment_detection": {"environment", []string{"env-1", "env-2"}},
		"roadmap":               {"documents", []string{"roadmap-1", "roadmap-2"}},
		"main_agent_advisory":   {"advisory", []string{"advice"}},
		"refactoring":           {"refactoring", []string{"refactor-1", "refactor-2"}},
		"baseline_check":        {"baseline", nil},
	}
	if len(activityConversationSources) != len(query.AsyncActivitySources) || len(tests) != len(query.AsyncActivitySources) {
		t.Fatal("activity registry and conversation coverage differ")
	}
	route := func(source, id string) string {
		return "/activities/" + base64.RawURLEncoding.EncodeToString([]byte("p\x00"+source+"\x00"+id)) + "/conversation"
	}
	for _, source := range query.AsyncActivitySources {
		t.Run(source, func(t *testing.T) {
			tc, ok := tests[source]
			if !ok {
				t.Fatal("activity source has no conversation coverage")
			}
			value, err := port.Read(context.Background(), route(source, tc.id), nil)
			require(t, err)
			list := value.(activityConversations)
			if len(list.Targets) != len(tc.runs) {
				t.Fatalf("unexpected targets: %+v", list)
			}
			for i, target := range list.Targets {
				wantState := "available"
				if source == "refactoring" && i == 1 {
					wantState = "unbound"
				}
				if target.RunID != tc.runs[i] || target.State != wantState {
					t.Fatalf("target %d has wrong scope or state: %+v", i, target)
				}
			}
			if source == "roadmap" && (list.Operation == nil || list.Operation.RequestID != "admit") {
				t.Fatalf("generation and admission lost their separate bindings: %+v", list)
			}
			if source == "baseline_check" && (list.Operation == nil || list.Operation.RequestID != "baseline" || list.Operation.Diagnostic != "已检查") {
				t.Fatalf("missing check result: %+v", list)
			}
		})
	}
	value, err := port.Read(context.Background(), route("environment_detection", "environment"), nil)
	require(t, err)
	target := value.(activityConversations).Targets[0]
	_, err = port.Read(context.Background(), route("roadmap", "documents"), url.Values{"session": {target.ID}, "cursor": {"older"}})
	var clientError *httpclient.ClientError
	if !errors.As(err, &clientError) || clientError.Code != "conversation_changed" {
		t.Fatalf("accepted another activity's conversation: %v", err)
	}
}

func TestActivityReadingRetriesOnlyTemporaryContention(t *testing.T) {
	locked := &ports.StorageError{Code: ports.StorageLocked}
	calls := 0
	value, err := retryActivityReading(context.Background(), func() (string, error) {
		calls++
		if calls == 1 {
			return "", locked
		}
		return "saved interaction", nil
	})
	if err != nil || value != "saved interaction" || calls != 2 {
		t.Fatalf("temporary read contention was not recovered: %d %q %v", calls, value, err)
	}

	calls = 0
	_, err = retryActivityReading(context.Background(), func() (string, error) {
		calls++
		return "", sql.ErrNoRows
	})
	if !errors.Is(err, sql.ErrNoRows) || calls != 1 {
		t.Fatalf("a missing record was retried: %d %v", calls, err)
	}
}
