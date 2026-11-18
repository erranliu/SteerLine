package refactoring

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	ref "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func TestDirectionsAreSharedByHomeAndVersioned(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	writer := sqlite.RefactoringDirections{Home: home}
	reader := sqlite.RefactoringDirections{Home: home}
	if list, err := reader.List(ctx); err != nil || len(list) != 0 {
		t.Fatal(list, err)
	}
	builtIn, err := DefaultDirection()
	if err != nil || builtIn.Name != ref.DefaultDirectionName || builtIn.Prompt == "" {
		t.Fatal(builtIn, err)
	}
	const original = "优先处理共享夹具。  \r\n"
	d, err := writer.Change(ctx, ref.DirectionChange{Action: "create", Name: "  Fixtures  ", Prompt: original})
	if err != nil || d.Name != "Fixtures" {
		t.Fatal(d, err)
	}
	if rows, err := reader.List(ctx); err != nil || len(rows) != 1 || rows[0] != d || d.Prompt != original {
		t.Fatal(rows, err)
	}
	for _, name := range []string{"fixtures", "  " + ref.DefaultDirectionName + "  "} {
		if _, err := writer.Change(ctx, ref.DirectionChange{Action: "create", Name: name, Prompt: original}); !errors.Is(err, ref.ErrDirectionDuplicate) {
			t.Fatal(name, err)
		}
	}
	if _, err := writer.Change(ctx, ref.DirectionChange{Action: "delete", ID: builtIn.ID, Revision: builtIn.Revision}); !errors.Is(err, ref.ErrDirectionInvalid) {
		t.Fatal("built-in deletion", err)
	}
	updated, err := writer.Change(ctx, ref.DirectionChange{Action: "update", ID: d.ID, Revision: d.Revision, Name: "新名称", Prompt: "新版正文"})
	if err != nil || updated.Revision == d.Revision {
		t.Fatal(updated, err)
	}
	start := ref.Start{Direction: d.ID, DirectionRevision: d.Revision, DirectionPrompt: original}
	if _, err := ResolveDirection(ctx, start, reader.Read); !errors.Is(err, ErrDirectionChanged) {
		t.Fatal("stale version", err)
	}
	if _, err := writer.Change(ctx, ref.DirectionChange{Action: "delete", ID: d.ID, Revision: d.Revision}); !errors.Is(err, ref.ErrDirectionConflict) {
		t.Fatal("stale delete", err)
	}
	if _, err := writer.Change(ctx, ref.DirectionChange{Action: "delete", ID: updated.ID, Revision: updated.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Read(ctx, d.ID); !errors.Is(err, ref.ErrDirectionMissing) {
		t.Fatal("deleted direction", err)
	}
}

func TestStartFreezesEditedDirectionBeforeBackgroundChecks(t *testing.T) {
	ctx := context.Background()
	s, _, seed := fixture(t)
	store := sqlite.RefactoringDirections{Home: t.TempDir()}
	d, err := store.Change(ctx, ref.DirectionChange{Action: "create", Name: "共享方向", Prompt: "模板正文"})
	if err != nil {
		t.Fatal(err)
	}
	s.ResolveDirection = func(ctx context.Context, in ref.Start) (ref.Direction, error) {
		return ResolveDirection(ctx, in, store.Read)
	}
	in := seed.Input
	in.Direction, in.DirectionRevision, in.DirectionPrompt = d.ID, d.Revision, "当次正文  \r\n"
	cmd := ref.Command{ID: ref.ID(), Action: "refactoring_start", Target: s.Project, Expected: 1, Raw: json.RawMessage(`{}`)}
	r, err := s.Command(ctx, cmd, &in)
	if err != nil || r.Frozen.Direction.Name != d.Name || r.Frozen.Direction.Prompt != in.DirectionPrompt {
		t.Fatal(r, err)
	}
	if current, err := store.Read(ctx, d.ID); err != nil || current != d {
		t.Fatal("one-time edit altered saved direction", current, err)
	}
	if _, err := store.Change(ctx, ref.DirectionChange{Action: "delete", ID: d.ID, Revision: d.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Command(ctx, cmd, &in); err != nil {
		t.Fatal("replaying accepted command after deletion", err)
	}
	r = tickUntil(t, s, r.ID, func(r ref.Record) bool { return r.Phase == "first" })
	if r.Frozen.Direction.Name != d.Name || r.Frozen.Direction.Prompt != in.DirectionPrompt {
		t.Fatal(r.Frozen.Direction)
	}
}
