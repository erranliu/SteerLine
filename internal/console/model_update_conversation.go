package console

import (
	api "github.com/erranliu/SteerLine/internal/product/api"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateConversation(msg conversationMsg) (tea.Model, tea.Cmd) {
	index := m.projectIndex(msg.projectKey)
	if index < 0 {
		return m, nil
	}
	state := &m.projects[index]
	if index != m.selected || msg.identity != state.ref.Identity || msg.generation != state.conversationGeneration || msg.key != state.conversationKey {
		return m, nil
	}
	state.conversationInFlight = false
	if msg.err != nil {
		state.conversationError = msg.err.Error()
		return m, nil
	}
	if msg.mode == "head" {
		state.conversationEvents = append([]api.ConversationEvent(nil), msg.page.Events...)
	} else {
		seen := map[int64]bool{}
		for _, event := range state.conversationEvents {
			if event.EventID > 0 {
				seen[event.EventID] = true
			}
		}
		for _, event := range msg.page.Events {
			if event.EventID == 0 || !seen[event.EventID] {
				state.conversationEvents = append(state.conversationEvents, event)
				if event.EventID > 0 {
					seen[event.EventID] = true
				}
			}
		}
	}
	sort.SliceStable(state.conversationEvents, func(i, j int) bool { return state.conversationEvents[i].EventID < state.conversationEvents[j].EventID })
	switch msg.mode {
	case "head":
		state.conversationMore = msg.page.HasMore
		state.conversationCursor = msg.page.NextCursor
		state.conversationTailMore = false
	case "page":
		state.conversationMore = msg.page.HasMore
		state.conversationCursor = msg.page.NextCursor
		if !msg.page.HasMore {
			state.conversationTailMore = false
		}
	case "tail":
		state.conversationTailMore = msg.page.HasMore
		if state.conversationCursor == "" && msg.page.HasMore {
			state.conversationMore = true
			state.conversationCursor = msg.page.NextCursor
		}
	}
	if len(state.conversationEvents) > 0 {
		latest := state.conversationEvents[len(state.conversationEvents)-1].EventID
		if cursor, err := state.client.CursorAfter(state.conversationFilter, latest); err == nil {
			state.conversationTailCursor = cursor
		}
	}
	state.conversationError = ""
	state.capabilities = nil
	conversationView, changed := BuildConversationView(state.conversationEvents, state.view.Groups)
	if state.view.AnchorGroup == "" && len(conversationView.Groups) > 0 {
		state.view.AnchorGroup = conversationView.Groups[len(conversationView.Groups)-1].ID
		changed = true
	}
	if changed {
		m.saveProjectView(index)
	}
	if state.panel == 2 && state.view.ReadGroup != "" {
		oldScroll := state.detailScroll
		hidden := restoreSessionAnchor(state, m.selectedRow(index), activityPanelWidth(m))
		if hidden {
			m.status = "会话阅读锚点已收起；已定位到轮次标题"
		}
		if state.detailScroll != oldScroll {
			m.saveProjectView(index)
		}
	}
	var cmds []tea.Cmd
	if len(state.conversationEvents) > 0 {
		cmds = append(cmds, m.readCapabilities(index, state.conversationEvents, state.conversationKey))
	}
	if state.bodyRestorePending {
		if event, ok := findBodyEvent(state.conversationEvents, state.view.AnchorItem, state.view.AnchorRevision); ok {
			state.bodyRef, state.bodyEvent = event.Body, conversationItemKey(event)
			state.bodyViewOffset = state.view.BodyOffset
			cmds = append(cmds, m.readBody(index, event, state.bodyViewOffset))
		} else if !msg.page.HasMore {
			state.bodyError = "先前阅读的正文版本未出现在本页；继续加载会话记录"
			state.bodyRestorePending = false
		}
	}
	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m Model) updateCapability(msg capabilityMsg) (tea.Model, tea.Cmd) {
	index := m.projectIndex(msg.projectKey)
	if index < 0 {
		return m, nil
	}
	state := &m.projects[index]
	if msg.generation != state.capabilityGeneration {
		return m, nil
	}
	state.capabilityInFlight = false
	if index != m.selected || msg.identity != state.ref.Identity || msg.key != state.conversationKey {
		return m, nil
	}
	state.capabilities = msg.reports
	if msg.err != nil {
		state.capabilityError = msg.err.Error()
	} else {
		state.capabilityError = ""
	}
	return m, nil
}

func (m Model) updateBody(msg bodyMsg) (tea.Model, tea.Cmd) {
	index := m.projectIndex(msg.projectKey)
	if index < 0 {
		return m, nil
	}
	state := &m.projects[index]
	if msg.generation != state.bodyGeneration {
		return m, nil
	}
	state.bodyBusy = false
	if msg.identity != state.ref.Identity || state.bodyEvent != msg.eventKey || state.bodyRef == nil || *state.bodyRef != msg.body {
		return m, nil
	}
	if msg.err != nil {
		state.bodyError = msg.err.Error()
		return m, nil
	}
	if !validBodyPage(msg.body, msg.offset, msg.page) {
		state.bodyError = "正文响应与请求的 item/revision/offset 或完整性不匹配"
		return m, nil
	}
	state.bodyOffset = msg.page.NextOffset
	state.bodyViewOffset = msg.page.Offset
	state.bodyPages = appendBodyPage(state.bodyPages, msg.page.Offset)
	state.bodyText = string(msg.page.Data)
	state.bodyMore = !msg.page.Complete
	state.bodyError = ""
	state.bodyRestorePending = false
	m.saveProjectView(index)
	return m, nil
}
