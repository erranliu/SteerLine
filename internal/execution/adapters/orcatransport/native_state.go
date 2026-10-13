package orcatransport

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
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
	ExpectedReply string    `json:"expected_reply,omitempty"`
	ReplyBody     string    `json:"reply_body,omitempty"`
	LastSignal    time.Time `json:"last_signal,omitempty"`
	Queried       bool      `json:"queried,omitempty"`
	Hint          string    `json:"hint,omitempty"`
	HintPending   bool      `json:"hint_pending,omitempty"`
	Diagnostic    string    `json:"diagnostic,omitempty"`
}

func stateHint(s native.AgentState) string {
	return s.PaneKey + ":" + s.State + ":" + strconv.FormatInt(s.StateStartedAt, 10) + ":" + strconv.FormatInt(s.UpdatedAt, 10)
}
func nativeBaseline(s native.AgentState, now time.Time) NativeProof {
	p := NativeProof{Pane: s.PaneKey, Provider: s.ProviderSession.ID, LastSignal: now, Hint: stateHint(s)}
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

var renderedInlineLink = regexp.MustCompile(`\[([^\]\r\n]+)\]\(([^()\s]+)\)`)
var renderedInlineCode = regexp.MustCompile("`([^`\\r\\n]+)`")
var renderedInlineBold = regexp.MustCompile(`\*\*([^*\r\n]+)\*\*`)

func replyAssociation(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json\n") && strings.HasSuffix(text, "\n```") {
		text = strings.TrimSuffix(strings.TrimPrefix(text, "```json\n"), "\n```")
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	compact := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, s)
	}
	var fold func(any) any
	fold = func(v any) any {
		switch v := v.(type) {
		case string:
			return compact(v)
		case []any:
			for i := range v {
				v[i] = fold(v[i])
			}
		case map[string]any:
			for k := range v {
				v[k] = fold(v[k])
			}
		}
		return v
	}
	if json.Valid([]byte(text)) && decoder.Decode(&value) == nil {
		data, _ := json.Marshal(fold(value))
		return digest(data)
	}
	// Codex renders these inline Markdown forms without their delimiters or
	// link targets. This is comparison only; persist the native original body.
	text = renderedInlineLink.ReplaceAllString(text, "$1")
	text = renderedInlineCode.ReplaceAllString(text, "$1")
	text = renderedInlineBold.ReplaceAllString(text, "$1")
	return digest([]byte(compact(text)))
}
func (p NativeProof) matchesVisible(text string) bool {
	if replyAssociation(text) == p.ExpectedReply {
		return true
	}
	// Codex shows non-hyperlinked targets as "label (target)". Compare that
	// rendering of this exact native body too, without dropping visible words.
	rendered := renderedInlineLink.ReplaceAllString(p.ReplyBody, "$1 ($2)")
	return p.ReplyBody != "" && replyAssociation(text) == replyAssociation(rendered)
}
func (p *NativeProof) body(message string) {
	// The public Codex hook field is capped at 8000 UTF-16 code units. Reject
	// the surrogate-safe truncation boundary too; never treat a prefix as full.
	if strings.TrimSpace(message) == "" || len(utf16.Encode([]rune(message))) >= 7999 {
		return
	}
	body := strings.TrimSpace(message)
	if strings.HasPrefix(body, "```json\n") && strings.HasSuffix(body, "\n```") {
		body = strings.TrimSuffix(strings.TrimPrefix(body, "```json\n"), "\n```")
	}
	hash := replyAssociation(body)
	// Association ignores display wrapping, but the native original is immutable.
	if p.ExpectedReply != "" && (p.ExpectedReply != hash || p.ReplyBody != body) {
		p.Diagnostic = "Native ended reply changed; current result is unverified"
		return
	}
	p.ExpectedReply = hash
	p.ReplyBody = body
}
func (p *NativeProof) signal(s native.AgentState, turn *Turn, now time.Time) {
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
			p.Queried = false
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
	if p.WorkingAt == 0 {
		if s.MainAgent.State != "working" || !turn.Started || s.ProviderSession.ID == "" || s.Prompt != preview(turn.Prompt) {
			return
		}
		p.Authority, p.Incarnation, p.Provider = o.AuthorityID, o.Incarnation, s.ProviderSession.ID
		p.WorkingAt = o.ObservedAt
		p.RootWorkingAt = s.MainAgent.StateStartedAt
	} else if o.Boundary && s.MainAgent.State == "working" && s.MainAgent.StateStartedAt != p.RootWorkingAt {
		p.Diagnostic = "Another native input boundary makes the original turn unverified"
		return
	}
	p.Revision, p.LastSignal, p.Queried = o.Revision, now, false
	if s.MainAgent.State == "done" && s.MainAgent.StateStartedAt > p.WorkingAt {
		p.end(s, now)
	} else if s.MainAgent.State == "waiting" || s.MainAgent.State == "blocked" {
		// Waiting remains in the current turn; it is not a completion.
		p.Diagnostic = "Native main agent is waiting; no next input was sent"
	} else if p.Diagnostic == "Native main agent is waiting; no next input was sent" {
		p.Diagnostic = ""
	}
}
func (p *NativeProof) end(s native.AgentState, now time.Time) bool {
	if p.WorkingAt == 0 || s.MainAgent == nil || s.MainAgent.State != "done" || s.AgentType != "codex" || s.PaneKey != p.Pane || s.ProviderSession.ID != p.Provider || s.MainAgent.StateStartedAt <= p.WorkingAt || s.MainAgent.StateStartedAt <= p.BaseRootAt {
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
		p.Queried = false
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
func (p *NativeProof) query(s native.AgentState, now time.Time) bool {
	if p.Diagnostic != "" && p.Diagnostic != "Native main agent is waiting; no next input was sent" {
		return false
	}
	if s.MainAgent == nil || s.UpdatedAt < s.MainAgent.StateStartedAt {
		return false
	}
	return p.end(s, now)
}
func (p NativeProof) wantsQuery(now time.Time) bool {
	return p.WorkingAt > 0 && (p.EndedAt == 0 || p.ExpectedReply == "") && !p.Queried && (p.HintPending || !now.Before(p.LastSignal.Add(30*time.Second)))
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
	feed, err := l.stateFeed(ctx, state.Process)
	if err != nil {
		return "Native state subscription is unverified: " + err.Error()
	}
	states, err := feed.Drain()
	if err != nil {
		return "Native state subscription is unverified: " + err.Error()
	}
	for _, s := range states {
		turn.Native.signal(s, turn, l.clock())
	}
	if turn.Native.Diagnostic != "" && turn.Native.Diagnostic != "Native main agent is waiting; no next input was sent" {
		return turn.Native.Diagnostic
	}
	if turn.Native.wantsQuery(l.clock()) {
		turn.Native.Queried = true
		turn.Native.HintPending = false
		s, err := feed.Query(ctx)
		if err != nil {
			return "Native main-agent query is unverified: " + err.Error()
		}
		// Drain notifications received while the single synchronous query ran.
		states, err = feed.Drain()
		if err != nil {
			return "Native state subscription is unverified: " + err.Error()
		}
		for _, update := range states {
			turn.Native.signal(update, turn, l.clock())
		}
		if _, err := l.original(ctx, snapshot); err != nil {
			return err.Error()
		}
		turn.Native.query(s, l.clock())
	}
	return turn.Native.Diagnostic
}
