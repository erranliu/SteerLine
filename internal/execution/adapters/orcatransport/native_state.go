package orcatransport

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

type NativeProof struct {
	Pane          string    `json:"pane"`
	Provider      string    `json:"provider,omitempty"`
	BaseAuthority string    `json:"base_authority,omitempty"`
	BaseRevision  int64     `json:"base_revision,omitempty"`
	BaseRootAt    int64     `json:"base_root_at,omitempty"`
	Authority     string    `json:"authority,omitempty"`
	Incarnation   int64     `json:"incarnation,omitempty"`
	Revision      int64     `json:"revision,omitempty"`
	WorkingAt     int64     `json:"working_at,omitempty"`
	RootWorkingAt int64     `json:"root_working_at,omitempty"`
	EndedAt       int64     `json:"ended_at,omitempty"`
	Outcome       string    `json:"outcome,omitempty"`
	ReplyDigest   string    `json:"reply_digest,omitempty"`
	ReplyBody     string    `json:"reply_body,omitempty"`
	LastSignal    time.Time `json:"last_signal,omitempty"`
	LastQuery     time.Time `json:"last_query,omitempty"`
	Hint          string    `json:"hint,omitempty"`
	HintPending   bool      `json:"hint_pending,omitempty"`
	Diagnostic    string    `json:"diagnostic,omitempty"`
}

func stateHint(s native.AgentState) string {
	return s.PaneKey + ":" + s.State + ":" + strconv.FormatInt(s.StateStartedAt, 10) + ":" + strconv.FormatInt(s.UpdatedAt, 10)
}
func nativeBaseline(s native.AgentState, now time.Time) NativeProof {
	p := NativeProof{Pane: s.PaneKey, Provider: s.ProviderSession.ID, LastSignal: now, LastQuery: now, Hint: stateHint(s)}
	if trustedState(s) {
		p.BaseAuthority = s.Observation.AuthorityID
		p.BaseRevision = s.Observation.Revision
	}
	if s.MainAgent != nil {
		p.BaseRootAt = s.MainAgent.StateStartedAt
	}
	return p
}
func trustedState(s native.AgentState) bool {
	o := s.Observation
	return s.AgentType == "codex" && s.MainAgent != nil && o != nil && o.Origin == "hook" && o.Kind == "transition" && o.AuthorityID != "" && o.Revision > 0 && o.ObservedAt > 0
}
func preview(text string) string {
	runes := []rune(strings.Join(strings.Fields(text), " "))
	count, end := 0, 0
	for end < len(runes) {
		n := 1
		if runes[end] > 0xffff {
			n = 2
		}
		if count+n > 200 {
			break
		}
		count += n
		end++
	}
	return string(runes[:end])
}

