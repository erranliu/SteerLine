package testenv

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBusinessDeadlineIsFrozenBeforeIndependentCleanup(t *testing.T) {
	for _, expired := range []bool{false, true} {
		name := "finished within deadline"
		if expired {
			name = "business already expired"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			deadline := time.Now().Add(time.Hour)
			if expired {
				deadline = time.Now().Add(-time.Second)
			}
			ctx, cancel := context.WithDeadline(context.Background(), deadline)
			defer cancel()
			s := &Scenario{Run: Run{Scratch: root}, ID: "deadline", Root: filepath.Join(root, "scratch"),
				Output: filepath.Join(root, "evidence"), Context: ctx, cancel: cancel, t: t}
			if err := os.MkdirAll(s.Root, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(s.Output, 0700); err != nil {
				t.Fatal(err)
			}
			err := s.FinishBody()
			if expired && !errors.Is(err, context.DeadlineExceeded) || !expired && err != nil {
				t.Fatalf("business deadline result: %v", err)
			}
			if s.Context.Err() == nil {
				t.Fatal("business I/O remained permitted during cleanup")
			}
			s.cleanup()
			raw, err := os.ReadFile(filepath.Join(s.Output, "scenario.json"))
			if err != nil {
				t.Fatal(err)
			}
			var record struct {
				Passed  bool      `json:"passed"`
				Cleanup bool      `json:"cleanup_complete"`
				Error   string    `json:"context_error"`
				BodyEnd time.Time `json:"body_finished_at"`
			}
			if err := json.Unmarshal(raw, &record); err != nil {
				t.Fatal(err)
			}
			if record.Passed == expired || !record.Cleanup || !record.BodyEnd.Equal(s.bodyFinishedAt) ||
				expired && record.Error != context.DeadlineExceeded.Error() || !expired && record.Error != "<nil>" {
				t.Fatalf("cleanup changed the business deadline result: %+v", record)
			}
			s.cleanup()
			raw, err = os.ReadFile(filepath.Join(s.Output, "scenario.json"))
			if err != nil {
				t.Fatal(err)
			}
			var repeated struct {
				Passed  bool      `json:"passed"`
				Error   string    `json:"context_error"`
				BodyEnd time.Time `json:"body_finished_at"`
			}
			if err := json.Unmarshal(raw, &repeated); err != nil {
				t.Fatal(err)
			}
			if repeated.Passed != record.Passed || repeated.Error != record.Error || !repeated.BodyEnd.Equal(record.BodyEnd) {
				t.Fatalf("repeated cleanup changed the frozen business result: before=%+v after=%+v", record, repeated)
			}
		})
	}
}
