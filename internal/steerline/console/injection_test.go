package console

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
)

func TestCM72RequiresExactInjectedProjectAndQuitOnlyClosesClient(t *testing.T) {
	ref := testProject("injected")
	opts := Options{ApplicationHome: t.TempDir()}
	if _, err := NewModel(context.Background(), []ProjectRef{ref}, opts); err == nil {
		t.Fatal("missing client admitted")
	}
	client := &testClient{ref: testProject("other")}
	opts.Clients = map[string]api.ConsoleClient{ref.Key: client}
	if _, err := NewModel(context.Background(), []ProjectRef{ref}, opts); err == nil {
		t.Fatal("foreign client admitted")
	}
	client.ref, client.control = ref, &testControlClient{}
	model, err := NewModel(context.Background(), []ProjectRef{ref}, opts)
	if err != nil {
		t.Fatal(err)
	}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("quit command missing")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("quit sent a product mutation")
	}
	if err := updated.(Model).Close(); err != nil {
		t.Fatal(err)
	}
	if client.closed != 1 || client.control.calls != 0 || client.control.decisionCalls != 0 {
		t.Fatal("exit changed service/worker")
	}
	invalid := testProject("invalid")
	invalid.Error = "configuration unavailable"
	model, err = NewModel(context.Background(), []ProjectRef{invalid}, Options{ApplicationHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer model.Close()
	msg := model.pollAction(0, &actionState{RequestID: "original", UpdatedAt: time.Now()})()
	result := msg.(actionReceiptMsg)
	if result.requestID != "original" || result.err == nil {
		t.Fatal("invalid project lost pending receipt identity")
	}
}