func (p *NativeProof) body(message string) {
	// The public Codex hook field is capped at 8000 UTF-16 code units. Reject
	// the surrogate-safe truncation boundary too; never treat a prefix as full.
	if strings.TrimSpace(message) == "" || len(utf16.Encode([]rune(message))) >= 7999 {
		return
	}
	body := message
	hash := digest([]byte(body))
	// The first complete native original is immutable; no display normalization.
	if p.ReplyDigest != "" && (p.ReplyDigest != hash || p.ReplyBody != body) {
		p.Diagnostic = "Native ended reply changed; current result is unverified"
		return
	}
	p.ReplyDigest = hash
	p.ReplyBody = body
}
func (p *NativeProof) signal(s native.AgentState, turn *Turn, now time.Time) {
	if p.Diagnostic != "" && p.Diagnostic != "Native main agent is waiting; no next input was sent" {
		return
	}
	if s.PaneKey != p.Pane {
		p.Diagnostic = "Native original pane changed"
		return
	}
	if p.Provider != "" && s.ProviderSession.ID != "" && s.ProviderSession.ID != p.Provider {
		p.Diagnostic = "Native original provider session changed"
		return
	}
	if stateHint(s) != p.Hint {
		p.Hint = stateHint(s)
		if s.State == "done" {
			p.HintPending = true
		}
	} // hint only; require a main-agent query
	if !trustedState(s) || !turn.Accepted {
		return
	}
	o := s.Observation
	if p.Authority != "" && (o.AuthorityID != p.Authority || o.Incarnation != p.Incarnation) {
		p.Diagnostic = "Native main-agent authority changed"
		return
	}
	if p.BaseAuthority != "" && o.AuthorityID != p.BaseAuthority {
		p.Diagnostic = "Native baseline authority changed"
		return
	}
	if o.Revision <= p.BaseRevision || o.Revision <= p.Revision {
		return
	}
	if p.rejectProvenance(s, turn) {
		return
	}
	if p.WorkingAt == 0 {
		if s.MainAgent.State == "done" {
			// Without a streamed start, confirm the retained main-agent record
			// through the independent query; never promote a tab snapshot alone.
			return
		}
		if s.MainAgent.State != "working" || s.MainAgent.StateStartedAt <= p.BaseRootAt || !turn.Started || s.ProviderSession.ID == "" || s.Prompt != preview(turn.Prompt) {
			return
		}
		p.Authority, p.Incarnation, p.Provider = o.AuthorityID, o.Incarnation, s.ProviderSession.ID
		p.WorkingAt = o.ObservedAt
		p.RootWorkingAt = s.MainAgent.StateStartedAt
	} else if o.Boundary && s.MainAgent.State == "working" && s.MainAgent.StateStartedAt != p.RootWorkingAt {
		p.Diagnostic = "Another native input boundary makes the original turn unverified"
		return
	}
	p.Revision, p.LastSignal = o.Revision, now
	if s.MainAgent.State == "done" && s.MainAgent.StateStartedAt > p.WorkingAt {
		p.end(s, turn, now)
	} else if s.MainAgent.State == "waiting" || s.MainAgent.State == "blocked" {
		// Waiting remains in the current turn; it is not a completion.
		p.Diagnostic = "Native main agent is waiting; no next input was sent"
	} else if p.Diagnostic == "Native main agent is waiting; no next input was sent" {
		p.Diagnostic = ""
	}
}
func (p *NativeProof) rejectProvenance(s native.AgentState, turn *Turn) bool {
	if s.MainAgent != nil && s.MainAgent.StateStartedAt > p.BaseRootAt && (s.MainAgent.State == "working" || s.MainAgent.State == "done") &&
		(s.SessionBoundary || s.RestoredUnconfirmed || s.Prompt != preview(turn.Prompt)) {
		p.Diagnostic = "Native original completion/input provenance is unverified"
		return true
	}
	return false
}
func (p *NativeProof) end(s native.AgentState, turn *Turn, now time.Time) bool {
	if !turn.Accepted || !turn.Started || p.Provider == "" || s.MainAgent == nil || s.MainAgent.State != "done" || s.AgentType != "codex" || s.PaneKey != p.Pane || s.ProviderSession.ID != p.Provider || s.MainAgent.StateStartedAt <= p.WorkingAt || s.MainAgent.StateStartedAt <= p.BaseRootAt || p.WorkingAt == 0 && p.BaseRootAt <= 0 {
		return false
	}
	if s.SessionBoundary || s.RestoredUnconfirmed || s.ToolOutput || s.Prompt != preview(turn.Prompt) {
		p.Diagnostic = "Native original completion/input provenance is unverified"
		return false
	}
	if s.Interrupted || s.MainAgent.Outcome != "" && s.MainAgent.Outcome != "success" {
		p.Diagnostic = "Native interruption/outcome cannot verify ordinary completion"
		return false
	}
	if p.EndedAt != 0 && p.EndedAt != s.MainAgent.StateStartedAt {
		p.Diagnostic = "Native original completion record changed"
		return false
	}
	if p.EndedAt == 0 {
		p.EndedAt = s.MainAgent.StateStartedAt
		p.LastSignal = now
		p.Outcome = "unknown"
		if s.MainAgent.Outcome != "" {
			p.Outcome = s.MainAgent.Outcome
		}
	}
	if p.Diagnostic == "Native main agent is waiting; no next input was sent" {
		p.Diagnostic = ""
	}
	p.body(s.LastAssistantMessage)
	return true
}
func (p *NativeProof) query(s native.AgentState, turn *Turn, now time.Time) bool {
	if p.Diagnostic != "" && p.Diagnostic != "Native main agent is waiting; no next input was sent" {
		return false
	}
	if s.MainAgent == nil || s.UpdatedAt < s.MainAgent.StateStartedAt {
		return false
	}
	if s.ProviderSession.ID == "" {
		return false // Missing metadata is unavailable evidence, not replacement.
	}
	if s.PaneKey != p.Pane || s.ProviderSession.ID != p.Provider {
		p.Diagnostic = "Native original queried pane/provider changed"
		return false
	}
	if p.rejectProvenance(s, turn) {
		return false
	}
	return p.end(s, turn, now)
}
func (p NativeProof) wantsQuery(now time.Time) bool {
	return (p.EndedAt == 0 || p.ReplyDigest == "") && (p.HintPending || !now.Before(p.LastQuery.Add(30*time.Second)))
}

