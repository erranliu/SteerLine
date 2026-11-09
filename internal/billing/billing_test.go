package billing

import (
	"math"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }
func fixture() (Usage, Policy) {
	at := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	p := Policy{ID: "fixture", Version: "one", CreatedAt: at, Source: "synthetic arithmetic, not vendor prices", Currency: "USD", Basis: Basis, Rules: []Rule{{ID: "r", Provider: "provider", Model: "model", From: at.Add(-time.Hour), Rates: Rates{Input: "2", Read: "0.2", Write: "3", Output: "4"}}}}
	u := Usage{RunID: "run", Revision: 7, Scope: "main_session", Status: "available", Finalized: true, CoverageComplete: true, PriceAt: at, Records: []Record{{ID: "response", StepID: "implement.execute", Model: "model", Tokens: Tokens{InputUncached: ptr(int64(1000000)), CacheRead: ptr(int64(100000)), CacheWrite: ptr(int64(0)), Output: ptr(int64(500000)), Reasoning: ptr(int64(200000))}}}}
	return u, p
}
func TestCalculateCoverageAndExactAmounts(t *testing.T) {
	for _, tc := range []struct {
		name, status, amount string
		change               func(*Usage, *Policy)
	}{
		{"full", "estimated", "4.020000000000000000", func(*Usage, *Policy) {}},
		{"active", "partial", "4.020000000000000000", func(u *Usage, _ *Policy) { u.Finalized = false }},
		{"unknown output", "partial", "2.020000000000000000", func(u *Usage, _ *Policy) { u.Records[0].Tokens.Output = nil }},
		{"missing model", "unavailable", "", func(u *Usage, _ *Policy) { u.Records[0].Model = "" }},
		{"missing stage", "unavailable", "", func(u *Usage, _ *Policy) { u.Records[0].StepID = "" }},
		{"missing price time", "unavailable", "", func(u *Usage, _ *Policy) { u.PriceAt = time.Time{} }},
		{"missing price", "unavailable", "", func(_ *Usage, p *Policy) { p.Rules = nil }},
		{"zero rates", "estimated", "0.000000000000000000", func(_ *Usage, p *Policy) { p.Rules[0].Rates = Rates{"0", "0", "0", "0"} }},
		{"tiny precise", "estimated", "0.000000000000000001", func(u *Usage, p *Policy) {
			u.Records[0].Tokens = Tokens{InputUncached: ptr(int64(1)), CacheRead: ptr(int64(0)), CacheWrite: ptr(int64(0)), Output: ptr(int64(0))}
			p.Rules[0].Rates.Input = "0.000000000001"
		}},
		{"large exact", "estimated", "18446744073709.551614000000000000", func(u *Usage, p *Policy) {
			u.Records[0].Tokens = Tokens{InputUncached: ptr(int64(math.MaxInt64)), CacheRead: ptr(int64(0)), CacheWrite: ptr(int64(0)), Output: ptr(int64(0))}
			p.Rules[0].Rates.Input = "2"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, p := fixture()
			tc.change(&u, &p)
			e, err := Calculate(u, p)
			if err != nil || e.Status != tc.status {
				t.Fatalf("%+v %v", e, err)
			}
			if tc.amount == "" {
				if e.Covered != nil {
					t.Fatal(e)
				}
			} else if e.Covered == nil || *e.Covered != tc.amount {
				t.Fatalf("got %v want %s", e.Covered, tc.amount)
			}
			if e.FullyPriced+e.PartlyPriced+e.Unpriced != e.RecordCount {
				t.Fatal("partition")
			}
			if (e.Total != nil) != (tc.status == "estimated") {
				t.Fatal("total certainty")
			}
		})
	}
}
func TestCalculateCandidatePricePeriodAndAmbiguity(t *testing.T) {
	u, p := fixture()
	boundary := u.PriceAt
	p.Rules[0].Until = &boundary
	next := p.Rules[0]
	next.ID = "next"
	next.From = boundary
	next.Until = nil
	next.Rates.Input = "5"
	p.Rules = append(p.Rules, next)
	e, err := Calculate(u, p)
	if err != nil || e.Covered == nil || *e.Covered != "7.020000000000000000" {
		t.Fatal(e, err)
	}
	u.PriceAt = boundary.Add(-time.Nanosecond)
	e, err = Calculate(u, p)
	if err != nil || e.Total == nil || *e.Total != "4.020000000000000000" {
		t.Fatal(e, err)
	}
	other := p.Rules[0]
	other.ID, other.Provider = "other", "other-provider"
	p.Rules = append(p.Rules, other)
	e, err = Calculate(u, p)
	if err != nil || e.Status != "unavailable" || len(e.Issues) != 1 || e.Issues[0] != "PRICE_AMBIGUOUS" {
		t.Fatal(e, err)
	}
}

func TestCalculateWorkflowModelsAndUnassignedUsage(t *testing.T) {
	u, p := fixture()
	for _, step := range []struct{ id, model, effort string }{
		{"audit.code.execute", "reviewer", "high"},
		{"publish.execute", "model", "medium"},
		{"publish.execute", "model", "medium"},
		{"initialize", "", ""},
	} {
		r := u.Records[0]
		r.ID, r.StepID, r.Model, r.ReasoningEffort = step.id+string(rune('0'+len(u.Records))), step.id, step.model, step.effort
		u.Records = append(u.Records, r)
	}
	price := p.Rules[0]
	price.ID, price.Model, price.Rates.Input = "review", "reviewer", "5"
	p.Rules = append(p.Rules, price)
	e, err := Calculate(u, p)
	// Writer 4.02 + reviewer 7.02 + publisher 2*4.02. Initialization is unassigned.
	if err != nil || e.Status != "partial" || e.Total != nil || e.Covered == nil || *e.Covered != "19.080000000000000000" || e.Unpriced != 1 || e.FullyPriced != 4 || len(e.Matches) != 3 {
		t.Fatal(e, err)
	}
	if e.Matches[0].StepID != "implement.execute" || e.Matches[1].Model != "reviewer" || e.Matches[1].ReasoningEffort != "high" || e.Matches[2].StepID != "publish.execute" || e.Matches[2].Records != 2 || e.Matches[2].Amount != "8.040000000000000000" || e.Issues[0] != "CONFIGURED_MODEL_UNASSIGNED" {
		t.Fatal("workflow grouping lost", e)
	}
}
func TestRejectAmbiguousOrInvalidPolicyAndFacts(t *testing.T) {
	_, p := fixture()
	p.Rules = append(p.Rules, p.Rules[0])
	if p.Validate() == nil {
		t.Fatal("duplicate")
	}
	p.Rules[1].ID = "other"
	if p.Validate() == nil {
		t.Fatal("overlap")
	}
	for _, rate := range []string{"-1", "NaN", "1e2", "0.0000000000001", ""} {
		_, p = fixture()
		p.Rules[0].Rates.Input = rate
		if p.Validate() == nil {
			t.Fatal(rate)
		}
	}
	u, p := fixture()
	u.Records = append(u.Records, u.Records[0])
	if _, err := Calculate(u, p); err == nil {
		t.Fatal("duplicate usage")
	}
	u, p = fixture()
	u.Records[0].Tokens.Output = ptr(int64(-1))
	if _, err := Calculate(u, p); err == nil {
		t.Fatal("negative")
	}
}
