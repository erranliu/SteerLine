package steerlineintegration

import (
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"testing"
)

func TestDetectNormalizer(t *testing.T) {
	if got := steerline.DetectNormalizer([]byte(`{"type":"session","version":3,"id":"x"}`), steerline.NewPiNormalizer(), codexworker.NewNormalizer()); got.Name() != "pi" {
		t.Fatalf("pi session header detected as %s", got.Name())
	}
	if got := steerline.DetectNormalizer([]byte(`{"type":"thread.started","thread_id":"t"}`), steerline.NewPiNormalizer(), codexworker.NewNormalizer()); got.Name() != "codex" {
		t.Fatalf("codex stream detected as %s", got.Name())
	}
}
