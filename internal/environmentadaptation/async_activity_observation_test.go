package environmentadaptation_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestEnvironmentActivityObservationErrorAndRecovery(t *testing.T) {
	s, remote, store, now := setup(t)
	r := start(t, s)
	observe(t, s)
	observe(t, s) // Submission and the first successful inspection are distinct.
	project := func() api.AsyncActivity {
		t.Helper()
		facts, _, err := store.ReadAsyncActivityFacts(ctx, s.Project, ports.AsyncActivityRead{Source: "environment_detection", Limit: 50})
		if err != nil || len(facts) != 1 {
			t.Fatalf("read facts: %v %+v", err, facts)
		}
		a, err := query.ProjectAsyncActivity(s.Project, "i", facts[0], *now)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	first := project()
	if first.State != "running" || first.Observation != "current" || first.ObservedAt == nil {
		t.Fatalf("initial successful observation: %+v", first)
	}
	*now = now.Add(time.Second)
	remote.inspectFailure = errors.New("temporary inspect outage")
	if err := s.Observe(ctx); err == nil {
		t.Fatal("inspection failure hidden")
	}
	failed := project()
	if failed.State != "running" || failed.Observation != "unavailable" || failed.ObservedAt == nil || !failed.ObservedAt.Equal(*first.ObservedAt) || !strings.Contains(failed.Diagnostic, "temporary inspect outage") {
		t.Fatalf("known failure appeared current or changed work: %+v", failed)
	}
	*now = now.Add(time.Second)
	remote.inspectFailure = nil
	observe(t, s)
	recovered := project()
	if recovered.State != "running" || recovered.Observation != "current" || recovered.ObservedAt == nil || !recovered.ObservedAt.Equal(*now) || strings.Contains(recovered.Diagnostic, "temporary inspect outage") {
		t.Fatalf("recovery retained stale observation error: %+v", recovered)
	}
	// A separate business diagnostic must not be cleared by a healthy read.
	stored := read(t, s, r.ID)
	stored.Diagnostic = "independent business diagnostic"
	stored.Revision++
	if err := store.SaveEnvironment(ctx, stored, stored.Revision-1); err != nil {
		t.Fatal(err)
	}
	observe(t, s)
	if a := project(); a.Diagnostic != "independent business diagnostic" || a.Observation != "current" {
		t.Fatalf("observation cleared domain diagnostic: %+v", a)
	}
}