func (l *Lifecycle) stateFeed(ctx context.Context, process b.ProcessIdentity) (native.StateFeed, error) {
	l.feedMu.Lock()
	defer l.feedMu.Unlock()
	if l.feed != nil {
		if l.feedProcess != process {
			return nil, errors.New("native state feed original resource changed")
		}
		return l.feed, nil
	}
	feed, err := l.stateFactory(ctx, l.profile.Environment, process)
	if err != nil {
		return nil, err
	}
	l.feed, l.feedProcess = feed, process
	return feed, nil
}
func (l *Lifecycle) closeStateFeed() {
	l.feedMu.Lock()
	defer l.feedMu.Unlock()
	if l.feed != nil {
		l.feed.Close()
		l.feed = nil
	}
}

func (l *Lifecycle) releaseStateFeed(feed native.StateFeed) {
	if feed == nil {
		return
	}
	l.feedMu.Lock()
	if l.feed == feed {
		l.feed = nil
	}
	l.feedMu.Unlock()
	feed.Close()
}

func (l *Lifecycle) observeNative(ctx context.Context, snapshot b.LifecycleSnapshot, state *State, turn *Turn) string {
	// Subscription is a notification source, not a prerequisite for a bounded
	// authoritative read. Opening a failed stream is retried only on the query
	// interval, so an offline host cannot create a tight reconnect loop.
	l.feedMu.Lock()
	feed := l.feed
	l.feedMu.Unlock()
	var streamErr error
	if feed == nil && !l.clock().Before(l.feedRetryAt) {
		feed, streamErr = l.stateFeed(ctx, state.Process)
		if streamErr != nil {
			l.feedRetryAt = l.clock().Add(30 * time.Second)
			if errors.Is(streamErr, native.ErrStateIdentityChanged) {
				turn.Native.Diagnostic = streamErr.Error()
			}
		} else {
			// A reattached stream's initial snapshot can already be done. It
			// triggers confirmation, not direct completion or a fake working.
			initial := feed.Initial()
			if stateHint(initial) != turn.Native.Hint && initial.State == "done" {
				turn.Native.Hint, turn.Native.HintPending = stateHint(initial), true
			}
		}
	}
	drain := func() {
		if feed == nil {
			return
		}
		states, err := feed.Drain()
		streamErr = err
		for _, s := range states {
			turn.Native.signal(s, turn, l.clock())
		}
		if errors.Is(err, native.ErrStateIdentityChanged) {
			turn.Native.Diagnostic = err.Error()
		}
	}
	drain()
	if turn.Native.Diagnostic != "" && turn.Native.Diagnostic != "Native main agent is waiting; no next input was sent" {
		return turn.Native.Diagnostic
	}
	if turn.Native.wantsQuery(l.clock()) {
		turn.Native.LastQuery = l.clock()
		turn.Native.HintPending = false
		if !turn.Accepted || !turn.Started {
			return "Original input has no verified start receipt; no input was resent"
		}
		var s native.AgentState
		var err error
		if l.queryNative != nil {
			s, err = l.queryNative(ctx, state.Process, turn.Native.Pane)
		} else if feed != nil {
			s, err = feed.Query(ctx)
		} else {
			err = streamErr
			if err == nil {
				err = errors.New("native original query source unavailable")
			}
		}
		if err != nil {
			if errors.Is(err, native.ErrStateIdentityChanged) {
				turn.Native.Diagnostic = err.Error()
			}
			return "Native main-agent query is unverified: " + err.Error()
		}
		// Drain notifications received while the single synchronous query ran.
		drain()
		if turn.Native.Diagnostic != "" && turn.Native.Diagnostic != "Native main agent is waiting; no next input was sent" {
			return turn.Native.Diagnostic
		}
		if _, err := l.original(ctx, snapshot); err != nil {
			return err.Error()
		}
		turn.Native.query(s, turn, l.clock())
	}
	if turn.Native.EndedAt == 0 && streamErr != nil {
		return "Native state subscription is unverified; original query remains available: " + streamErr.Error()
	}
	return turn.Native.Diagnostic
}
