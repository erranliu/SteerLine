package testmanagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/testmanagement/model"
	"path"
	"strings"
	"sync"
	"time"
)

type EvidenceStore interface {
	RunnerCandidates(context.Context, string, string) ([]runnerstate.Candidate, error)
	Evidence(context.Context, string, string) (model.Evidence, error)
	SaveEvidence(context.Context, model.Evidence, uint64) error
}
type EvidenceRunner interface {
	DiscoverArtifacts(context.Context, string) (runnerclient.ArtifactDiscovery, error)
	ListArtifacts(context.Context, string) (runnerclient.ArtifactList, error)
	ReadArtifact(context.Context, string, string) (runnerclient.Artifact, []byte, error)
	CollectArtifact(context.Context, string, runnerclient.Request) (runnerclient.Artifact, error)
	WorkspaceGit(context.Context, string) (runnerclient.WorkspaceGit, error)
	Inspect(context.Context, string, string, bool) (runnerclient.Inspection, error)
}
type EvidenceService struct {
	AllowCollect func(context.Context, runnerstate.Candidate) bool
	Project      string
	Store        EvidenceStore
	Runner       EvidenceRunner
	Admit        func(context.Context) error
	mu           sync.Mutex
}

func (s *EvidenceService) save(ctx context.Context, r *model.Evidence) error {
	old := r.Revision
	r.Revision++
	return s.Store.SaveEvidence(ctx, *r, old)
}
func reportPath(p string) bool {
	return runnercontract.ValidArtifactPath(p) && strings.HasPrefix(path.Base(p), "managed-") && strings.HasSuffix(p, ".json")
}
func (s *EvidenceService) Observe(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.Store.RunnerCandidates(ctx, s.Project, "")
	if err != nil {
		return err
	}
	for _, c := range rows {
		var snap Snapshot
		if c.RunID == "" || c.InputDigest == "" || c.Participation == "abandoned" || json.Unmarshal(c.TestPlan, &snap) != nil || snap.Status != "ready" || len(snap.Definitions) == 0 {
			continue
		}
		r, err := s.Store.Evidence(ctx, s.Project, c.ID)
		if errors.Is(err, model.ErrNotFound) {
			r = model.Evidence{Project: s.Project, CandidateID: c.ID, RunID: c.RunID, InputDigest: c.InputDigest, Collections: []model.Collection{}, Artifacts: []model.EvidenceArtifact{}}
			if err = s.save(ctx, &r); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if r.RunID != c.RunID || r.InputDigest != c.InputDigest {
			return errors.New("test evidence original Run changed")
		}
		err = s.observeCandidate(ctx, &r, s.AllowCollect == nil || s.AllowCollect(ctx, c))
		if err != nil {
			r.Diagnostic = err.Error()
			r.Git = runnercontract.WorkspaceGit{RunID: r.RunID, ObservedAt: time.Now().UTC(), Diagnostic: err.Error()}
		} else {
			r.Diagnostic = ""
		}
		if err = s.save(ctx, &r); err != nil {
			return err
		}
	}
	return nil
}
func (s *EvidenceService) observeCandidate(ctx context.Context, r *model.Evidence, collect bool) error {
	if s.Admit != nil {
		if err := s.Admit(ctx); err != nil {
			return err
		}
	}
	// An uncertain original mutation is inspected, never automatically resent.
	for i := range r.Collections {
		op := &r.Collections[i]
		if op.State != "unknown" {
			continue
		}
		v, err := s.Runner.Inspect(ctx, "request-id", op.Request.RequestID, false)
		if err != nil {
			continue
		}
		if v.Command == nil || v.Command.RequestID != op.Request.RequestID || v.Command.Command != "artifacts collect" || v.Command.RunID == nil || *v.Command.RunID != r.RunID {
			return errors.New("collection original request association differs")
		}
		if v.Command.ActionStatus == "completed" {
			op.State = "completed"
		}
	}
	listed, err := s.Runner.ListArtifacts(ctx, r.RunID)
	if err != nil {
		return err
	}
	// Restore committed immutable evidence before asking for new files.
	if err = s.readListed(ctx, r, listed.Artifacts); err != nil {
		return err
	}
	if !collect {
		r.Git.Verified = false
		return nil
	}
	discovery, discoverErr := s.Runner.DiscoverArtifacts(ctx, r.RunID)
	if discoverErr == nil {
		readyDirs := map[string]bool{}
		for _, f := range discovery.Candidates {
			if reportPath(f.RelativePath) {
				readyDirs[path.Dir(f.RelativePath)] = true
			}
		}
		for _, f := range discovery.Candidates {
			if !reportPath(f.RelativePath) && path.Base(f.RelativePath) != "summary.json" {
				continue
			}
			if path.Base(f.RelativePath) == "summary.json" && !readyDirs[path.Dir(f.RelativePath)] {
				continue
			}
			known := false
			for _, a := range r.Artifacts {
				if a.Metadata.RelativePath == f.RelativePath {
					known = true
				}
			}
			for _, op := range r.Collections {
				if op.Path == f.RelativePath && op.State != "stale" {
					known = true
				}
			}
			if known {
				continue
			}
			inspection, err := s.Runner.Inspect(ctx, "run-id", r.RunID, false)
			if err != nil {
				return err
			}
			if inspection.Run == nil || inspection.Run.RunID != r.RunID || inspection.Run.InputDigest != r.InputDigest {
				return errors.New("test report Run binding differs")
			}
			body, _ := json.Marshal(runnercontract.CollectArtifactInput{ExpectedRevision: inspection.Run.Revision, RelativePath: f.RelativePath})
			op := model.Collection{Path: f.RelativePath, State: "unknown", Request: runnercontract.Request{Protocol: runnercontract.Protocol, RequestID: randomID(), Input: body}}
			r.Collections = append(r.Collections, op)
			if err = s.save(ctx, r); err != nil {
				return err
			}
			a, err := s.Runner.CollectArtifact(ctx, r.RunID, op.Request)
			last := &r.Collections[len(r.Collections)-1]
			if err != nil {
				last.Diagnostic = err.Error()
				var f *runnerclient.Failure
				if errors.As(err, &f) && !f.Unknown {
					if f.Code == "stale_revision" {
						last.State = "stale"
					} else {
						last.State = "rejected"
					}
				}
			} else {
				last.State = "completed"
				if a.RelativePath != op.Path {
					return errors.New("collection returned different path")
				}
				if err = s.readListed(ctx, r, []runnerclient.Artifact{a}); err != nil {
					return err
				}
			}
			if err = s.save(ctx, r); err != nil {
				return err
			}
		}
	}
	git, err := s.Runner.WorkspaceGit(ctx, r.RunID)
	if err != nil {
		return err
	}
	r.Git = git
	if discoverErr != nil {
		return discoverErr
	}
	return nil
}
func (s *EvidenceService) readListed(ctx context.Context, r *model.Evidence, artifacts []runnerclient.Artifact) error {
	for _, a := range artifacts {
		if !reportPath(a.RelativePath) && path.Base(a.RelativePath) != "summary.json" {
			continue
		}
		exists := false
		for _, old := range r.Artifacts {
			if old.Metadata.ArtifactID == a.ArtifactID {
				if Digest(old.Metadata) != Digest(a) {
					return errors.New("immutable artifact metadata changed")
				}
				exists = true
			}
		}
		if exists {
			continue
		}
		metadata, body, err := s.Runner.ReadArtifact(ctx, r.RunID, a.ArtifactID)
		if err != nil {
			return err
		}
		if Digest(metadata) != Digest(a) {
			return errors.New("read artifact identity differs")
		}
		if len(body) != a.SizeBytes || len(body) > runnercontract.MaxArtifactBytes || ContentDigest(body) != a.SHA256 {
			return errors.New("artifact content integrity differs")
		}
		r.Artifacts = append(r.Artifacts, model.EvidenceArtifact{Metadata: metadata, Content: body})
	}
	return nil
}

// EvidenceView performs no I/O. Fresh current Git is separate from retained history.
func EvidenceView(s Snapshot, r model.Evidence, now time.Time) View {
	reports := []Report{}
	diagnostic := r.Diagnostic
	byPath := map[string]model.EvidenceArtifact{}
	for _, a := range r.Artifacts {
		if old, ok := byPath[a.Metadata.RelativePath]; ok && old.Metadata.SHA256 != a.Metadata.SHA256 {
			diagnostic = "同一执行产物路径被重写"
		}
		byPath[a.Metadata.RelativePath] = a
	}
	for _, op := range r.Collections {
		if op.State != "completed" {
			if _, ok := byPath[op.Path]; !ok {
				diagnostic = "测试报告收集尚未核实：" + op.Diagnostic
			}
		}
	}
	seen := map[string]string{}
	order := map[uint64]map[string]bool{}
	for _, a := range r.Artifacts {
		if !reportPath(a.Metadata.RelativePath) {
			continue
		}
		report, err := ParseReport(a.Content)
		if err != nil {
			diagnostic = err.Error()
			continue
		}
		digest := ContentDigest(a.Content)
		if old, ok := seen[report.ID]; ok {
			if old != digest {
				diagnostic = "同一测试执行ID的报告冲突"
			}
			continue
		}
		seen[report.ID] = digest
		if order[report.Sequence] == nil {
			order[report.Sequence] = map[string]bool{}
		}
		for _, id := range report.Selection {
			if order[report.Sequence][id] {
				diagnostic = "同一测试的执行顺序冲突"
			}
			order[report.Sequence][id] = true
		}
		valid := true
		for i, item := range report.Results {
			if !runnercontract.ValidArtifactPath(item.Log) {
				valid = false
				break
			}
			log, ok := byPath[path.Join(path.Dir(a.Metadata.RelativePath), item.Log)]
			if !ok {
				valid = false
				break
			}
			if len(log.Content) != log.Metadata.SizeBytes || ContentDigest(log.Content) != log.Metadata.SHA256 || ContentDigest(a.Content) != a.Metadata.SHA256 || len(log.Content) != report.Evidence.Bytes || ContentDigest(log.Content) != report.Evidence.SHA256 {
				valid = false
				break
			}
			report.Results[i].Log = fmt.Sprintf("Run %s · %s · artifact %s · SHA256 %s", r.RunID, log.Metadata.RelativePath, log.Metadata.ArtifactID, log.Metadata.SHA256)
		}
		if valid {
			reports = append(reports, report)
		} else {
			diagnostic = "测试原始证据缺失或摘要不匹配"
		}
	}
	verified := r.Git.Verified && r.Git.RunID == r.RunID && r.Git.Commit != nil && r.Git.Clean != nil && now.Sub(r.Git.ObservedAt) >= 0 && now.Sub(r.Git.ObservedAt) < 20*time.Second && diagnostic == ""
	commit := ""
	clean := false
	if r.Git.Commit != nil {
		commit = *r.Git.Commit
	}
	if r.Git.Clean != nil {
		clean = *r.Git.Clean
	}
	v := Project(s, reports, commit, clean, verified)
	v.Diagnostic = diagnostic
	if !verified && v.Diagnostic == "" {
		v.Diagnostic = "当前代码来源未核实；历史结果待复验"
	}
	return v
}
