package console

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

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

func conversationItemKey(event api.ConversationEvent) string {
	return strings.Join([]string{event.ExecutionID, event.AttemptID, event.Source, event.SourceSegment, event.SessionID, event.TurnID, firstText(event.ItemKey, event.ItemID, event.SourceEventID)}, "\x00")
}

func (m Model) readFinalReplies(index int) tea.Cmd {
	state := &m.projects[index]
	state.finalBusy = true
	state.finalGeneration++
	state.finalReadAt = m.clock.Now()
	generation, key, filter := state.finalGeneration, state.finalKey, state.finalFilter
	ref, query, ctx := state.ref, state.client, m.ctx
	full := m.detailExpanded && m.detailSection == "final"
	if !full {
		if row := m.selectedRow(index); row != nil && row.run != nil {
			filter.AttemptID = row.run.AttemptID
			filter.TurnID = row.run.CurrentTurnID
			if row.run.StepID == "implement.report" {
				for i := len(row.run.StageResults) - 1; i >= 0; i-- {
					if row.run.StageResults[i].StepID == "implement.execute" {
						filter.TurnID = row.run.StageResults[i].TurnID
						break
					}
				}
			}
		}
	}
	return func() tea.Msg {
		var all api.ConversationPage
		cursor := ""
		for {
			page, err := query.Conversation(ctx, filter, cursor)
			if err != nil {
				return finalRepliesMsg{ref.Key, ref.Identity, key, generation, api.ConversationPage{}, err}
			}
			all.Events = append(all.Events, page.Events...)
			if !page.HasMore || !full {
				all.HasMore = page.HasMore
				return finalRepliesMsg{ref.Key, ref.Identity, key, generation, all, nil}
			}
			if page.NextCursor == "" || page.NextCursor == cursor {
				return finalRepliesMsg{ref.Key, ref.Identity, key, generation, api.ConversationPage{}, fmt.Errorf("最终回复索引游标未推进")}
			}
			cursor = page.NextCursor
		}
	}
}

func (m Model) updateFinalReplies(msg finalRepliesMsg) (tea.Model, tea.Cmd) {
	index := m.projectIndex(msg.projectKey)
	if index < 0 {
		return m, nil
	}
	state := &m.projects[index]
	if msg.identity != state.ref.Identity || msg.key != state.finalKey || msg.generation != state.finalGeneration {
		return m, nil
	}
	state.finalBusy = false
	if msg.err != nil {
		state.finalError = msg.err.Error()
		invalidateFinalPreview(state, state.finalError)
		return m, nil
	}
	state.finalError = ""
	var latest []api.ConversationEvent
	for _, event := range msg.page.Events {
		if event.Version != api.ConversationVersion || event.ExecutionID != state.finalFilter.ExecutionID || event.WorkflowID != state.finalFilter.WorkflowID || event.ProjectID != state.ref.Identity || state.finalFilter.SessionID != "" && event.SessionID != state.finalFilter.SessionID {
			state.finalError = "最终回复记录不属于原候选"
			invalidateFinalPreview(state, state.finalError)
			return m, nil
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
		invalidateFinalPreview(state, firstText(state.finalError, "本轮 final 尚未采集"))
		return m, nil
	}
	if !m.detailExpanded || m.detailSection != "final" {
		if msg.page.HasMore {
			invalidateFinalPreview(state, "final 索引尚未完整；f 按需读取完整记录")
			return m, nil
		}
		return m, m.readFinalPreview(index, state.finalEvents[len(state.finalEvents)-1])
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
		return m, nil
	}
	return m.selectFinal(index, event)
}

func (m Model) selectFinal(index int, event api.ConversationEvent) (tea.Model, tea.Cmd) {
	state := &m.projects[index]
	state.finalSelected, state.bodyRef = conversationItemKey(event), event.Body
	state.bodyText, state.bodyError, state.detailScroll = "", "", 0
	state.bodyGeneration++
	state.bodyBusy = false
	if event.Body == nil {
		state.bodyError = "最终回复正文未采集"
		return m, nil
	}
	body := *event.Body
	if body.ExecutionID != event.ExecutionID || body.AttemptID != event.AttemptID || body.Bytes < 0 || body.Bytes > 16<<20 {
		state.bodyError = "最终回复正文身份或大小无效"
		return m, nil
	}
	state.bodyBusy = true
	generation, key, ref, query, ctx := state.bodyGeneration, state.finalKey, state.ref, state.client, m.ctx
	return m, func() tea.Msg {
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
		return bodyMsg{ref.Key, ref.Identity, key, generation, body, completeText, err}
	}
}

func (m Model) updateBody(msg bodyMsg) (tea.Model, tea.Cmd) {
	index := m.projectIndex(msg.projectKey)
	if index < 0 {
		return m, nil
	}
	state := &m.projects[index]
	if msg.identity != state.ref.Identity || msg.key != state.finalKey || msg.generation != state.bodyGeneration || state.bodyRef == nil || *state.bodyRef != msg.body {
		return m, nil
	}
	state.bodyBusy = false
	if msg.err != nil {
		state.bodyError = msg.err.Error()
		return m, nil
	}
	state.bodyText, state.bodyError = msg.text, ""
	return m, nil
}

func validBodyPage(ref api.ConversationBodyRef, offset int64, page api.ConversationBodyPage) bool {
	return page.ItemKey == ref.ItemKey && page.Revision == ref.Revision && page.Offset == offset && page.TotalBytes == ref.Bytes && page.SHA256 == ref.SHA256 && page.Completeness == ref.Completeness && utf8.Valid(page.Data) && page.NextOffset == offset+int64(len(page.Data)) && page.NextOffset <= ref.Bytes && page.Complete == (page.NextOffset == ref.Bytes)
}

func (m Model) cycleFinal(delta int) (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	if len(state.finalEvents) == 0 {
		m.status = "最终回复尚未采集或来源未识别"
		return m, nil
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
	return m.selectFinal(m.selected, state.finalEvents[selected])
}

func finalReplyLines(state projectState) []string {
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
