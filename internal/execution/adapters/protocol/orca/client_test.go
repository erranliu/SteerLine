package orca

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

func fixtureClient(result any, runtime string) Client {
	return New(Config{Output: func(context.Context, time.Duration, ...string) ([]byte, error) {
		return json.Marshal(map[string]any{"ok": true, "result": result, "_meta": map[string]string{"runtimeId": runtime}})
	}})
}

func TestBoundTerminalRejectsRuntimeAndIncarnationReplacement(t *testing.T) {
	p := b.ProcessIdentity{Terminal: "original", RuntimeID: "runtime", Incarnation: "incarnation"}
	for _, field := range []string{"valid", "runtime", "handle", "incarnation"} {
		t.Run(field, func(t *testing.T) {
			terminal := Terminal{Handle: p.Terminal, IncarnationID: p.Incarnation}
			runtime := p.RuntimeID
			switch field {
			case "runtime":
				runtime = "replacement"
			case "handle":
				terminal.Handle = "replacement"
			case "incarnation":
				terminal.IncarnationID = "replacement"
			}
			_, err := fixtureClient(map[string]any{"terminal": terminal}, runtime).InspectBoundTerminal(context.Background(), b.ExecutionTarget{}, p)
			if (err == nil) != (field == "valid") {
				t.Fatalf("%s: %v", field, err)
			}
		})
	}
}

func TestTerminalCloseRequiresExactRuntimeHandleAndPtyProof(t *testing.T) {
	for _, field := range []string{"confirmed", "no-proof", "runtime", "handle"} {
		t.Run(field, func(t *testing.T) {
			runtime, handle := "runtime", "original"
			if field == "runtime" {
				runtime = "replacement"
			}
			if field == "handle" {
				handle = "replacement"
			}
			receipt, err := fixtureClient(map[string]any{"close": map[string]any{"handle": handle, "ptyKilled": field != "no-proof"}}, runtime).CloseTerminalChecked(context.Background(), "original", "runtime")
			if field == "runtime" || field == "handle" {
				if err == nil {
					t.Fatal("foreign close accepted")
				}
				return
			}
			if err != nil || (receipt.Disposition == "confirmed") != (field == "confirmed") {
				t.Fatal(receipt, err)
			}
		})
	}
	_, err := ParseEnvelope([]byte(`{"ok":false,"error":{"message":"unreachable"}}`))
	if err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatal("native rejection lost", err)
	}
}
