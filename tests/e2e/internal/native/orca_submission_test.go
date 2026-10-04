package native

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func pasteScreen(t *testing.T, handle, source, draft string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"ok": true, "result": map[string]any{"terminal": map[string]any{"handle": handle, "source": source, "draft": draft}}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestOrcaSubmissionWaitsForOriginalDraftToClear(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls := 0
	cleared := pasteScreen(t, "original", "screen", "")
	raw, err := waitForOrcaPasteSubmission(ctx, "original", func(context.Context) ([]byte, error) {
		calls++
		if calls < 3 {
			return pasteScreen(t, "original", "screen", "[Pasted Content 1985 chars]c dispa"), nil
		}
		return cleared, nil
	})
	if err != nil || calls != 3 || string(raw) != string(cleared) {
		t.Fatalf("accepted but pending input was mistaken for submission: calls=%d err=%v", calls, err)
	}
}

func TestOrcaSubmissionRejectsUnsubmittedOrReplacedScreen(t *testing.T) {
	for _, tc := range []struct {
		name, handle, source string
		timeout              bool
	}{
		{"pending original", "original", "screen", true},
		{"replacement terminal", "replacement", "screen", false},
		{"historical output", "original", "transcript", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			pending := pasteScreen(t, tc.handle, tc.source, "[Pasted Content 1985 chars]")
			raw, err := waitForOrcaPasteSubmission(ctx, "original", func(context.Context) ([]byte, error) { return pending, nil })
			if err == nil || string(raw) != string(pending) || errors.Is(err, context.DeadlineExceeded) != tc.timeout {
				t.Fatalf("unverified submission lost original evidence or passed: %v", err)
			}
		})
	}
}
