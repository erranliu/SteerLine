package testenv

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

// AssertClosedDeliveryQuiescent observes normal polling. It must not query the
// closed task's PR or submit another input.
func (p *Project) AssertClosedDeliveryQuiescent(done Snapshot) {
	s := p.Scenario
	s.t.Helper()
	// Delivery commits the stop obligation before its asynchronous CLI receipt.
	// Establish that receipt before asserting a fixed final request count.
	p.WaitStoppedCandidates()
	before, err := controlled.ProviderQueryCount(p.Provider)
	s.Must(err)
	if before == 0 {
		s.t.Fatal("provider query evidence did not observe the original delivery")
	}
	const phase = "closed-poll"
	until := time.Now().Add(2 * time.Second) // fixture PR/task polls every 100ms
	s.Wait(phase, func() (bool, error) {
		count, err := controlled.ProviderQueryCount(p.Provider)
		if err != nil {
			return false, err
		}
		if count != before {
			s.t.Fatalf("closed task queried provider: before=%d after=%d", before, count)
		}
		return !time.Now().Before(until), nil
	})
	snap, err := p.Snapshot()
	s.Must(err)
	if len(snap.Domain.Deliveries) != 1 || len(p.CallerRequests()) != 2 {
		s.t.Fatal("closed polling created another delivery or input")
	}
	p.Record(phase, snap)
	after, err := controlled.ProviderQueryCount(p.Provider)
	s.Must(err)
	s.Must(controlled.Write(filepath.Join(s.Output, fmt.Sprintf("%s-provider-queries.json", phase)), map[string]any{"before": before, "after": after, "additional_queries": after - before, "requests": len(p.CallerRequests()), "deliveries": len(snap.Domain.Deliveries)}))
	if after != before {
		s.t.Fatal("closed task queried provider during final snapshot")
	}
}
