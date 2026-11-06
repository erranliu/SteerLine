package app

import (
	"context"
	"errors"
	"time"

	roadmap "github.com/erranliu/SteerLine/internal/roadmap/model"
)

func (p *roadmapPort) recordPublicationObservation(ctx context.Context, before, after roadmap.Record, err error) {
	if before.Publication == nil || errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	at, diagnostic, revision := time.Now().UTC(), "", after.Revision
	if err != nil {
		at, diagnostic, revision = time.Time{}, err.Error(), before.Revision
	}
	metadataCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 100*time.Millisecond)
	defer cancel()
	_ = p.store.RecordSourceObservation(metadataCtx, before.Project, "roadmap", before.ID, revision, at, diagnostic)
}
