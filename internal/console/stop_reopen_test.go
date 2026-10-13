package console

import (
	"context"
	"strings"
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestPendingStopVerificationSurvivesJournalReopen(t *testing.T) {
	m, _ := ccFixture(t, 1)
	home := t.TempDir()
	profile, prefs, diagnostic, err := acquireViewProfile(home)
	if err != nil || diagnostic != "" {
		t.Fatal("initial view profile", diagnostic, err)
	}
	m.profile, m.prefs = profile, prefs
	defer m.Close()
	ref, client := m.projects[0].ref, m.projects[0].client
	id := strings.Repeat("a", 32)
	instance := m.projects[0].snapshot.Connection.Snapshot.Page.Service.InstanceID
	m.projects[0].actions[id] = &actionState{RequestID: id, Action: "stop_service", TargetInstance: instance, Status: "submitted"}
	m.projects[0].view.Outstanding[id] = "task:"
	m.projects[0].view.Drafts = map[string]CandidateDraft{"original-draft": {Note: "保留中文草稿"}}
	next, _ := m.Update(actionReceiptMsg{projectKey: ref.Key, identity: ref.Identity, requestID: id, receipt: &api.ControlReceipt{RequestID: id, InstanceID: instance, Action: "stop_service", Status: "applied", Code: "stop_exit_unverified"}})
	m = next.(Model)
	if m.projects[0].actions[id].Status != "pending_verification" {
		t.Fatal("applied did not remain pending exact exit proof")
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewModel(context.Background(), []ProjectRef{ref}, Options{ApplicationHome: home, Clients: map[string]ConsoleClient{ref.Key: client}})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	state := &reopened.projects[0]
	if reopened.prefsErr != "" || state.actions[id] == nil || state.actions[id].Status != "pending_verification" || state.actions[id].Resolved || state.view.Outstanding[id] != "task:" || state.view.Drafts["original-draft"].Note != "保留中文草稿" {
		t.Fatal("pending stop journal or draft was lost on reopening", reopened.prefsErr)
	}
	if err = reopened.saveProjectView(0); err != nil {
		t.Fatal("restored journal remained unwritable", err)
	}
}
