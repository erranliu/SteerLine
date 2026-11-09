// Package mainagentbridge owns host-side advisory work. The advisory core knows none of these facts.
package mainagentbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/mainagent"
	"github.com/erranliu/SteerLine/internal/mainagent/contract"
)

const NativeUnavailable = "严格给定上下文/零工具能力尚未核验，生成保持阻断；未提交模型请求"

// There is deliberately no production model client in this draft. Only a newly
// verified native capability may add one. No setting or model text can open it.
func ProductionAvailable() bool { return false }

type Source struct {
	CandidateID    string              `json:"candidate_id"`
	TaskID         string              `json:"task_id"`
	Occurrence     string              `json:"occurrence"`
	Revision       uint64              `json:"revision"`
	Reason         string              `json:"reason"`
	Goal           string              `json:"goal"`
	BaseRevision   string              `json:"base_revision,omitempty"`
	ContractDigest string              `json:"contract_digest,omitempty"`
	Facts          []contract.Material `json:"facts,omitempty"`
}

func (s Source) Valid() bool {
	return contract.ID.MatchString(s.CandidateID) && s.Occurrence != "" && utf8.ValidString(s.Occurrence) && len(s.Occurrence) <= 1024 && s.Revision > 0 && strings.TrimSpace(s.Goal) != "" && utf8.ValidString(s.Goal) && len(s.Goal) <= contract.MaxPromptBytes && (s.Reason == "requirements_unclear" || s.Reason == "requirements_conflict")
}
func SourceID(project string, s Source) string {
	raw, _ := json.Marshal([]string{project, "candidate-material", s.CandidateID, s.Occurrence, contract.Protocol})
	return contract.Digest(raw)[:32]
}

type Record struct {
	Project      string               `json:"project"`
	ID           string               `json:"id"`
	Revision     uint64               `json:"revision"`
	Source       Source               `json:"source"`
	SourceActive bool                 `json:"source_active"`
	State        string               `json:"state"`
	CreatedAt    time.Time            `json:"created_at"`
	FinishedAt   *time.Time           `json:"finished_at,omitempty"`
	ObservedAt   time.Time            `json:"observed_at"`
	Request      *contract.Request    `json:"request,omitempty"`
	InputDigest  string               `json:"input_digest,omitempty"`
	Suggestion   *contract.Suggestion `json:"suggestion,omitempty"`
	Diagnostic   string               `json:"diagnostic,omitempty"`
	Settled      bool                 `json:"settled"`
}

func (r Record) Complete() bool {
	return r.Settled && (r.State == "blocked" || r.State == "failed" || r.State == "cancelled" || r.State == "ready")
}

type Store interface {
	LoadAdvisory(context.Context, string, string) ([]byte, uint64, error)
	SaveAdvisory(context.Context, string, string, string, []byte, uint64) error
	AdvisoryIDs(context.Context, string, string) ([]string, error)
	AdvisorySummaries(context.Context, string, string) ([]json.RawMessage, bool, error)
}
type Service struct {
	Project string
	Store   Store
	// Build is called only by the host's worker, never by the advisory core or a query.
	Build    func(context.Context, Source, string) (contract.Request, error)
	gateOnce sync.Once
	gate     chan struct{}
	inflight sync.Map
}

