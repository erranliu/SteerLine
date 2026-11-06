// Package billing converts immutable usage facts into a manual flat-rate estimate.
// It has no filesystem, transport or execution dependencies.
package billing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"strings"
	"time"
)

const Basis = "manual_flat_token_estimate"
const CalculationVersion = "flat-token/v1"
const MaxRecords = 50000

type Rates struct {
	Input  string `json:"input_uncached_per_million"`
	Read   string `json:"cache_read_per_million"`
	Write  string `json:"cache_write_per_million"`
	Output string `json:"output_per_million"`
}
type Rule struct {
	ID       string     `json:"id"`
	Provider string     `json:"provider"`
	Model    string     `json:"model"`
	From     time.Time  `json:"effective_from"`
	Until    *time.Time `json:"effective_until"`
	Rates    Rates      `json:"rates"`
}
type Policy struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source"`
	SourceURL string    `json:"source_url,omitempty"`
	Currency  string    `json:"currency"`
	Basis     string    `json:"basis"`
	Rules     []Rule    `json:"rules"`
}

var decimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})(\.[0-9]{1,12})?$`)
var currency = regexp.MustCompile(`^[A-Z]{3}$`)

func (p Policy) Validate() error {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Version) == "" || p.CreatedAt.IsZero() || strings.TrimSpace(p.Source) == "" || !currency.MatchString(p.Currency) || p.Basis != Basis || len(p.Rules) > 1000 {
		return errors.New("invalid pricing policy identity, currency, basis or rule limit")
	}
	ids := map[string]bool{}
	for i, r := range p.Rules {
		if r.ID == "" || ids[r.ID] || strings.TrimSpace(r.Provider) == "" || strings.TrimSpace(r.Model) == "" || strings.ContainsAny(r.Provider+r.Model, "*?") || r.From.IsZero() || r.Until != nil && !r.Until.After(r.From) {
			return errors.New("invalid or duplicate price rule")
		}
		ids[r.ID] = true
		for _, v := range []string{r.Rates.Input, r.Rates.Read, r.Rates.Write, r.Rates.Output} {
			if !decimal.MatchString(v) {
				return errors.New("rates require nonnegative decimal strings with at most 12 fractional digits")
			}
		}
		for _, b := range p.Rules[:i] {
			if r.Provider == b.Provider && r.Model == b.Model && (r.Until == nil || b.From.Before(*r.Until)) && (b.Until == nil || r.From.Before(*b.Until)) {
				return errors.New("overlapping price periods")
			}
		}
	}
	return nil
}
func (p Policy) Digest() string {
	raw, _ := json.Marshal(p)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

type Tokens struct {
	InputTotal    *int64 `json:"input_total"`
	InputUncached *int64 `json:"input_uncached"`
	CacheRead     *int64 `json:"cache_read"`
	CacheWrite    *int64 `json:"cache_write"`
	Output        *int64 `json:"output"`
	Reasoning     *int64 `json:"reasoning"`
	Total         *int64 `json:"total"`
}
type Record struct {
	ID                                    string
	Provider, Model                       *string
	MetadataBasis, TimeBasis, Granularity string
	CallTime, IntervalStart, IntervalEnd  *time.Time
	Tokens                                Tokens
	MetadataConflict                      bool
}
type Usage struct {
	RunID, Scope, Status string
	Revision             uint64
	Finalized            bool
	CoverageComplete     bool
	ZeroKnown            bool
	Records              []Record
}
type Match struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	RuleID   string `json:"rule_id"`
	Records  int64  `json:"records"`
	Amount   string `json:"amount"`
}
type Estimate struct {
	Status             string   `json:"status"`
	RunID              string   `json:"run_id"`
	Scope              string   `json:"scope"`
	UsageRevision      uint64   `json:"usage_revision"`
	PolicyID           string   `json:"pricing_policy"`
	PolicyVersion      string   `json:"pricing_policy_version"`
	PolicyDigest       string   `json:"pricing_policy_digest"`
	PolicySource       string   `json:"pricing_source"`
	Generation         uint64   `json:"settings_generation"`
	Currency           string   `json:"currency"`
	CalculationVersion string   `json:"calculation_version"`
	Basis              string   `json:"basis"`
	Covered            *string  `json:"covered_estimated_amount"`
	Total              *string  `json:"total_estimated_amount"`
	RecordCount        int64    `json:"record_count"`
	FullyPriced        int64    `json:"fully_priced_records"`
	PartlyPriced       int64    `json:"partially_priced_records"`
	Unpriced           int64    `json:"unpriced_records"`
	Matches            []Match  `json:"matches"`
	Assumptions        []string `json:"assumptions"`
	Issues             []string `json:"issues"`
}

func Calculate(u Usage, p Policy) (Estimate, error) {
	e := Estimate{Status: "unavailable", RunID: u.RunID, Scope: u.Scope, UsageRevision: u.Revision, PolicyID: p.ID, PolicyVersion: p.Version, PolicyDigest: p.Digest(), PolicySource: p.Source, Currency: p.Currency, CalculationVersion: CalculationVersion, Basis: Basis, RecordCount: int64(len(u.Records)), Matches: []Match{}, Issues: []string{}, Assumptions: []string{"按配置固定费率估算，非实际账单；未建模上下文阶梯、服务档位、缓存多档、折扣和附加费。"}}
	if err := p.Validate(); err != nil {
		return e, err
	}
	if u.RunID == "" || u.Revision == 0 || u.Scope != "main_session" || len(u.Records) > MaxRecords {
		return e, errors.New("invalid usage identity or record budget")
	}
	sum := new(big.Rat)
	seen := map[string]bool{}
	reasons := map[string]bool{}
	amounts := map[string]*big.Rat{}
	counts := map[string]int64{}
	addIssue := func(s string) {
		if !reasons[s] {
			e.Issues = append(e.Issues, s)
			reasons[s] = true
		}
	}
	for _, r := range u.Records {
		if r.ID == "" || seen[r.ID] {
			return e, errors.New("duplicate or empty usage record")
		}
		seen[r.ID] = true
		for _, n := range []*int64{r.Tokens.InputTotal, r.Tokens.InputUncached, r.Tokens.CacheRead, r.Tokens.CacheWrite, r.Tokens.Output, r.Tokens.Reasoning, r.Tokens.Total} {
			if n != nil && *n < 0 {
				return e, errors.New("negative usage counter")
			}
		}
		if r.Tokens.Reasoning != nil && r.Tokens.Output != nil && *r.Tokens.Reasoning > *r.Tokens.Output {
			return e, errors.New("reasoning exceeds output")
		}
		fail := ""
		var matched *Rule
		if r.MetadataConflict {
			fail = "PRICING_METADATA_CONFLICT"
		} else if r.Provider == nil || r.Model == nil || r.MetadataBasis != "observed" {
			fail = "MODEL_UNKNOWN"
		} else if r.Granularity == "interval" && (r.IntervalStart == nil || r.IntervalEnd == nil || !r.IntervalEnd.After(*r.IntervalStart)) || r.Granularity != "interval" && (r.CallTime == nil || r.TimeBasis != "request") {
			fail = "PRICE_PERIOD_UNKNOWN"
		} else {
			for i := range p.Rules {
				rule := &p.Rules[i]
				if rule.Provider != *r.Provider || rule.Model != *r.Model {
					continue
				}
				at := r.CallTime
				if r.Granularity == "interval" {
					at = r.IntervalStart
				}
				if at.Before(rule.From) || rule.Until != nil && !at.Before(*rule.Until) {
					continue
				}
				if r.Granularity == "interval" && rule.Until != nil && r.IntervalEnd.After(*rule.Until) {
					continue
				}
				matched = rule
				break
			}
			if matched == nil {
				fail = "PRICE_NOT_FOUND"
			}
		}
		if fail != "" {
			e.Unpriced++
			addIssue(fail)
			continue
		}
		amount := new(big.Rat)
		known := 0
		ns := []*int64{r.Tokens.InputUncached, r.Tokens.CacheRead, r.Tokens.CacheWrite, r.Tokens.Output}
		rates := []string{matched.Rates.Input, matched.Rates.Read, matched.Rates.Write, matched.Rates.Output}
		for i, n := range ns {
			if n == nil {
				continue
			}
			rate, _ := new(big.Rat).SetString(rates[i])
			part := new(big.Rat).Mul(rate, new(big.Rat).SetInt64(*n))
			amount.Add(amount, part)
			known++
		}
		amount.Quo(amount, big.NewRat(1000000, 1))
		if known == 0 {
			e.Unpriced++
			addIssue("USAGE_PARTIAL")
			continue
		}
		if known == 4 {
			e.FullyPriced++
		} else {
			e.PartlyPriced++
			addIssue("USAGE_PARTIAL")
		}
		sum.Add(sum, amount)
		if amounts[matched.ID] == nil {
			amounts[matched.ID] = new(big.Rat)
		}
		amounts[matched.ID].Add(amounts[matched.ID], amount)
		counts[matched.ID]++
	}
	complete := u.Status == "available" && u.Finalized && u.CoverageComplete && (len(u.Records) > 0 || u.ZeroKnown)
	if complete && e.FullyPriced == e.RecordCount {
		e.Status = "estimated"
	} else if e.FullyPriced+e.PartlyPriced > 0 {
		e.Status = "partial"
	}
	if !complete {
		addIssue("USAGE_NOT_FINALIZED")
	}
	if e.Status != "unavailable" {
		v := sum.FloatString(18)
		e.Covered = &v
		if e.Status == "estimated" {
			e.Total = &v
		}
	}
	for _, r := range p.Rules {
		if a := amounts[r.ID]; a != nil {
			e.Matches = append(e.Matches, Match{RuleID: r.ID, Provider: r.Provider, Model: r.Model, Records: counts[r.ID], Amount: a.FloatString(18)})
		}
	}
	return e, nil
}
