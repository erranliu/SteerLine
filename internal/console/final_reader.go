package console

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

// finalReplyReader owns all final index/body/preview state. Commands capture
// fixed requests and the query port; navigation and controls remain in Model.
type finalReplyReader struct {
	finalKey          string
	finalFilter       api.ConversationFilter
	finalEvents       []api.ConversationEvent
	finalError        string
	finalBusy         bool
	finalReadAt       time.Time
	finalGeneration   uint64
	finalSelected     string
	finalPinned       bool
	bodyRef           *api.ConversationBodyRef
	bodyText          string
	bodyError         string
	bodyBusy          bool
	bodyGeneration    uint64
	previewEvent      *api.ConversationEvent
	previewText       string
	previewError      string
	previewGeneration uint64
	previewBusy       bool
	previewReady      bool
}

type finalReplyRoute struct{ projectKey, identity string }
type finalPreviewTarget struct{ key, attempt, turn string }

// bind reports a candidate change so the host can reset its scroll/recovery.
func (s *finalReplyReader) bind(key string) bool {
	if key == s.finalKey {
		return false
	}
	*s = finalReplyReader{finalKey: key, finalFilter: s.finalFilter,
		finalGeneration: s.finalGeneration + 1, bodyGeneration: s.bodyGeneration + 1,
		previewGeneration: s.previewGeneration + 1}
	return true
}

func (s *finalReplyReader) refresh(ctx context.Context, query ConversationQueries, now time.Time, route finalReplyRoute, key string, filter api.ConversationFilter, full bool, attempt, turn string) (bool, tea.Cmd) {
	changed := s.bind(key)
	if key == "" || s.finalBusy || !s.finalReadAt.IsZero() && now.Sub(s.finalReadAt) < 5*time.Second {
		return changed, nil
	}
	if filter.ExecutionID == "" {
		s.finalError = "当前候选缺少 execution_id，无法核验最终回复"
		return changed, nil
	}
	s.finalFilter = filter
	return changed, s.read(ctx, query, now, route, full, attempt, turn, false)
}

// retry schedules an index read on the next project refresh.
func (s *finalReplyReader) retry() { s.finalReadAt = time.Time{} }

type finalRepliesMsg struct {
	projectKey, identity, key string
	generation                uint64
	page                      api.ConversationPage
	err                       error
}

type bodyMsg struct {
	projectKey, identity, key string
	generation                uint64
	body                      api.ConversationBodyRef
	text                      string
	err                       error
}

type finalPreviewMsg struct {
	projectKey, identity, key string
	generation                uint64
	event                     api.ConversationEvent
	text                      string
	err                       error
}

func conversationItemKey(event api.ConversationEvent) string {
	return strings.Join([]string{event.ExecutionID, event.AttemptID, event.Source, event.SourceSegment, event.SessionID, event.TurnID, firstText(event.ItemKey, event.ItemID, event.SourceEventID)}, "\x00")
}

func (state *finalReplyReader) read(ctx context.Context, query ConversationQueries, now time.Time, route finalReplyRoute, full bool, attempt, turn string, resetPin bool) tea.Cmd {
	if resetPin {
		state.finalPinned = false
	}
	state.finalBusy = true
	state.finalGeneration++
	state.finalReadAt = now
	generation, key, filter := state.finalGeneration, state.finalKey, state.finalFilter
	if !full {
		filter.AttemptID, filter.TurnID = attempt, turn
	}
	return func() tea.Msg {
		var all api.ConversationPage
		cursor := ""
		for {
			page, err := query.Conversation(ctx, filter, cursor)
			if err != nil {
				return finalRepliesMsg{route.projectKey, route.identity, key, generation, api.ConversationPage{}, err}
			}
			all.Events = append(all.Events, page.Events...)
			if !page.HasMore || !full {
				all.HasMore = page.HasMore
				return finalRepliesMsg{route.projectKey, route.identity, key, generation, all, nil}
			}
			if page.NextCursor == "" || page.NextCursor == cursor {
				return finalRepliesMsg{route.projectKey, route.identity, key, generation, api.ConversationPage{}, fmt.Errorf("最终回复索引游标未推进")}
			}
			cursor = page.NextCursor
		}
	}
}

