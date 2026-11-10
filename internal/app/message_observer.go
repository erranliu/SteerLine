package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

type messageObservationWriter interface {
	SaveMessageObservation(context.Context, ports.MessageObservation) error
	LoadMessageObservation(context.Context, ports.MessageObservation) (ports.MessageObservation, error)
}
type messageEntry struct {
	attempt time.Time
	running chan struct{}
	view    api.LastMessageView
	err     error
}
type messageProject struct {
	ctx     context.Context
	writer  messageObservationWriter
	workers sync.WaitGroup
}

// Only public reads are admitted here. This cache has no command or recovery API.
type messageObserver struct {
	mu       sync.Mutex
	entries  map[string]*messageEntry
	projects map[string]*messageProject
	slots    chan struct{}
	now      func() time.Time
}

func newMessageObserver() *messageObserver {
	return &messageObserver{entries: map[string]*messageEntry{}, projects: map[string]*messageProject{}, slots: make(chan struct{}, 2), now: time.Now}
}
func (o *messageObserver) register(ctx context.Context, project string, connection any) {
	w, _ := connection.(messageObservationWriter)
	o.mu.Lock()
	o.projects[project] = &messageProject{ctx: ctx, writer: w}
	o.mu.Unlock()
}

// Called after project cancellation and before closing its original store.
func (o *messageObserver) release(project string) {
	o.mu.Lock()
	p := o.projects[project]
	delete(o.projects, project)
	o.mu.Unlock()
	if p != nil {
		p.workers.Wait()
	}
}
func messageKey(c runnerstate.Candidate) string {
	return c.StateDir + "\x00" + c.Project + "\x00" + c.ID + "\x00" + c.RunID + "\x00" + c.Projection.Execution.AttemptID + "\x00" + c.Projection.Execution.SessionID
}

