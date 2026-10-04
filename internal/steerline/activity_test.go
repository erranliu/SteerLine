package steerline

import (
	"strings"
	"testing"
	"time"
)

func TestRenderActivity(t *testing.T) {
	ev := ActivityEvent{TS: time.Now(), Source: "fixture-stream", Kind: "command", Status: "completed", Title: "go test", ElapsedMS: 13200, Meta: map[string]any{"exit_code": 0}}
	s := RenderActivity(ev, RenderOptions{})
	for _, want := range []string{"command", "go test", "13.2s", "exit 0"} {
		if !strings.Contains(s, want) {
			t.Fatalf("render %q missing %q", s, want)
		}
	}
}
