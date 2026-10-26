package testmanagement

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/testmanagement/model"
)

func ProgressKey(testID string) string { return "test." + testID }

func InitialProgress(s Snapshot) (*runnercontract.ProgressFileInput, error) {
	if s.Status != "ready" {
		return nil, errors.New("progress requires a frozen ready test arrangement")
	}
	fields := map[string]bool{}
	for _, d := range s.Definitions {
		if d.ID == "" {
			return nil, errors.New("empty test progress ID")
		}
		if _, exists := fields[ProgressKey(d.ID)]; exists {
			return nil, errors.New("duplicate test progress ID")
		}
		fields[ProgressKey(d.ID)] = false
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	if len(raw) > runnercontract.MaxProgressBytes {
		return nil, errors.New("test progress exceeds Runner file limit")
	}
	return &runnercontract.ProgressFileInput{InitialContentBase64: base64.StdEncoding.EncodeToString(raw)}, nil
}

func ParseProgress(raw []byte, s Snapshot) (map[string]bool, error) {
	var values map[string]json.RawMessage
	if err := Decode(raw, &values); err != nil {
		return nil, fmt.Errorf("进度文件必须是布尔字段对象：%w", err)
	}
	if values == nil || len(values) != len(s.Definitions) {
		return nil, errors.New("进度字段与本次固定测试安排不一致")
	}
	fields := map[string]bool{}
	for _, d := range s.Definitions {
		v, ok := values[ProgressKey(d.ID)]
		if !ok || string(v) != "true" && string(v) != "false" {
			return nil, errors.New("进度字段必须与固定安排一致且只取 true/false")
		}
		fields[ProgressKey(d.ID)] = string(v) == "true"
	}
	return fields, nil
}

type ProgressStore interface {
	RunnerCandidates(context.Context, string, string) ([]runnerstate.Candidate, error)
	Progress(context.Context, string, string) (model.Progress, error)
	SaveProgress(context.Context, model.Progress, uint64) error
}

type ProgressRunner interface {
	ProgressFile(context.Context, string) (runnercontract.ProgressFile, error)
}

type ProgressService struct {
	Project         string
	Store           ProgressStore
	Runner          ProgressRunner
	Admit           func(context.Context) error
	mu              sync.Mutex
	nextObservation int
}

func (s *ProgressService) Observe(ctx context.Context) error {
	return s.observeRows(ctx, false)
}

// ObserveNext gives each candidate a turn without scanning every progress file
// before the actor can refresh live execution state again.
func (s *ProgressService) ObserveNext(ctx context.Context) error {
	return s.observeRows(ctx, true)
}

func (s *ProgressService) observeRows(ctx context.Context, one bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.Store.RunnerCandidates(ctx, s.Project, "")
	if err != nil {
		return err
	}
	var failures []error
	start := 0
	if one && len(rows) > 0 {
		start = s.nextObservation % len(rows)
	}
	for offset := range rows {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		index := (start + offset) % len(rows)
		c := rows[index]
		var plan Snapshot
		if c.RunID == "" || c.InputDigest == "" || c.Participation == "abandoned" || json.Unmarshal(c.TestPlan, &plan) != nil || plan.Status != "ready" || len(plan.Definitions) == 0 {
			continue
		}
		previous, err := s.Store.Progress(ctx, s.Project, c.ID)
		if errors.Is(err, model.ErrNotFound) {
			previous = model.Progress{Project: s.Project, CandidateID: c.ID, RunID: c.RunID, InputDigest: c.InputDigest}
		} else if err != nil {
			failures = append(failures, err)
			continue
		}
		if previous.RunID != c.RunID || previous.InputDigest != c.InputDigest {
			failures = append(failures, errors.New("progress original Run association differs"))
			continue
		}
		if previous.Snapshot.CollectionStatus == "closed" && previous.Diagnostic == "" && progressTerminal(c) {
			continue
		}
		if one {
			s.nextObservation = (index + 1) % len(rows)
		}
		next := previous
		callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = s.observeCandidate(callCtx, &next, plan)
		cancel()
		if err != nil {
			next.Diagnostic = err.Error()
		}
		if reflect.DeepEqual(previous, next) {
			if one {
				break
			}
			continue
		}
		next.Revision++
		if err = s.Store.SaveProgress(ctx, next, previous.Revision); err != nil {
			failures = append(failures, err)
		}
		if one {
			break
		}
	}
	return errors.Join(failures...)
}

// A later explicitly resumed original Run must become observable again.
func progressTerminal(c runnerstate.Candidate) bool {
	if c.Participation == "completed" {
		return true
	}
	if c.Projection == nil {
		return false
	}
	if c.Projection.StopStatus == "stopped" || c.Projection.Closed {
		return true
	}
	switch c.Projection.Status {
	case "pr_ready", "completed", "closed", "stopped":
		return true
	}
	return false
}

func (s *ProgressService) observeCandidate(ctx context.Context, r *model.Progress, plan Snapshot) error {
	if s.Admit != nil {
		if err := s.Admit(ctx); err != nil {
			return err
		}
	}
	p, err := s.Runner.ProgressFile(ctx, r.RunID)
	if err != nil {
		return err
	}
	raw, err := runnerclient.ValidateProgressFile(p, r.RunID)
	if err != nil {
		return err
	}
	old := r.Snapshot
	if p.Version < old.Version || p.Version == old.Version && p.Version > 0 && (p.SHA256 != old.SHA256 || p.SizeBytes != old.SizeBytes || !reflect.DeepEqual(p.ContentBase64, old.ContentBase64) || !reflect.DeepEqual(p.ObservedAt, old.ObservedAt)) {
		return errors.New("Runner progress version regressed or its saved content changed")
	}
	r.Snapshot = p
	r.Diagnostic = ""
	if p.Version > 0 {
		fields, err := ParseProgress(raw, plan)
		if err != nil {
			return err
		}
		r.Completed, r.ParsedVersion, r.ParsedAt = fields, p.Version, p.ObservedAt
	}
	if p.CollectionStatus == "unavailable" || p.CollectionStatus == "detached" || p.CollectionStatus == "disabled" {
		r.Diagnostic = "进度同步异常：" + p.CollectionStatus
		if p.Diagnostic != "" {
			r.Diagnostic += " · " + p.Diagnostic
		}
	}
	return nil
}

func ProgressView(s Snapshot, r model.Progress) View {
	v := View{Commit: s.Commit, Status: s.Status, PlanPath: s.PlanPath, Diagnostic: r.Diagnostic, Rows: []Row{}, ProgressVersion: r.ParsedVersion, ObservedAt: r.ParsedAt, CollectionStatus: r.Snapshot.CollectionStatus}
	if v.Diagnostic == "" {
		v.Diagnostic = s.Diagnostic
	}
	if s.Status != "ready" {
		return v
	}
	for _, d := range s.Definitions {
		row := Row{Definition: d, Status: "pending"}
		if r.ParsedVersion > 0 && r.Completed[ProgressKey(d.ID)] {
			row.Status = "passed"
			v.Passed++
		} else {
			v.Pending++
		}
		v.Total++
		v.Rows = append(v.Rows, row)
	}
	return v
}
