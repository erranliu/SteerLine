package app

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/environmentadaptation"
	envmodel "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/refactoring"
	refactoringmodel "github.com/erranliu/SteerLine/internal/refactoring/model"
	"github.com/erranliu/SteerLine/internal/roadmap"
	roadmapmodel "github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/steeragent/suggestion/contract"
	"github.com/erranliu/SteerLine/internal/steeragentbridge/suggestion"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func TestActivityAbandonmentStrategyCoversFormalSources(t *testing.T) {
	store, err := sqlite.OpenSQLiteStore(filepath.Join(t.TempDir(), "domain.db"))
	require(t, err)
	t.Cleanup(func() { require(t, store.Close()) })
	const project = "project"
	id, request := strings.Repeat("a", 32), strings.Repeat("b", 32)
	raw := []byte(`{}`)

	// Wire only the expected owner. Its missing-record error proves the stop
	// callback entered that owner without sending anything to Runner.
	strategies := map[string]struct {
		config           Config
		ownerNotFoundErr error // nil means archive only
	}{
		"baseline_check": {},
		"task_candidate": {},
		"environment_detection": {
			config:           Config{runtimeEnvironment: &environmentPort{service: &environmentadaptation.Service{Project: project, Store: store}}},
			ownerNotFoundErr: envmodel.ErrNotFound,
		},
		"roadmap": {
			config:           Config{runtimeRoadmaps: &roadmapPort{service: &roadmap.Service{Project: project, Store: store}}},
			ownerNotFoundErr: roadmapmodel.ErrNotFound,
		},
		"main_agent_advisory": {
			config:           Config{runtimeContinuationSuggestion: &continuationSuggestionPort{service: &suggestionbridge.Service{Project: project, Store: store}}},
			ownerNotFoundErr: contract.ErrNotFound,
		},
		"refactoring": {
			config:           Config{runtimeRefactoring: &refactoringPort{service: &refactoring.Service{Project: project, Store: store}}},
			ownerNotFoundErr: refactoringmodel.ErrNotFound,
		},
	}
	for _, source := range query.AsyncActivitySources {
		strategy, ok := strategies[source]
		if !ok {
			t.Errorf("formal source %q has no abandonment strategy coverage or is registered twice", source)
			continue
		}
		delete(strategies, source)
		t.Run(source, func(t *testing.T) {
			port := &runtimeProjectPort{config: strategy.config}
			stop, err := port.activityStop(source, id, request, raw)
			require(t, err)
			if strategy.ownerNotFoundErr == nil {
				if stop != nil {
					t.Fatal("archive-only activity registered a stop callback")
				}
				return
			}
			if stop == nil {
				t.Fatal("activity must delegate stop to its owner")
			}
			if err := stop(t.Context()); !errors.Is(err, strategy.ownerNotFoundErr) {
				t.Fatalf("stop callback returned %v, want owner error %v", err, strategy.ownerNotFoundErr)
			}
			unavailable := &runtimeProjectPort{}
			if stop, err := unavailable.activityStop(source, id, request, raw); stop != nil || err == nil {
				t.Fatal("unavailable owner must not fall back to archive only")
			}
		})
	}
	for source := range strategies {
		t.Errorf("abandonment strategy covers unregistered source %q", source)
	}
}