func (state *finalReplyReader) acceptReplies(ctx context.Context, query ConversationQueries, route finalReplyRoute, full bool, msg finalRepliesMsg) (tea.Cmd, bool) {
	if msg.identity != route.identity || msg.key != state.finalKey || msg.generation != state.finalGeneration {
		return nil, false
	}
	state.finalBusy = false
	if msg.err != nil {
		state.finalError = msg.err.Error()
		state.invalidatePreview(state.finalError)
		return nil, false
	}
	state.finalError = ""
	var latest []api.ConversationEvent
	for _, event := range msg.page.Events {
		if event.Version != api.ConversationVersion || event.ExecutionID != state.finalFilter.ExecutionID || event.WorkflowID != state.finalFilter.WorkflowID || event.ProjectID != route.identity || state.finalFilter.SessionID != "" && event.SessionID != state.finalFilter.SessionID {
			state.finalError = "最终回复记录不属于原候选"
			state.invalidatePreview(state.finalError)
			return nil, false
		}
		if event.Type == api.ConversationGap {
			state.finalError = firstText(event.Diagnostic, "规范记录存在采集缺口")
			continue
		}
		if event.Phase != api.MessagePhaseFinalAnswer || event.TurnID == "" || (event.TurnQuality != api.IdentityNative && event.TurnQuality != api.IdentityDerived) || event.Type != api.ConversationMessageComplete {
			continue
		}
		key := conversationItemKey(event)
		found := false
		for i, old := range latest {
			if conversationItemKey(old) == key {
				found = true
				if event.SourceRevision > old.SourceRevision || event.SourceRevision == old.SourceRevision && event.EventID >= old.EventID {
					latest[i] = event
				}
				break
			}
		}
		if !found {
			for _, old := range state.finalEvents {
				if conversationItemKey(old) == key && old.SourceRevision > event.SourceRevision {
					event = old
					break
				}
			}
			latest = append(latest, event)
		}
	}
	state.finalEvents = latest
	if len(state.finalEvents) == 0 {
		state.invalidatePreview(firstText(state.finalError, "本轮 final 尚未采集"))
		return nil, false
	}
	if !full {
		if msg.page.HasMore {
			state.invalidatePreview("final 索引尚未完整；f 按需读取完整记录")
			return nil, false
		}
		return state.readPreview(ctx, query, route, state.finalEvents[len(state.finalEvents)-1]), false
	}
	selected := len(state.finalEvents) - 1
	if state.finalPinned {
		for i, event := range state.finalEvents {
			if conversationItemKey(event) == state.finalSelected {
				selected = i
				break
			}
		}
	}
	event := state.finalEvents[selected]
	if state.finalSelected == conversationItemKey(event) && state.bodyRef != nil && event.Body != nil && *state.bodyRef == *event.Body && state.bodyError == "" {
		return nil, false
	}
	return state.selectBody(ctx, query, route, event), true
}

func (state *finalReplyReader) selectBody(ctx context.Context, query ConversationQueries, route finalReplyRoute, event api.ConversationEvent) tea.Cmd {
	state.finalSelected, state.bodyRef = conversationItemKey(event), event.Body
	state.bodyText, state.bodyError = "", ""
	state.bodyGeneration++
	state.bodyBusy = false
	if event.Body == nil {
		state.bodyError = "最终回复正文未采集"
		return nil
	}
	body := *event.Body
	if body.ExecutionID != event.ExecutionID || body.AttemptID != event.AttemptID || body.Bytes < 0 || body.Bytes > 16<<20 {
		state.bodyError = "最终回复正文身份或大小无效"
		return nil
	}
	state.bodyBusy = true
	generation, key := state.bodyGeneration, state.finalKey
	return func() tea.Msg {
		var text strings.Builder
		var err error
		for offset := int64(0); ; {
			page, readErr := query.Body(ctx, body, offset, 64<<10)
			if readErr != nil {
				err = readErr
				break
			}
			if !validBodyPage(body, offset, page) || !page.Complete && page.NextOffset <= offset {
				err = fmt.Errorf("最终回复正文响应与原版本/偏移不匹配")
				break
			}
			text.Write(page.Data)
			if page.Complete {
				break
			}
			offset = page.NextOffset
		}
		completeText := text.String()
		if err == nil {
			digest := sha256.Sum256([]byte(completeText))
			if hex.EncodeToString(digest[:]) != body.SHA256 {
				err = fmt.Errorf("完整最终回复与原正文摘要不匹配")
			}
		}
		return bodyMsg{route.projectKey, route.identity, key, generation, body, completeText, err}
	}
}

func (state *finalReplyReader) acceptBody(identity string, msg bodyMsg) {
	if msg.identity != identity || msg.key != state.finalKey || msg.generation != state.bodyGeneration || state.bodyRef == nil || *state.bodyRef != msg.body {
		return
	}
	state.bodyBusy = false
	if msg.err != nil {
		state.bodyError = msg.err.Error()
		return
	}
	state.bodyText, state.bodyError = msg.text, ""
}

func validBodyPage(ref api.ConversationBodyRef, offset int64, page api.ConversationBodyPage) bool {
	return page.ItemKey == ref.ItemKey && page.Revision == ref.Revision && page.Offset == offset && page.TotalBytes == ref.Bytes && page.SHA256 == ref.SHA256 && page.Completeness == ref.Completeness && utf8.Valid(page.Data) && page.NextOffset == offset+int64(len(page.Data)) && page.NextOffset <= ref.Bytes && page.Complete == (page.NextOffset == ref.Bytes)
}

