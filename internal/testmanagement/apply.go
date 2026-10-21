package testmanagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	"github.com/erranliu/SteerLine/internal/testmanagement/model"
	"os"
	"path/filepath"
	"strings"
)

func output(r Generation) ([]byte, error) {
	if len(r.Result) == 0 {
		return nil, errors.New("generation has no verified result")
	}
	if _, e := Select(r.Result, r.Catalogue); e != nil {
		return nil, e
	}
	var a Arrangement
	if e := Decode(r.Result, &a); e != nil {
		return nil, e
	}
	b, e := json.MarshalIndent(a, "", "  ")
	if len(b)+1 > MaxDocumentBytes {
		return nil, errors.New("formatted output exceeds 1 MiB")
	}
	return append(b, '\n'), e
}
func (s *GenerationService) preview(ctx context.Context, r Generation) (model.Preview, error) {
	var v model.Preview
	if s.Workspace == nil {
		return v, errors.New("Git workspace observation unavailable")
	}
	if r.Cancelled {
		return v, errors.New("generation cancelled")
	}
	if r.Projection == nil || r.Projection.ContentResult == nil || r.Projection.ContentResult.Status != "succeeded" || !r.Projection.ContentResult.Source.Verified {
		return v, errors.New("verified generation result required")
	}
	after, e := output(r)
	if e != nil {
		return v, e
	}
	before, e := ReadProjectFile(s.Repository, r.Target)
	if e != nil && !os.IsNotExist(e) {
		return v, e
	}
	target, e := targetDigest(s.Repository, r.Target)
	if e != nil {
		return v, e
	}
	workspace, e := s.Workspace(ctx)
	if e != nil {
		return v, e
	}
	originalTarget := strings.Split(r.Workspace, "\n")
	currentTarget := strings.Split(workspace, "\n")
	if len(originalTarget) == 3 && (len(currentTarget) != 3 || originalTarget[2] != currentTarget[2]) {
		return v, errors.New("repository target changed since generation")
	}
	guards := []string{s.Project, r.ID, r.Target, ContentDigest(after), workspace, target}
	for _, source := range r.Sources {
		b, e := ReadProjectFile(s.Repository, source.Path)
		if e != nil {
			return v, e
		}
		if pm.TextDigest(b) != source.TextDigest {
			return v, fmt.Errorf("generation input changed: %s", source.Path)
		}
		guards = append(guards, source.Path, ContentDigest(b))
	}
	end, e := s.Workspace(ctx)
	if e != nil {
		return v, e
	}
	checkTarget, e := targetDigest(s.Repository, r.Target)
	if e != nil {
		return v, e
	}
	if checkTarget != target || (target != "missing" && ContentDigest(before) != target) {
		return v, errors.New("target changed during preview")
	}
	if end != workspace {
		return v, errors.New("workspace changed during preview")
	}
	v.ReviewDigest = pm.Hash(guards)
	v.Before = string(before)
	v.After = string(after)
	return v, nil
}

// Preview is read-only: no new Runner invocation, application or approval.
func (s *GenerationService) Preview(ctx context.Context, r Generation) (model.Preview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.preview(ctx, r)
}

var ErrApplicationUnknown = errors.New("application outcome requires original request reconciliation")

