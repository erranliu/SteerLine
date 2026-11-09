package app

import (
	"context"
	"time"
)

func (m *projectMaintenance) runPreparation(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		_ = m.refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
