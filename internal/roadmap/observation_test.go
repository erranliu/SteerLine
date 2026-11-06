package roadmap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/roadmap/model"
)

type roadmapObservationStore struct {
	*memoryRoadmaps
	calls      int
	at         time.Time
	diagnostic string
}

func (s *roadmapObservationStore) RecordSourceObservation(_ context.Context, project, source, id string, _ uint64, at time.Time, diagnostic string) error {
	if project != "p" || source != "roadmap" || id == "" {
		return errors.New("wrong observation identity")
	}
	s.calls++
	s.at, s.diagnostic = at, diagnostic
	return nil
}

func TestRoadmapObservationTracksOriginalGenerationWithoutInventingSamples(t *testing.T) {
	s, store, runner, input := generationSetup(t)
	metadata := &roadmapObservationStore{memoryRoadmaps: store}
	s.Store = metadata
	r, err := s.Parse(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Observe(context.Background(), r.ID)
	if err != nil || metadata.calls != 1 || metadata.at.IsZero() || metadata.diagnostic != "" {
		t.Fatalf("successful inspection not recorded: %+v %v", metadata, err)
	}
	runner.hidden = true
	_, err = s.Observe(context.Background(), r.ID)
	if err != nil || metadata.calls != 1 {
		t.Fatalf("missing Run invented sample: %+v %v", metadata, err)
	}
	runner.hidden = false
	runner.run.Correlation = nil
	_, err = s.Observe(context.Background(), r.ID)
	if err == nil || metadata.calls != 2 || !metadata.at.IsZero() || metadata.diagnostic == "" {
		t.Fatalf("invalid inspection not recorded: %+v %v", metadata, err)
	}
	// Metadata-only cancellation must not report a source outage.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.recordObservation(ctx, model.Record{Project: "p", ID: r.ID}, r, false, ctx.Err())
	if metadata.calls != 2 {
		t.Fatal("shutdown became observation failure")
	}
}