func (s *Service) enter(ctx context.Context) error {
	s.gateOnce.Do(func() { s.gate = make(chan struct{}, 1) })
	select {
	case s.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Service) leave() { <-s.gate }

func (s *Service) load(ctx context.Context, id string) (Record, error) {
	raw, rev, err := s.Store.LoadAdvisory(ctx, s.Project, id)
	var r Record
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(raw, &r); err != nil {
		return r, err
	}
	if r.ID != id || r.Project != s.Project || r.Revision != rev {
		return r, errors.New("advisory identity or revision mismatch")
	}
	return r, nil
}
func (s *Service) save(ctx context.Context, r Record) (Record, error) {
	expected := r.Revision
	r.Revision++
	r.ObservedAt = time.Now().UTC()
	raw, err := json.Marshal(r)
	if err == nil {
		err = s.Store.SaveAdvisory(ctx, r.Project, r.ID, r.Source.CandidateID, raw, expected)
	}
	return r, err
}
func (s *Service) Get(ctx context.Context, id string) (Record, error) { return s.load(ctx, id) }
func (s *Service) List(ctx context.Context, candidate string) ([]json.RawMessage, bool, error) {
	return s.Store.AdvisorySummaries(ctx, s.Project, candidate)
}

// Sync consumes explicit owner facts. Plain questions and arbitrary attention flags are not classifications.
func (s *Service) Sync(ctx context.Context, sources []Source) error {
	if err := s.enter(ctx); err != nil {
		return err
	}
	defer s.leave()
	current := map[string]bool{}
	for _, source := range sources {
		if !source.Valid() {
			return errors.New("invalid explicit material advisory source")
		}
		id := SourceID(s.Project, source)
		current[id] = true
		if _, err := s.load(ctx, id); err == nil {
			continue
		} else if !errors.Is(err, contract.ErrNotFound) {
			return err
		}
		r := Record{Project: s.Project, ID: id, Source: source, SourceActive: true, State: "queued", CreatedAt: time.Now().UTC(), Settled: true}
		if _, err := s.save(ctx, r); err != nil && !errors.Is(err, contract.ErrConflict) {
			return err
		}
	}
	ids, err := s.Store.AdvisoryIDs(ctx, s.Project, "active")
	if err != nil {
		return err
	}
	for _, id := range ids {
		if current[id] {
			continue
		}
		r, err := s.load(ctx, id)
		if err != nil {
			return err
		}
		r.SourceActive = false
		if !r.Complete() {
			if cancel, ok := s.inflight.Load(id); ok {
				cancel.(context.CancelFunc)()
			}
			finish(&r, "cancelled", "原事项已解除，未提交模型")
		}
		if _, err = s.save(ctx, r); err != nil {
			return err
		}
	}
	return nil
}
func finish(r *Record, state, diagnostic string) {
	now := time.Now().UTC()
	r.State = state
	r.Diagnostic = diagnostic
	r.FinishedAt = &now
	r.Settled = true
}

// ProcessNext is asynchronous host work. Production inference remains fail-closed
// until the native precondition has passed; there is no read-only fallback.
func (s *Service) ProcessNext(ctx context.Context) error {
	if err := s.enter(ctx); err != nil {
		return err
	}
	ids, err := s.Store.AdvisoryIDs(ctx, s.Project, "pending")
	if err != nil {
		s.leave()
		return err
	}
	var id string
	for _, candidate := range ids {
		if _, busy := s.inflight.Load(candidate); !busy {
			id = candidate
			break
		}
	}
	if id == "" {
		s.leave()
		return nil
	}
	r, err := s.load(ctx, id)
	if err != nil {
		s.leave()
		return err
	}
	if !r.SourceActive {
		finish(&r, "cancelled", "原事项已解除")
		_, err = s.save(ctx, r)
		s.leave()
		return err
	}
	r.State = "preparing"
	r, err = s.save(ctx, r)
	if err != nil {
		s.leave()
		return err
	}
	work, cancel := context.WithCancel(ctx)
	s.inflight.Store(id, cancel)
	defer s.inflight.Delete(id)
	s.leave()
	defer cancel()
	request, buildErr := s.Build(work, r.Source, r.ID)
	var prepared mainagent.Prepared
	if buildErr == nil {
		prepared, buildErr = mainagent.Prepare(request)
	}
	// Preparation is outside the gate: explicit cancellation/source resolution can
	// persist first. A cancelled or newer record always wins over this result.
	saveCtx, stopSave := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer stopSave()
	if err = s.enter(saveCtx); err != nil {
		return err
	}
	defer s.leave()
	current, err := s.load(saveCtx, id)
	if err != nil {
		return err
	}
	if current.Complete() {
		return nil
	}
	if current.Revision != r.Revision {
		return contract.ErrConflict
	}
	if work.Err() != nil {
		finish(&current, "cancelled", "上下文准备已取消，未提交模型")
	} else if buildErr != nil {
		finish(&current, "failed", fmt.Sprintf("上下文准备失败：%v", buildErr))
	} else {
		current.Request = &request
		current.InputDigest = prepared.Digest
		finish(&current, "blocked", NativeUnavailable)
	}
	_, err = s.save(saveCtx, current)
	return err
}
func (s *Service) Cancel(ctx context.Context, id string, expected uint64) (Record, error) {
	if err := s.enter(ctx); err != nil {
		return Record{}, err
	}
	defer s.leave()
	r, err := s.load(ctx, id)
	if err != nil {
		return r, err
	}
	if r.Revision != expected {
		return r, contract.ErrConflict
	}
	if r.Complete() {
		if r.State == "cancelled" {
			return r, nil
		}
		return r, errors.New("本次生成已结束，未执行取消")
	}
	if cancel, ok := s.inflight.Load(id); ok {
		cancel.(context.CancelFunc)()
	}
	finish(&r, "cancelled", "本次提示词生成已取消，未提交模型")
	return s.save(ctx, r)
}

// Detail is the public host projection, independent of the stored Record shape.
type Detail struct {
	ID          string            `json:"id"`
	Revision    uint64            `json:"revision"`
	CandidateID string            `json:"candidate_id"`
	State       string            `json:"state"`
	Context     *contract.Context `json:"context,omitempty"`
	Prompt      string            `json:"prompt,omitempty"`
	Diagnostic  string            `json:"diagnostic,omitempty"`
	Settled     bool              `json:"settled"`
}

func View(r Record) Detail {
	d := Detail{ID: r.ID, Revision: r.Revision, CandidateID: r.Source.CandidateID, State: r.State, Diagnostic: r.Diagnostic, Settled: r.Settled}
	if r.Request != nil {
		d.Context = &r.Request.Context
	}
	if r.Suggestion != nil {
		d.Prompt = r.Suggestion.Prompt
	}
	return d
}
func (s *Service) Close(ctx context.Context) error {
	ids, err := s.Store.AdvisoryIDs(ctx, s.Project, "pending")
	if err != nil {
		return err
	}
	for _, id := range ids {
		r, e := s.load(ctx, id)
		if e != nil {
			return e
		}
		if _, e = s.Cancel(ctx, id, r.Revision); e != nil {
			return e
		}
	}
	return nil
}
