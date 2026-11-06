package roadmap

import (
	"context"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/roadmap/model"
)

func (s *Service) recordObservation(ctx context.Context, before, after model.Record, observed bool, err error) {
	if errors.Is(ctx.Err(), context.Canceled) || !observed && err == nil {
		return
	}
	recorder, ok := s.Store.(interface {
		RecordSourceObservation(context.Context, string, string, string, uint64, time.Time, string) error
	})
	if !ok {
		return
	}
	at, diagnostic, revision := time.Now().UTC(), "", after.Revision
	if err != nil {
		at, diagnostic = time.Time{}, err.Error()
	}
	if err != nil || revision == 0 {
		revision = before.Revision
	}
	metadataCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 100*time.Millisecond)
	defer cancel()
	_ = recorder.RecordSourceObservation(metadataCtx, before.Project, "roadmap", before.ID, revision, at, diagnostic)
}
