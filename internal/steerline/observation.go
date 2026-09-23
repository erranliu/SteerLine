package steerline

import (
	"strings"
	"time"
)

type SnapshotState string

const (
	SnapshotNone           SnapshotState = "none"
	SnapshotValid          SnapshotState = "valid"
	SnapshotStale          SnapshotState = "stale"
	SnapshotReadFailed     SnapshotState = "read-failed"
	SnapshotUnknownVersion SnapshotState = "unknown-version"
)

// ObservationSnapshot carries runtime data and the independent read/freshness
// state. Runtime.Service.Status remains the last service report; this type does
// not infer process liveness from a PID.
type ObservationSnapshot struct {
	Runtime        *Runtime
	State          SnapshotState
	SampledAt      time.Time
	HeartbeatAge   time.Duration
	FreshnessLimit time.Duration
	ClockSkew      bool
	Error          string
}

// ReadObservationSnapshot is a pure, read-only observation of one runtime file.
// nowFn is injectable so freshness and future-heartbeat behavior stay testable.
func ReadObservationSnapshot(path, heartbeat string, nowFn func() time.Time) ObservationSnapshot {
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn().UTC()
	limit := freshnessLimit(heartbeat)
	rt, err := LoadRuntime(path)
	if err != nil {
		state := SnapshotReadFailed
		if strings.Contains(err.Error(), "unsupported runtime schema") {
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