func (state *finalReplyReader) cycle(ctx context.Context, query ConversationQueries, route finalReplyRoute, delta int) (tea.Cmd, bool) {
	if len(state.finalEvents) == 0 {
		return nil, false
	}
	selected := len(state.finalEvents) - 1
	for i, event := range state.finalEvents {
		if conversationItemKey(event) == state.finalSelected {
			selected = i
			break
		}
	}
	selected = (selected + delta + len(state.finalEvents)) % len(state.finalEvents)
	state.finalPinned = true
	return state.selectBody(ctx, query, route, state.finalEvents[selected]), true
}

func finalReplyLines(state finalReplyReader) []string {
	lines := []string{"最终回复 · [ / ] 选择历史业务轮次"}
	if state.finalError != "" {
		lines = append(lines, "采集缺口/读取错误："+safeTerminalText(state.finalError))
	}
	if len(state.finalEvents) == 0 {
		return append(lines, "最终回复尚未采集或来源未识别；摘要不是完整正文。")
	}
	for i, event := range state.finalEvents {
		if conversationItemKey(event) == state.finalSelected {
			lines = append(lines, fmt.Sprintf("回复 %d/%d · turn %s", i+1, len(state.finalEvents), safeTerminalText(event.TurnID)))
			break
		}
	}
	if state.bodyError != "" {
		lines = append(lines, "正文读取："+safeTerminalText(state.bodyError))
	}
	if state.bodyBusy {
		lines = append(lines, "正在有界读取完整正文…")
	}
	if state.bodyRef != nil && state.bodyRef.Completeness != api.BodyComplete {
		lines = append(lines, "正文完整性："+safeTerminalText(string(state.bodyRef.Completeness)))
	}
	return append(lines, strings.Split(safeTerminalText(state.bodyText), "\n")...)
}

func previewPrefix(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) > 6 {
		lines = lines[:6]
	}
	runes := []rune(strings.Join(lines, "\n"))
	if len(runes) > 800 {
		runes = runes[:800]
	}
	return string(runes)
}

func (s *finalReplyReader) readPreview(ctx context.Context, query ConversationQueries, route finalReplyRoute, event api.ConversationEvent) tea.Cmd {
	if s.previewEvent != nil && conversationItemKey(*s.previewEvent) == conversationItemKey(event) && s.previewEvent.SourceRevision == event.SourceRevision && s.previewEvent.Body != nil && event.Body != nil && *s.previewEvent.Body == *event.Body && s.previewError == "" && (s.previewBusy || s.previewReady) {
		return nil
	}
	s.previewGeneration++
	s.previewText = ""
	s.previewError = ""
	s.previewBusy, s.previewReady = false, false
	s.previewEvent = &event
	body := event.Body
	if body == nil || body.ExecutionID != event.ExecutionID || body.AttemptID != event.AttemptID || body.Bytes < 0 || body.Bytes > 16<<20 || body.Completeness != api.BodyComplete || event.Type != api.ConversationMessageComplete {
		s.previewError = "final 正文或轮次尚未完整采集"
		return nil
	}
	generation, key := s.previewGeneration, s.finalKey
	fixedBody := *body
	event.Body = &fixedBody
	s.previewBusy = true
	return func() tea.Msg {
		page, err := query.Body(ctx, fixedBody, 0, 4096)
		if err == nil && (!validBodyPage(fixedBody, 0, page) || len(page.Data) > 4096) {
			err = fmt.Errorf("final 预览身份/版本/大小不匹配")
		}
		if err == nil && page.Complete {
			digest := sha256.Sum256([]byte(page.Data))
			if hex.EncodeToString(digest[:]) != fixedBody.SHA256 {
				err = fmt.Errorf("final 正文摘要不匹配")
			}
		}
		return finalPreviewMsg{route.projectKey, route.identity, key, generation, event, previewPrefix(string(page.Data)), err}
	}
}

func (s *finalReplyReader) acceptPreview(identity string, target finalPreviewTarget, msg finalPreviewMsg) {
	if msg.identity != identity || msg.key != s.finalKey || msg.generation != s.previewGeneration {
		return
	}
	s.previewBusy = false
	if target.key != s.finalKey || target.attempt != msg.event.AttemptID {
		s.invalidatePreview("final 预览目标/Attempt已变化")
		return
	}
	if target.turn != "" && target.turn != msg.event.TurnID {
		s.invalidatePreview("final 预览轮次已变化")
		return
	}
	if msg.err != nil {
		s.previewError = msg.err.Error()
		s.previewText = ""
	} else {
		s.previewText = msg.text
		s.previewError = ""
		s.previewReady = true
	}
}

func (s *finalReplyReader) invalidatePreview(reason string) {
	s.previewGeneration++
	s.previewEvent = nil
	s.previewText = ""
	s.previewError = reason
	s.previewBusy, s.previewReady = false, false
}
