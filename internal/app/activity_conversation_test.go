package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
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
	store := &conversationCandidateStore{candidate: runnerstate.Candidate{RunID: "run", StateDir: "state", Projection: &runnercontract.Run{
		Execution: runnercontract.Execution{AttemptID: "attempt", SessionID: "session", Generation: "generation"},
	}}}
	port := &runtimeProjectPort{config: Config{identity: "p", runtimeRunner: &runnerclient.Client{InstanceID: "instance", StateDir: "state"}}, store: store}
	for _, tc := range []struct {
		identity, code string
		query          url.Values
	}{
		{"other\x00task_candidate\x00candidate", "activity_not_found", nil},
		{"p\x00roadmap\x00candidate", "activity_not_found", nil},
		{"p\x00task_candidate\x00candidate", "invalid_reading_query", url.Values{"run_id": {"other"}}},
		{"p\x00task_candidate\x00candidate", "conversation_unsupported", nil},
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
	store := &conversationCandidateStore{candidate: runnerstate.Candidate{RunID: "run", StateDir: state, Projection: &runnercontract.Run{
		Execution: runnercontract.Execution{AttemptID: "attempt", SessionID: "session", Generation: "generation"},
	}}}
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
	value, err := port.Read(context.Background(), route, url.Values{"cursor": {"older-page"}})
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
	if _, err := port.Read(context.Background(), route, nil); err == nil {
		t.Fatal("accepted a page for a different session")
	}
}
