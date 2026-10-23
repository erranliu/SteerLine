package testmanagement

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"regexp"
	"sort"
	"strings"
	"time"
)

const RecordsPath = "SteerlineFlow/testing/records"

var sourceCommit = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
var testIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)
var ErrInvalidRecordFilter = errors.New("invalid test record filter")

// RecordFile is the entire on-disk history format. Its filename is the tested SHA.
type RecordFile struct {
	Runs []RecordRun `json:"runs"`
}
type RecordRun struct {
	At      time.Time      `json:"at"`
	Results []RecordResult `json:"results"`
}
type RecordResult struct {
	TestID      string `json:"test_id"`
	Level       string `json:"level"`
	Environment string `json:"environment"`
	Status      string `json:"status"`
	Note        string `json:"note,omitempty"`
}
type RecordView struct {
	RecordResult
	ID     string    `json:"id"`
	Commit string    `json:"commit"`
	Path   string    `json:"path"`
	At     time.Time `json:"at"`
}
type RecordSnapshot struct {
	Commit  string
	Records []RecordView
}
type RecordSummary struct {
	Total      int         `json:"total"`
	Failed     int         `json:"failed"`
	Latest     *RecordView `json:"latest,omitempty"`
	LastPassed *RecordView `json:"last_passed,omitempty"`
}
type RecordPage struct {
	Records    []RecordView  `json:"records"`
	Summary    RecordSummary `json:"summary"`
	Cursor     string        `json:"cursor"`
	Commit     string        `json:"commit"`
	Path       string        `json:"path"`
	ObservedAt time.Time     `json:"observed_at"`
}

func RecordCommit(filename string) (string, error) {
	commit := strings.TrimSuffix(filename, ".json")
	if filename != commit+".json" || !sourceCommit.MatchString(commit) {
		return "", errors.New("record filename must be a full tested commit SHA followed by .json")
	}
	return commit, nil
}
func ParseRecordFile(raw []byte) (RecordFile, error) {
	var file RecordFile
	canonical, err := runnercontract.CanonicalJSON(raw)
	if err != nil {
		return file, err
	}
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return file, err
	}
	if len(file.Runs) == 0 {
		return file, errors.New("record file requires runs")
	}
	times := map[string]bool{}
	for _, run := range file.Runs {
		stamp := run.At.UTC().Format(time.RFC3339Nano)
		if run.At.IsZero() || times[stamp] || len(run.Results) == 0 {
			return file, errors.New("run requires a unique time and nonempty results")
		}
		times[stamp] = true
		seen := map[string]bool{}
		for _, result := range run.Results {
			if !testIDPattern.MatchString(result.TestID) || seen[result.TestID] || (result.Level != "L1" && result.Level != "L2" && result.Level != "L3" && result.Level != "L4") || strings.TrimSpace(result.Environment) == "" || (result.Status != "passed" && result.Status != "failed" && result.Status != "incomplete") {
				return file, errors.New("invalid or duplicate test result")
			}
			seen[result.TestID] = true
		}
	}
	return file, nil
}
func RecordViews(commit string, file RecordFile) []RecordView {
	rows := []RecordView{}
	for _, run := range file.Runs {
		for _, result := range run.Results {
			id := Digest([]string{commit, run.At.UTC().Format(time.RFC3339Nano), result.TestID})[:32]
			rows = append(rows, RecordView{RecordResult: result, ID: id, Commit: commit, Path: RecordsPath + "/" + commit + ".json", At: run.At})
		}
	}
	return rows
}

