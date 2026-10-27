//go:build e2e

package app

import "testing"

// Explicit-only native model run; provider and Git remote remain isolated.
func TestRoadmapLocalCodexPromotion(t *testing.T) {
	roadmapPromotionChain(t, true)
}