func (s *GenerationService) Apply(ctx context.Context, in model.ApplyCommand) (out Generation, err error) {
	entered := false
	defer func() {
		if entered && err != nil {
			err = fmt.Errorf("%w: %v", ErrApplicationUnknown, err)
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !requestID.MatchString(in.RequestID) || in.GenerationID == "" || in.ExpectedRevision == 0 || len(in.ReviewDigest) != 64 {
		return Generation{}, errors.New("fixed apply request and review digest required")
	}
	r, e := s.Store.Generation(ctx, s.Project, in.GenerationID)
	if e != nil {
		return r, e
	}
	var app *model.Application
	for n := range r.Applications {
		if r.Applications[n].Command.RequestID == in.RequestID {
			if r.Applications[n].Command != in {
				return r, errors.New("request_conflict: application input differs")
			}
			app = &r.Applications[n]
			break
		}
	}
	if app != nil && app.State == "applied" {
		return r, nil
	}
	if r.Cancelled {
		return r, errors.New("cancelled generation cannot start a file write")
	}
	data, e := output(r)
	if e != nil {
		return r, e
	}
	var a Arrangement
	_ = Decode(r.Result, &a)
	if len(a.Tests) == 0 {
		plan, e := ReadProjectFile(s.Repository, r.Command.Plan)
		if e != nil {
			return r, e
		}
		if !in.AllowEmpty || !strings.Contains(string(plan), "无需自动测试") && !strings.Contains(strings.ToLower(string(plan)), "no automated tests") {
			return r, errors.New("empty arrangement requires explicit review and Plan explanation")
		}
	}
	current, e := targetDigest(s.Repository, r.Target)
	if e != nil {
		return r, e
	}
	workspace, e := s.Workspace(ctx)
	if e != nil {
		return r, e
	}
	if app == nil {
		if r.Revision != in.ExpectedRevision {
			return r, errors.New("stale_revision: generation changed")
		}
		v, e := s.preview(ctx, r)
		if e != nil {
			return r, e
		}
		if v.ReviewDigest != in.ReviewDigest {
			return r, errors.New("stale_review: reviewed workspace or files changed")
		}
		entered = true
		r.Applications = append(r.Applications, model.Application{Command: in, Before: current, Output: ContentDigest(data), Workspace: workspace, State: "intent"})
		r.Revision++
		if e = s.Store.SaveGeneration(ctx, r, r.Revision-1); e != nil {
			return r, e
		}
		app = &r.Applications[len(r.Applications)-1]
	} else {
		entered = true
		if current != app.Output && (current != app.Before || workspace != app.Workspace) {
			return r, errors.New("application conflicts with current workspace")
		}
		// Review also rechecks all original inputs before any continuation.
		if current != app.Output {
			v, e := s.preview(ctx, r)
			if e != nil {
				return r, e
			}
			if v.ReviewDigest != in.ReviewDigest {
				return r, errors.New("application source changed")
			}
		}
	}
	if current != app.Output {
		// Recheck every reviewed precondition after durable intent; queries never enter here.
		v, err := s.preview(ctx, r)
		if err != nil {
			return r, err
		}
		if v.ReviewDigest != in.ReviewDigest {
			return r, errors.New("review changed after application intent")
		}
		// Recheck the target immediately before replacing; queries never enter here.
		current, e = targetDigest(s.Repository, r.Target)
		if e != nil {
			return r, e
		}
		if current != app.Before {
			return r, errors.New("application target changed")
		}
		target := filepath.Join(s.Repository, filepath.FromSlash(r.Target))
		f, e := os.CreateTemp(filepath.Dir(target), ".steerline-tests-*")
		if e != nil {
			return r, e
		}
		name := f.Name()
		defer os.Remove(name)
		if _, e = f.Write(data); e == nil {
			e = f.Sync()
		}
		e = errors.Join(e, f.Close())
		if e != nil {
			return r, e
		}
		v, e = s.preview(ctx, r)
		if e != nil {
			return r, e
		}
		if v.ReviewDigest != in.ReviewDigest {
			return r, errors.New("review changed before atomic replacement")
		}
		if e = os.Rename(name, target); e != nil {
			return r, e
		}
	}
	app.State = "applied"
	r.State = "applied"
	r.Revision++
	e = s.Store.SaveGeneration(ctx, r, r.Revision-1)
	return r, e
}

// reconcileApplications only records already-present output. It never creates,
// replaces, deletes or rolls back a project file, including after cancellation.
func (s *GenerationService) reconcileApplications(ctx context.Context, r Generation) (Generation, error) {
	changed := false
	for i := range r.Applications {
		a := &r.Applications[i]
		if a.State != "intent" {
			continue
		}
		current, e := targetDigest(s.Repository, r.Target)
		if e != nil {
			return r, e
		}
		if current == a.Output {
			a.State = "applied"
			changed = true
		}
	}
	if !changed {
		return r, nil
	}
	if !r.Cancelled {
		r.State = "applied"
	}
	r.Revision++
	return r, s.Store.SaveGeneration(ctx, r, r.Revision-1)
}