// ExportRun accepts complete registered reports from one actual execution only.
// Dirty/changed/unknown sources remain local reports and cannot name a Git record.
func ExportRun(reports []Report) (string, RecordRun, error) {
	var run RecordRun
	commit := ""
	var started time.Time
	seen := map[string]bool{}
	for _, r := range reports {
		if !sourceCommit.MatchString(r.BeforeCommit) || r.BeforeCommit != r.AfterCommit || !r.BeforeClean || !r.AfterClean || (r.Scope != nil && !r.Scope.Complete) {
			return "", run, errors.New("history requires complete tests on clean, unchanged, fixed source")
		}
		if commit != "" && (commit != r.BeforeCommit || !started.Equal(r.StartedAt)) {
			return "", run, errors.New("report directory mixes executions or source commits")
		}
		if seen[r.Definition.ID] {
			return "", run, errors.New("duplicate report test ID")
		}
		seen[r.Definition.ID] = true
		commit, started = r.BeforeCommit, r.StartedAt
		if r.FinishedAt.After(run.At) {
			run.At = r.FinishedAt.UTC()
		}
		result := r.Results[0]
		note := ""
		if result.Status != "passed" {
			note = result.Message
		}
		run.Results = append(run.Results, RecordResult{TestID: r.Definition.ID, Level: r.Definition.Level, Environment: r.Environment, Status: result.Status, Note: note})
	}
	if commit == "" {
		return "", run, errors.New("no managed test reports")
	}
	sort.Slice(run.Results, func(i, j int) bool { return run.Results[i].TestID < run.Results[j].TestID })
	return commit, run, nil
}
func (f *RecordFile) Append(run RecordRun) (bool, error) {
	for _, existing := range f.Runs {
		if existing.At.Equal(run.At) {
			if Digest(existing) == Digest(run) {
				return false, nil
			}
			return false, errors.New("run time already exists with different results")
		}
	}
	f.Runs = append(f.Runs, run)
	sort.Slice(f.Runs, func(i, j int) bool { return f.Runs[i].At.Before(f.Runs[j].At) })
	return true, nil
}

type recordCursor struct {
	Project, Filter, Commit, ID string
	At                          time.Time
}

func (s RecordSnapshot) List(project string, f RecordFilter) (RecordPage, error) {
	p := RecordPage{Records: []RecordView{}, Commit: s.Commit, Path: RecordsPath, ObservedAt: time.Now().UTC()}
	bad := func(reason string) (RecordPage, error) {
		return p, fmt.Errorf("%w: %s", ErrInvalidRecordFilter, reason)
	}
	if f.Limit == 0 {
		f.Limit = 30
	}
	if f.Limit < 1 || f.Limit > 100 {
		return bad("limit must be 1..100")
	}
	if f.Level != "" && f.Level != "L1" && f.Level != "L2" && f.Level != "L3" && f.Level != "L4" {
		return bad("invalid level")
	}
	if f.Outcome != "" && f.Outcome != "passed" && f.Outcome != "failed" && f.Outcome != "incomplete" {
		return bad("invalid outcome")
	}
	var from, to time.Time
	var err error
	if f.From != "" {
		from, err = time.Parse(time.RFC3339, f.From)
		if err != nil {
			return bad("invalid from time")
		}
	}
	if f.To != "" {
		to, err = time.Parse(time.RFC3339, f.To)
		if err != nil {
			return bad("invalid to time")
		}
	}
	if !from.IsZero() && !to.IsZero() && !to.After(from) {
		return bad("end must follow start")
	}
	key := f
	key.Cursor = ""
	var cursor recordCursor
	if f.Cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(f.Cursor)
		if e != nil || len(raw) > 1024 || Decode(raw, &cursor) != nil || cursor.Project != project || cursor.Filter != Digest(key) || cursor.Commit != s.Commit || !recordID.MatchString(cursor.ID) || cursor.At.IsZero() {
			return bad("cursor no longer matches project, filters or Git snapshot; refresh records")
		}
	}
	rows := append([]RecordView{}, s.Records...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].At.Equal(rows[j].At) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].At.After(rows[j].At)
	})
	for _, v := range rows {
		if f.TestID != "" && f.TestID != v.TestID || f.Level != "" && f.Level != v.Level || f.Environment != "" && f.Environment != v.Environment || f.Search != "" && !strings.Contains(strings.ToLower(v.TestID), strings.ToLower(f.Search)) || f.Source != "" && !strings.Contains(v.Commit, strings.ToLower(f.Source)) || !from.IsZero() && v.At.Before(from) || !to.IsZero() && !v.At.Before(to) {
			continue
		}
		p.Summary.Total++
		if p.Summary.Latest == nil {
			x := v
			p.Summary.Latest = &x
		}
		if v.Status == "passed" && p.Summary.LastPassed == nil {
			x := v
			p.Summary.LastPassed = &x
		}
		if v.Status == "failed" {
			p.Summary.Failed++
		}
		if f.Outcome != "" && f.Outcome != v.Status {
			continue
		}
		if f.Cursor != "" && (v.At.After(cursor.At) || v.At.Equal(cursor.At) && v.ID >= cursor.ID) {
			continue
		}
		p.Records = append(p.Records, v)
	}
	if len(p.Records) > f.Limit {
		p.Records = p.Records[:f.Limit]
		last := p.Records[len(p.Records)-1]
		raw, _ := json.Marshal(recordCursor{Project: project, Filter: Digest(key), Commit: s.Commit, ID: last.ID, At: last.At})
		p.Cursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return p, nil
}
