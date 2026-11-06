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
	u := Usage{RunID: "run", Revision: 7, Scope: "main_session", Status: "available", Finalized: true, CoverageComplete: true, Records: []Record{{ID: "response", Provider: ptr("provider"), Model: ptr("model"), MetadataBasis: "observed", TimeBasis: "request", CallTime: &at, Tokens: Tokens{InputUncached: ptr(int64(1000000)), CacheRead: ptr(int64(100000)), CacheWrite: ptr(int64(0)), Output: ptr(int64(500000)), Reasoning: ptr(int64(200000))}}}}
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
		{"missing model", "unavailable", "", func(u *Usage, _ *Policy) { u.Records[0].Model = nil }},
		{"conflict", "unavailable", "", func(u *Usage, _ *Policy) { u.Records[0].MetadataConflict = true }},
		{"completion", "unavailable", "", func(u *Usage, _ *Policy) { u.Records[0].TimeBasis = "completion" }},
		{"provider isolation", "unavailable", "", func(u *Usage, _ *Policy) { u.Records[0].Provider = ptr("other") }},
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
func TestCalculatePeriodBoundaryAndIntervals(t *testing.T) {
	u, p := fixture()
	boundary := *u.Records[0].CallTime
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
	u.Records[0].Granularity = "interval"
	u.Records[0].IntervalStart = ptr(boundary.Add(-time.Minute))
	u.Records[0].IntervalEnd = ptr(boundary.Add(time.Minute))
	e, err = Calculate(u, p)
	if err != nil || e.Status != "unavailable" {
		t.Fatal(e, err)
	}
	u.Records[0].IntervalEnd = &boundary
	e, err = Calculate(u, p)
	if err != nil || e.Covered == nil || *e.Covered != "4.020000000000000000" {
		t.Fatal(e, err)
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
