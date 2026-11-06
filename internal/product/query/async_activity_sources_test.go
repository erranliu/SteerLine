package query

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/storage/ports"
)

type activityReadProbe struct {
	mu        sync.Mutex
	calls     []ports.AsyncActivityRead
	deadlines []time.Time
	slow      string
}

func (p *activityReadProbe) ReadAsyncActivityFacts(ctx context.Context, _ string, in ports.AsyncActivityRead) ([]ports.AsyncActivityFact, bool, error) {
	p.mu.Lock()
	p.calls = append(p.calls, in)
	deadline, _ := ctx.Deadline()
	p.deadlines = append(p.deadlines, deadline)
	p.mu.Unlock()
	if in.Source == p.slow {
		<-ctx.Done()
		return nil, false, ctx.Err()
	}
	return []ports.AsyncActivityFact{{Source: "baseline_check", ID: "request", Payload: json.RawMessage(`{"command_id":"request","status":"pending","payload":{"instance_id":"i"}}`), Related: json.RawMessage(`{"current":1}`)}}, false, nil
}

func TestAsyncActivitySourceOperationsShareOneBudget(t *testing.T) {
	p := &activityReadProbe{}
	_, err := ReadAsyncActivities(context.Background(), p, "p", "i", AsyncActivityOptions{Sources: []string{"baseline_check"}, RequestIDs: []string{"request"}})
	if err != nil || len(p.calls) != 3 {
		t.Fatalf("expected summary/list/links: %+v %v", p.calls, err)
	}
	for _, deadline := range p.deadlines {
		if deadline.IsZero() || !deadline.Equal(p.deadlines[0]) {
			t.Fatalf("source budget renewed between operations: %v", p.deadlines)
		}
	}
}

func TestAsyncActivityParentCancellationDoesNotStartReads(t *testing.T) {
	p := &activityReadProbe{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	page, err := ReadAsyncActivities(ctx, p, "p", "i", AsyncActivityOptions{})
	if err != nil || len(p.calls) != 0 || page.CountsKnown || len(page.Items) != 0 {
		t.Fatalf("cancelled observation still read sources: %+v %v calls=%v", page, err, p.calls)
	}
	for _, source := range page.Sources {
		if source.Complete || source.Diagnostic == "" {
			t.Fatalf("cancellation hidden as empty success: %+v", source)
		}
	}
}

func TestAsyncActivitySourceBudgetsAreIndependent(t *testing.T) {
	p := &activityReadProbe{slow: "environment_detection"}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	started := time.Now()
	page, err := ReadAsyncActivities(ctx, p, "p", "i", AsyncActivityOptions{Sources: []string{"environment_detection", "baseline_check"}})
	if err != nil || len(page.Items) != 1 || page.Items[0].Source != "baseline_check" || page.CountsKnown || page.Sources[0].Diagnostic == "" || !page.Sources[1].Complete || ctx.Err() != nil {
		t.Fatalf("one source consumed another source's budget: %+v %v", page, err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("source did not honor its bounded budget")
	}
}

func TestAsyncActivityLinksOnlyNeverReadsSummaryOrList(t *testing.T) {
	p := &activityReadProbe{}
	links, err := ReadAsyncActivityLinks(context.Background(), p, "p", "i", AsyncActivityOptions{PublicProjectID: "public", RequestIDs: []string{"request"}, Sources: []string{"baseline_check"}})
	if err != nil || links.ProjectID != "public" || links.Links["request"].ProjectID != "public" || len(p.calls) != 1 {
		t.Fatalf("unexpected links read %+v %v calls=%+v", links, err, p.calls)
	}
	for _, call := range p.calls {
		if call.Filter != "" || len(call.RequestIDs) != 1 {
			t.Fatalf("association lookup computed a list/summary: %+v", call)
		}
	}
	raw, _ := json.Marshal(links)
	for _, field := range []string{`"counts"`, `"items"`, `"has_more"`} {
		if strings.Contains(string(raw), field) {
			t.Fatalf("links response claims list semantics: %s", raw)
		}
	}
}

func TestAsyncActivityLinksSourceTimeoutIsPartial(t *testing.T) {
	p := &activityReadProbe{slow: "environment_detection"}
	links, err := ReadAsyncActivityLinks(context.Background(), p, "p", "i", AsyncActivityOptions{RequestIDs: []string{"request"}, Sources: []string{"environment_detection", "baseline_check"}})
	if err != nil || len(links.Links) != 1 || links.Sources[0].Complete || links.Sources[0].Diagnostic == "" || !links.Sources[1].Complete {
		t.Fatalf("links failure lost healthy sources: %+v %v", links, err)
	}
}
