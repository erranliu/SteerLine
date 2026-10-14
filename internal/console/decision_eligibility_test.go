package console

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

type httpEligibilityClient struct {
	ConsoleClient
	url string
}

func (c httpEligibilityClient) DecisionEligibility(ctx context.Context, _ *api.CandidateSummary) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	response, err := http.DefaultClient.Do(req)
	if err == nil {
		response.Body.Close()
	}
	return err
}

func TestDecisionEligibilityHTTPDoesNotBlockUI(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { close(entered); <-release; w.WriteHeader(200) }))
	defer server.Close()
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	m, _ := ccFixture(t, 1)
	m.projects[0].client = httpEligibilityClient{ConsoleClient: m.projects[0].client, url: server.URL}
	type result struct {
		model   tea.Model
		command tea.Cmd
	}
	opened := make(chan result, 1)
	go func() { next, cmd := m.Update(runeKey('a')); opened <- result{next, cmd} }()
	select {
	case r := <-opened:
		m = r.model.(Model)
		if r.command == nil || m.answer != nil {
			t.Fatal("eligibility did not defer opening")
		}
		completed := make(chan tea.Msg, 1)
		go func() { completed <- r.command() }()
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("eligibility HTTP query did not start")
		}
		pending := m
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = next.(Model)
		if m.answerCheck != nil {
			t.Fatal("Escape did not cancel pending open")
		}
		// Quit remains available while the HTTP request is blocked.
		_, quit := m.Update(runeKey('q'))
		if quit == nil {
			t.Fatal("blocked HTTP query disabled quit")
		}
		if _, ok := quit().(tea.QuitMsg); !ok {
			t.Fatal("quit was not processed")
		}
		releaseOnce.Do(func() { close(release) })
		var reply tea.Msg
		select {
		case reply = <-completed:
		case <-time.After(2 * time.Second):
			t.Fatal("HTTP qualification did not finish")
		}
		// A completed check must also be rejected after target version changes.
		pending.selectedRow(0).run.ControlRevision++
		next, _ = pending.Update(reply)
		pending = next.(Model)
		if pending.answer != nil {
			t.Fatal("stale eligibility opened an answer")
		}
		// Delivery after cancellation must not reopen the dialog.
		next, _ = m.Update(reply)
		m = next.(Model)
		if m.answer != nil {
			t.Fatal("late eligibility reopened a cancelled answer")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("HTTP eligibility blocked the UI update handler")
	}
}
