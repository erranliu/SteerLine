package testmanagement

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	m "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

var ErrInvalidRecord = m.ErrInvalidRecord
var ErrRecordConflict = m.ErrRecordConflict

var ErrInvalidRecordFilter = errors.New("invalid test record filter")

var recordID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var summarySHA = regexp.MustCompile(`^[0-9a-f]{64}$`)

type RecordStore interface {
	SaveTestRecord(context.Context, m.TestRecord) (m.TestRecord, error)
	TestRecord(context.Context, string, string) (m.TestRecord, error)
	TestRecordHistory(context.Context, string, string, int64) ([]m.TestRecord, int64, error)
	TestRecordDefinitions(context.Context, string) ([]Definition, error)
	PendingTestEvidence(context.Context, string, string, int) ([]m.Evidence, error)
	MarkTestEvidenceProjected(context.Context, string, string, int) error
}

type RecordService struct {
	Project         string
	Store           RecordStore
	projectionAfter string
}

// ParseReport extends the existing execution report, without borrowing
// the candidate's clean/current-code admission rules for historical records.
func ParseReport(raw []byte) (Report, error) {
	if len(raw) > m.MaxReportBytes {
		return Report{}, errors.New("test report exceeds 128 KiB")
	}
	r, err := parseReport(raw)
	if err != nil {
		return r, err
	}
	if !recordID.MatchString(r.ID) || len(r.Selection) != 1 || len(r.Results) != 1 || len(r.Definitions) != 1 || r.Definition.ID != r.Selection[0] || r.Definitions[r.Definition.ID] != Digest(r.Definition) {
		return r, errors.New("record requires one frozen test definition and a 32-hex report ID")
	}
	body, _ := json.Marshal(Catalogue{Tests: []Definition{r.Definition}})
	if _, err = ParseCatalogue(body); err != nil {
		return r, err
	}
	if r.StartedAt.IsZero() || r.FinishedAt.Before(r.StartedAt) || r.Command == "" || r.Repository == "" {
		return r, errors.New("record requires execution time, command and repository")
	}
	if r.RerunOf != "" && (!recordID.MatchString(r.RerunOf) || r.RerunOf == r.ID) {
		return r, errors.New("invalid rerun_of")
	}
	for _, sha := range []string{r.BeforeCommit, r.AfterCommit} {
		if sha != "" && !regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`).MatchString(sha) {
			return r, errors.New("invalid source commit")
		}
	}
	if r.Environment != r.Definition.Environment {
		return r, errors.New("actual environment differs from frozen definition")
	}
	if r.Evidence.Path != "summary.json" || r.Results[0].Log != "summary.json" || r.Evidence.Bytes < 0 || r.Evidence.SHA256 != "" && !summarySHA.MatchString(r.Evidence.SHA256) {
		return r, errors.New("record evidence must declare summary.json and its content digest")
	}
	if r.Scope != nil {
		seen := map[string]bool{}
		failed, incomplete := false, false
		for _, v := range r.Scope.Items {
			if strings.TrimSpace(v.ID) == "" || seen[v.ID] || v.Status != "passed" && v.Status != "failed" && v.Status != "incomplete" || v.Status != "passed" && strings.TrimSpace(v.Message) == "" {
				return r, errors.New("invalid scope result")
			}
			seen[v.ID] = true
			failed = failed || v.Status == "failed"
			incomplete = incomplete || v.Status == "incomplete"
		}
		if len(seen) == 0 || failed && r.Results[0].Status != "failed" || !failed && incomplete && r.Results[0].Status != "incomplete" || !failed && !incomplete && r.Results[0].Status != "passed" {
			return r, errors.New("scope and overall result differ")
		}
	}
	return r, nil
}

func prepareRecord(project string, in m.RegisterRecord, source m.RecordSource) (m.TestRecord, error) {
	r, err := ParseReport(in.Report)
	if err != nil {
		return m.TestRecord{}, fmt.Errorf("%w: %v", ErrInvalidRecord, err)
	}
	v := m.TestRecord{Project: project, Report: r, Source: source, ObservedAt: time.Now().UTC(), EvidenceStatus: "unavailable", Diagnostic: in.Diagnostic}
	if source.Kind == "" {
		v.Source.Kind = "script"
	}
	if len(in.Summary) > m.MaxSummaryBytes {
		return v, fmt.Errorf("%w: summary exceeds 512 KiB", ErrInvalidRecord)
	}
	if len(in.Summary) > 0 {
		if len(in.Summary) != r.Evidence.Bytes || ContentDigest(in.Summary) != r.Evidence.SHA256 {
			return v, fmt.Errorf("%w: summary length/digest mismatch", ErrInvalidRecord)
		}
		v.Summary = in.Summary
		v.EvidenceStatus = "available"
		v.Diagnostic = ""
	} else if v.Diagnostic == "" {
		v.Diagnostic = "必要 summary 尚不可用或未上传"
	}
	return v, nil
}
func (s *RecordService) Register(ctx context.Context, in m.RegisterRecord, source m.RecordSource) (m.TestRecord, error) {
	v, err := prepareRecord(s.Project, in, source)
	if err != nil {
		return v, err
	}
	return s.Store.SaveTestRecord(ctx, v)
}

type RegistrationReceipt struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	Seq            int64  `json:"seq"`
	EvidenceStatus string `json:"evidence_status"`
	Diagnostic     string `json:"diagnostic,omitempty"`
}

func ReceiptOf(v m.TestRecord) RegistrationReceipt {
	return RegistrationReceipt{ID: v.Report.ID, Status: "applied", Seq: v.Seq, EvidenceStatus: v.EvidenceStatus, Diagnostic: v.Diagnostic}
}

// ReconcileRegistration is a read of one authoritative record. It validates the
// complete original input, including supplied evidence, without running a test
// or constructing the UI's history/detail projection.
func (s *RecordService) ReconcileRegistration(ctx context.Context, in m.RegisterRecord) (RegistrationReceipt, error) {
	requested, err := prepareRecord(s.Project, in, m.RecordSource{})
	if err != nil {
		return RegistrationReceipt{}, err
	}
	stored, err := s.Store.TestRecord(ctx, s.Project, requested.Report.ID)
	if err != nil {
		return RegistrationReceipt{}, err
	}
	if Digest(stored.Report) != Digest(requested.Report) {
		return RegistrationReceipt{}, ErrRecordConflict
	}
	out := ReceiptOf(stored)
	if len(in.Summary) > 0 && stored.EvidenceStatus != "available" {
		out.Status = "outcome_unknown"
	}
	return out, nil
}

type RecordView struct {
	Seq               int64          `json:"seq"`
	ID                string         `json:"id"`
	TestID            string         `json:"test_id"`
	Name              string         `json:"name"`
	Level             string         `json:"level"`
	Environment       string         `json:"environment"`
	Status            string         `json:"status"`
	Message           string         `json:"message"`
	Retest            string         `json:"retest"`
	RetestID          string         `json:"retest_id,omitempty"`
	RetestSourceFixed *bool          `json:"retest_source_fixed,omitempty"`
	StartedAt         time.Time      `json:"started_at"`
	FinishedAt        time.Time      `json:"finished_at"`
	Branch            string         `json:"branch"`
	Commit            string         `json:"commit"`
	SourceFixed       bool           `json:"source_fixed"`
	Partial           bool           `json:"partial"`
	RerunOf           string         `json:"rerun_of,omitempty"`
	EvidenceStatus    string         `json:"evidence_status"`
	Diagnostic        string         `json:"diagnostic,omitempty"`
	Source            m.RecordSource `json:"source"`
}
type RecordSummary struct {
	Total      int         `json:"total"`
	Pending    int         `json:"pending"`
	Latest     *RecordView `json:"latest,omitempty"`
	LastPassed *RecordView `json:"last_passed,omitempty"`
}
type RecordPage struct {
	Records    []RecordView  `json:"records"`
	Summary    RecordSummary `json:"summary"`
	Cursor     string        `json:"cursor"`
	Upper      int64         `json:"upper"`
	ObservedAt time.Time     `json:"observed_at"`
}
type RecordDetail struct {
	RecordView
	Report BrowserReport `json:"report"`
	Chain  []RecordView  `json:"chain"`
}

// The producer sequence can exceed JavaScript's safe integer range.
// Query projections preserve it as text; the immutable report stays numeric.
type BrowserReport struct {
	Report
	Sequence string `json:"sequence"`
}

func sourceFixed(r Report) bool {
	return r.BeforeCommit != "" && r.BeforeCommit == r.AfterCommit && r.BeforeClean && r.AfterClean
}
func recordView(r m.TestRecord) RecordView {
	return RecordView{Seq: r.Seq, ID: r.Report.ID, TestID: r.Report.Definition.ID, Name: r.Report.Definition.Name, Level: r.Report.Definition.Level, Environment: r.Report.Environment, Status: r.Report.Results[0].Status, Message: r.Report.Results[0].Message, StartedAt: r.Report.StartedAt, FinishedAt: r.Report.FinishedAt, Branch: r.Report.Branch, Commit: r.Report.AfterCommit, SourceFixed: sourceFixed(r.Report), Partial: r.Report.Scope != nil && !r.Report.Scope.Complete, RerunOf: r.Report.RerunOf, EvidenceStatus: r.EvidenceStatus, Diagnostic: r.Diagnostic, Source: r.Source}
}
func scopeFailures(r Report) []string {
	if r.Scope == nil {
		return []string{"$whole"}
	}
	var ids []string
	for _, v := range r.Scope.Items {
		if v.Status == "failed" {
			ids = append(ids, v.ID)
		}
	}
	return ids
}
func retestAgainst(original, next m.TestRecord) string {
	if original.Report.Environment != next.Report.Environment {
		return "environment_changed"
	}
	if original.Report.Definitions[original.Report.Definition.ID] != next.Report.Definitions[next.Report.Definition.ID] {
		return "definition_changed"
	}

	if next.Report.Results[0].Status != "passed" || next.EvidenceStatus != "available" {
		return "pending"
	}
	if original.Report.Scope == nil {
		if next.Report.Scope != nil && !next.Report.Scope.Complete {
			return "partial"
		}
		return "passed"
	}
	if next.Report.Scope == nil {
		return "passed"
	} // Complete registered entry, not a guessed sub-selection.
	needed := scopeFailures(original.Report)
	if len(needed) == 0 {
		return "pending"
	}
	covered := 0
	for _, id := range needed {
		for _, v := range next.Report.Scope.Items {
			if v.ID == id && v.Status == "passed" {
				covered++
				break
			}
		}
	}
	if covered == len(needed) {
		return "passed"
	}
	if covered > 0 {
		return "partial"
	}
	return "pending"
}

// Relations determine execution order, not clocks or delivery order. Every
// decision compares one report; partial results are never stitched together.
func projectRecordViews(rows []m.TestRecord) []RecordView {
	byID := map[string]m.TestRecord{}
	children := map[string][]string{}
	for _, r := range rows {
		byID[r.Report.ID] = r
		if r.Report.RerunOf != "" {
			children[r.Report.RerunOf] = append(children[r.Report.RerunOf], r.Report.ID)
		}
	}
	views := make([]RecordView, 0, len(rows))
	for _, r := range rows {
		v := recordView(r)
		v.Retest = "independent"
		if v.Status == "failed" {
			v.Retest = "pending"
			id := v.ID
			for len(children[id]) == 1 {
				id = children[id][0]
			}
			if len(children[id]) > 1 {
				v.Retest = "multiple"
			} else if id != v.ID {
				next := byID[id]
				v.Retest = retestAgainst(r, next)
				v.RetestID = id
				fixed := sourceFixed(next.Report)
				v.RetestSourceFixed = &fixed
			}
		} else if r.Report.RerunOf != "" {
			v.Retest = "pending"
			id := r.Report.RerunOf
			for id != "" {
				parent, ok := byID[id]
				if !ok {
					break
				}
				if parent.Report.Results[0].Status == "failed" {
					v.Retest = retestAgainst(parent, r)
					break
				}
				id = parent.Report.RerunOf
			}
		}
		views = append(views, v)
	}
	return views
}

type recordCursor struct {
	Project string    `json:"p"`
	Filter  string    `json:"f"`
	Upper   int64     `json:"u"`
	Started time.Time `json:"t"`
	ID      string    `json:"i"`
}

func (s *RecordService) List(ctx context.Context, f m.RecordFilter) (RecordPage, error) {
	page := RecordPage{Records: []RecordView{}, ObservedAt: time.Now().UTC()}
	if f.Limit == 0 {
		f.Limit = 30
	}
	if f.Limit < 1 || f.Limit > 100 {
		return page, fmt.Errorf("%w: limit must be 1..100", ErrInvalidRecordFilter)
	}
	if f.Level != "" && f.Level != "L1" && f.Level != "L2" && f.Level != "L3" && f.Level != "L4" {
		return page, fmt.Errorf("%w: invalid test level", ErrInvalidRecordFilter)
	}
	if f.Outcome != "" && f.Outcome != "passed" && f.Outcome != "failed" && f.Outcome != "incomplete" {
		return page, fmt.Errorf("%w: invalid outcome", ErrInvalidRecordFilter)
	}
	allowed := map[string]bool{"": true, "pending": true, "passed": true, "partial": true, "multiple": true, "definition_changed": true, "environment_changed": true, "independent": true}
	if !allowed[f.Retest] {
		return page, fmt.Errorf("%w: invalid retest filter", ErrInvalidRecordFilter)
	}
	var from, to time.Time
	var err error
	if f.From != "" {
		from, err = time.Parse(time.RFC3339, f.From)
		if err != nil {
			return page, fmt.Errorf("%w: %v", ErrInvalidRecordFilter, err)
		}
	}
	if f.To != "" {
		to, err = time.Parse(time.RFC3339, f.To)
		if err != nil {
			return page, fmt.Errorf("%w: %v", ErrInvalidRecordFilter, err)
		}
	}
	if !from.IsZero() && !to.IsZero() && !to.After(from) {
		return page, fmt.Errorf("%w: end must follow start", ErrInvalidRecordFilter)
	}
	key := f
	key.Cursor = ""
	fingerprint := Digest(key)
	var cursor recordCursor
	if f.Cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(f.Cursor)
		if e != nil || len(b) > 1024 || Decode(b, &cursor) != nil || cursor.Project != s.Project || cursor.Filter != fingerprint || cursor.Upper < 1 || !recordID.MatchString(cursor.ID) {
			return page, fmt.Errorf("%w: invalid cursor/project/filter", ErrInvalidRecordFilter)
		}
	}
	rows, upper, err := s.Store.TestRecordHistory(ctx, s.Project, f.TestID, cursor.Upper)
	if err != nil {
		return page, err
	}
	page.Upper = upper
	for _, v := range projectRecordViews(rows) {
		if f.Level != "" && f.Level != v.Level || f.Environment != "" && f.Environment != v.Environment || f.Search != "" && !strings.Contains(strings.ToLower(v.TestID+" "+v.Name), strings.ToLower(f.Search)) || f.Source != "" && !strings.Contains(strings.ToLower(v.Branch+" "+v.Commit), strings.ToLower(f.Source)) || !from.IsZero() && v.StartedAt.Before(from) || !to.IsZero() && !v.StartedAt.Before(to) {
			continue
		}
		page.Summary.Total++
		if page.Summary.Latest == nil {
			x := v
			page.Summary.Latest = &x
		}
		if v.Status == "passed" && page.Summary.LastPassed == nil {
			x := v
			page.Summary.LastPassed = &x
		}
		if v.Status == "failed" && v.Retest != "passed" {
			page.Summary.Pending++
		}
		if f.Outcome != "" && f.Outcome != v.Status || f.Retest != "" && f.Retest != v.Retest {
			continue
		}
		if f.Cursor != "" && (v.StartedAt.After(cursor.Started) || v.StartedAt.Equal(cursor.Started) && v.ID >= cursor.ID) {
			continue
		}
		page.Records = append(page.Records, v)
	}
	if len(page.Records) > f.Limit {
		page.Records = page.Records[:f.Limit]
		last := page.Records[len(page.Records)-1]
		b, _ := json.Marshal(recordCursor{Project: s.Project, Filter: fingerprint, Upper: upper, Started: last.StartedAt, ID: last.ID})
		page.Cursor = base64.RawURLEncoding.EncodeToString(b)
	}
	return page, nil
}
func (s *RecordService) Detail(ctx context.Context, id string) (RecordDetail, error) {
	var d RecordDetail
	if !recordID.MatchString(id) {
		return d, errors.New("invalid record ID")
	}
	r, err := s.Store.TestRecord(ctx, s.Project, id)
	if err != nil {
		return d, err
	}
	rows, _, err := s.Store.TestRecordHistory(ctx, s.Project, r.Report.Definition.ID, 0)
	if err != nil {
		return d, err
	}
	views := projectRecordViews(rows)
	root := id
	byID := map[string]m.TestRecord{}
	for _, row := range rows {
		byID[row.Report.ID] = row
	}
	for byID[root].Report.RerunOf != "" {
		root = byID[root].Report.RerunOf
	}
	for _, v := range views {
		if v.ID == id {
			d.RecordView = v
		}
		current := v.ID
		for current != "" {
			if current == root {
				d.Chain = append(d.Chain, v)
				break
			}
			current = byID[current].Report.RerunOf
		}
	}
	sort.Slice(d.Chain, func(i, j int) bool { return d.Chain[i].Seq < d.Chain[j].Seq })
	d.Report = BrowserReport{Report: r.Report, Sequence: strconv.FormatUint(r.Report.Sequence, 10)}
	return d, nil
}
func (s *RecordService) Summary(ctx context.Context, id string) (map[string]any, error) {
	r, err := s.Store.TestRecord(ctx, s.Project, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "status": r.EvidenceStatus, "diagnostic": r.Diagnostic, "path": r.Report.Evidence.Path, "sha256": r.Report.Evidence.SHA256, "text": string(r.Summary)}, nil
}

// Project only already committed public artifacts. No Runner operation or
// candidate participation is needed to recover an interrupted projection.
func (s *RecordService) ProjectEvidence(ctx context.Context) error {
	rows, err := s.Store.PendingTestEvidence(ctx, s.Project, s.projectionAfter, 20)
	if err != nil {
		return err
	}
	var failures []error
	for _, e := range rows {
		// Only an in-memory traversal cursor: persistent completion remains the
		// artifact count. A bad candidate must not monopolize the bounded batch.
		s.projectionAfter = e.CandidateID
		if err = s.projectEvidence(ctx, e); err != nil {
			failures = append(failures, fmt.Errorf("candidate %s: %w", e.CandidateID, err))
		}
	}
	return errors.Join(failures...)
}
func (s *RecordService) projectEvidence(ctx context.Context, e m.Evidence) error {
	byPath := map[string]m.EvidenceArtifact{}
	for _, a := range e.Artifacts {
		byPath[a.Metadata.RelativePath] = a
	}
	type item struct {
		report Report
		input  m.RegisterRecord
	}
	var items []item
	byID := map[string]int{}
	var failures []error
	for _, a := range e.Artifacts {
		if !reportPath(a.Metadata.RelativePath) {
			continue
		}
		r, err := ParseReport(a.Content)
		if err != nil {
			failures = append(failures, fmt.Errorf("stored test report: %w", err))
			continue
		}
		if len(a.Content) != a.Metadata.SizeBytes || ContentDigest(a.Content) != a.Metadata.SHA256 {
			failures = append(failures, errors.New("stored report digest differs"))
			continue
		}
		input := m.RegisterRecord{Report: a.Content}
		summary, ok := byPath[path.Join(path.Dir(a.Metadata.RelativePath), r.Evidence.Path)]
		if ok && len(summary.Content) <= m.MaxSummaryBytes {
			if len(summary.Content) != summary.Metadata.SizeBytes || ContentDigest(summary.Content) != summary.Metadata.SHA256 {
				failures = append(failures, errors.New("stored summary digest differs"))
				continue
			}
			input.Summary = summary.Content
		} else {
			input.Diagnostic = "summary 尚未收集或超过页面 512 KiB 预算"
		}
		if _, exists := byID[r.ID]; !exists {
			byID[r.ID] = len(items)
		}
		items = append(items, item{r, input})
	}
	// Artifact discovery order is not execution order. Register an available
	// explicit parent first, without trusting clocks or creating waiting records.
	state := make([]uint8, len(items))
	results := make([]error, len(items))
	var visit func(int) error
	visit = func(i int) error {
		if state[i] == 2 {
			return results[i]
		}
		if state[i] == 1 {
			return errors.New("cyclic test report references")
		}
		state[i] = 1
		if parent, ok := byID[items[i].report.RerunOf]; ok {
			results[i] = visit(parent)
		}
		if results[i] == nil {
			_, results[i] = s.Register(ctx, items[i].input, m.RecordSource{Kind: "runner", TaskID: e.TaskID, CandidateID: e.CandidateID, RunID: e.RunID})
		}
		state[i] = 2
		return results[i]
	}
	for i := range items {
		if err := visit(i); err != nil {
			failures = append(failures, err)
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	return s.Store.MarkTestEvidenceProjected(ctx, s.Project, e.CandidateID, len(e.Artifacts))
}
