package steerline

import (
"github.com/erranliu/SteerLine/internal/steerline/contracts"
 "github.com/erranliu/SteerLine/internal/execution"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
)

type ObservationSnapshot = clientcontracts.ObservationSnapshot

const (
	SnapshotNone           = productapi.SnapshotNone
	SnapshotValid          = productapi.SnapshotValid
	SnapshotStale          = productapi.SnapshotStale
	SnapshotReadFailed     = productapi.SnapshotReadFailed
	SnapshotUnknownVersion = productapi.SnapshotUnknownVersion
)

// ReadObservationSnapshot reads the committed SQLite service checkpoint.
// nowFn keeps freshness and future-heartbeat behavior testable.
func ReadObservationSnapshot(c execution.Config, nowFn func() time.Time) ObservationSnapshot {
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn().UTC()
	limit := freshnessLimit(c.Heartbeat)
	if _, err := os.Stat(filepath.Join(c.RunDir, "steerline.db")); errors.Is(err, os.ErrNotExist) {
		return ObservationSnapshot{State: SnapshotNone, SampledAt: now, FreshnessLimit: limit}
	} else if err != nil {
		return ObservationSnapshot{State: SnapshotReadFailed, SampledAt: now, FreshnessLimit: limit, Error: err.Error()}
	}
	rt, err := loadObservedRuntime(c)
	return ObserveRuntimeSnapshot(c, rt, err, now)
}

// ObserveRuntimeSnapshot applies freshness to an already sampled database
// runtime. It does not read storage or infer whether the service process lives.
func ObserveRuntimeSnapshot(c execution.Config, rt *Runtime, err error, sampledAt time.Time) ObservationSnapshot {
	now := sampledAt.UTC()
	limit := freshnessLimit(c.Heartbeat)
	if err != nil {
		state := SnapshotReadFailed
		if strings.Contains(err.Error(), "schema") {
			state = SnapshotUnknownVersion
		}
		return ObservationSnapshot{State: state, SampledAt: now, FreshnessLimit: limit, Error: err.Error()}
	}
	if rt == nil {
		return ObservationSnapshot{State: SnapshotNone, SampledAt: now, FreshnessLimit: limit}
	}
	obs := ObservationSnapshot{Runtime: rt, State: SnapshotValid, SampledAt: now, FreshnessLimit: limit}
	if rt.HeartbeatAt.IsZero() {
		obs.State = SnapshotStale
		return obs
	}
	obs.HeartbeatAge = now.Sub(rt.HeartbeatAt.UTC())
	if obs.HeartbeatAge < 0 {
		obs.ClockSkew = true
		obs.State = SnapshotStale
		return obs
	}
	if obs.HeartbeatAge > limit {
		obs.State = SnapshotStale
	}
	return obs
}

func freshnessLimit(heartbeat string) time.Duration {
	limit := 15 * time.Second
	if d, err := time.ParseDuration(heartbeat); err == nil && d > 0 && 3*d > limit {
		limit = 3 * d
	}
	return limit
}
