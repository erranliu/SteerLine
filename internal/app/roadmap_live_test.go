//go:build e2e

package app

import (
	"os"
	"testing"
)

// Explicit-only native model run; provider and Git remote remain isolated.
func TestRoadmapLocalCodexPromotion(t *testing.T) {
	roadmapPromotionChain(t, true)
}

// Reads the real prepared project commit into an owned temporary Git/SQLite
// scene. No installed service, default remote branch or existing Task is changed.
func TestRoadmapPreparedSteerlineTestLocalCodexPromotion(t *testing.T) {
	if os.Getenv("STEERLINE_ROADMAP_SOURCE_REPOSITORY") == "" {
		t.Fatal("STEERLINE_ROADMAP_SOURCE_REPOSITORY required")
	}
	roadmapPromotionChain(t, true)
}