func (o *messageObserver) observe(ctx context.Context, c runnerstate.Candidate, cli runnerbridge.CLI, writer messageObservationWriter, background bool) (api.LastMessageView, error) {
	view := api.LastMessageView{RunID: c.RunID}
	if c.Projection == nil || c.RunID == "" || c.Projection.Execution.AttemptID == "" || c.Projection.Execution.SessionID == "" {
		return view, nil
	}
	view.AttemptID = c.Projection.Execution.AttemptID
	if background && (c.Projection.Status == "completed" || c.Projection.Status == "closed" || c.Projection.Status == "failed" || c.Projection.Status == "stopped") {
		return view, nil
	}
	key := messageKey(c)
	now := o.now()
	interval := 3 * time.Second
	if background {
		interval = 10 * time.Second
	}
	o.mu.Lock()
	project := o.projects[c.Project]
	if project != nil {
		writer = project.writer
	}
	if background {
		if project == nil || project.ctx.Err() != nil {
			o.mu.Unlock()
			return view, nil
		}
		ctx = project.ctx
	}
	for k, e := range o.entries {
		if e.running == nil && now.Sub(e.attempt) > 10*time.Minute {
			delete(o.entries, k)
		}
	}
	e := o.entries[key]
	if e == nil {
		if len(o.entries) >= 1024 {
			o.mu.Unlock()
			return view, errors.New("message observation cache busy")
		}
		e = &messageEntry{view: view}
		o.entries[key] = e
	}
	if e.running != nil {
		done := e.running
		o.mu.Unlock()
		if background {
			return view, nil
		}
		select {
		case <-ctx.Done():
			return view, ctx.Err()
		case <-done:
			o.mu.Lock()
			v, err := e.view, e.err
			o.mu.Unlock()
			return v, err
		}
	}
	if !e.attempt.IsZero() && now.Sub(e.attempt) < interval {
		v, err := e.view, e.err
		o.mu.Unlock()
		return v, err
	}
	e.attempt = now
	e.running = make(chan struct{})
	if background {
		project.workers.Add(1)
	}
	o.mu.Unlock()
	run := func() (api.LastMessageView, error) {
		bounded, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if background {
			select {
			case o.slots <- struct{}{}:
			default:
				return view, nil
			}
		} else {
			select {
			case o.slots <- struct{}{}:
			case <-bounded.Done():
				return view, bounded.Err()
			}
		}
		defer func() { <-o.slots }()
		response, err := cli.Call(bounded, "last-message", []string{"--run-id", c.RunID}, nil)
		if err != nil {
			return view, err
		}
		var sampled api.LastMessageView
		if json.Unmarshal(response.Data, &sampled) != nil || sampled.RunID != c.RunID || sampled.AttemptID != view.AttemptID {
			return view, errors.New("Runner last message target changed")
		}
		if m := sampled.Message; m != nil && (m.RunID != c.RunID || m.AttemptID != view.AttemptID || m.SessionID != c.Projection.Execution.SessionID || m.TurnID == "" || m.ObservedAt.IsZero() || len(m.Text) > 4096 || !utf8.ValidString(m.Text) || (m.Kind != "assistant" && m.Kind != "tool")) {
			return view, errors.New("Runner last message identity or payload invalid")
		}
		return sampled, nil
	}
	finish := func() (api.LastMessageView, error) {
		// A process restart must not erase a persisted assistant sample when the
		// next public observation is null or a tool message.
		o.mu.Lock()
		empty := e.view.Message == nil
		o.mu.Unlock()
		if empty && writer != nil {
			bounded, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
			m, loadErr := writer.LoadMessageObservation(bounded, ports.MessageObservation{Project: c.Project, CandidateID: c.ID, RunID: c.RunID, AttemptID: view.AttemptID, SessionID: c.Projection.Execution.SessionID})
			cancel()
			if loadErr == nil && m.Text != "" {
				if at, parseErr := time.Parse(time.RFC3339Nano, m.ObservedAt); parseErr == nil {
					o.mu.Lock()
					e.view.Message = &api.LastMessage{RunID: c.RunID, AttemptID: view.AttemptID, SessionID: m.SessionID, TurnID: m.TurnID, Kind: "assistant", Text: m.Text, ObservedAt: at}
					o.mu.Unlock()
				}
			}
		}
		sampled, err := run()
		o.mu.Lock()
		if err == nil && sampled.Message != nil && sampled.Message.Kind == "assistant" {
			e.view = sampled
		}
		e.err = err
		v := e.view
		o.mu.Unlock()
		if writer != nil {
			m := ports.MessageObservation{Project: c.Project, CandidateID: c.ID, RunID: c.RunID, AttemptID: view.AttemptID, SessionID: c.Projection.Execution.SessionID}
			if v.Message != nil {
				m.Text = v.Message.Text
				m.TurnID = v.Message.TurnID
				m.ObservedAt = v.Message.ObservedAt.UTC().Format(time.RFC3339Nano)
			}
			if err != nil {
				m.Diagnostic = err.Error()
				if len(m.Diagnostic) > 1024 {
					m.Diagnostic = m.Diagnostic[:1024]
					for !utf8.ValidString(m.Diagnostic) {
						m.Diagnostic = m.Diagnostic[:len(m.Diagnostic)-1]
					}
				}
			}
			persist, stop := context.WithTimeout(ctx, 100*time.Millisecond)
			_ = writer.SaveMessageObservation(persist, m)
			stop()
		}
		o.mu.Lock()
		done := e.running
		e.running = nil
		close(done)
		o.mu.Unlock()
		return v, err
	}
	if background {
		go func() { defer project.workers.Done(); _, _ = finish() }()
		return view, nil
	}
	return finish()
}

func bindMessageObserver(config Config, connection any, cli runnerbridge.CLI, c runnerstate.Candidate, ctx context.Context) {
	if config.runtimeMessages == nil {
		return
	}
	if config.runtimeRunner == nil || !strings.EqualFold(filepath.Clean(c.StateDir), filepath.Clean(config.runtimeRunner.StateDir)) {
		return
	}
	w, _ := connection.(messageObservationWriter)
	_, _ = config.runtimeMessages.observe(ctx, c, cli, w, true)
}
