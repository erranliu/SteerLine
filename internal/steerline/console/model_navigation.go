package console

import (
	"sort"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func (m Model) cycleAttempt(next bool) (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	row := m.selectedRow(m.selected)
	if row == nil || row.run == nil || state.snapshot.Domain == nil {
		m.status = "没有可切换的执行尝试"
		return m, nil
	}
	attempts := make([]contracts.StepAttemptRecord, 0)
	seenAttempts := map[string]bool{}
	maxAttemptNumber := 0
	for _, attempt := range state.snapshot.Domain.Attempts {
		if attempt.ExecutionID == row.run.ExecutionID && !seenAttempts[attempt.AttemptID] {
			attempts = append(attempts, attempt)
			seenAttempts[attempt.AttemptID] = true
			if attempt.AttemptNumber > maxAttemptNumber {
				maxAttemptNumber = attempt.AttemptNumber
			}
		}
	}
	if row.run.AttemptID != "" {
		found := false
		for _, attempt := range attempts {
			if attempt.AttemptID == row.run.AttemptID {
				found = true
			}
		}
		if !found {
			created := time.Time{}
			if row.run.StartedAt != nil {
				created = *row.run.StartedAt
			}
			attempts = append(attempts, contracts.StepAttemptRecord{ExecutionID: row.run.ExecutionID, AttemptID: row.run.AttemptID, AttemptNumber: maxAttemptNumber + 1, CreatedAt: created})
		}
	}
	sort.SliceStable(attempts, func(i, j int) bool {
		if attempts[i].CreatedAt.Equal(attempts[j].CreatedAt) {
			return attempts[i].AttemptNumber < attempts[j].AttemptNumber
		}
		if !attempts[i].CreatedAt.IsZero() && !attempts[j].CreatedAt.IsZero() {
			return attempts[i].CreatedAt.Before(attempts[j].CreatedAt)
		}
		return attempts[i].AttemptNumber < attempts[j].AttemptNumber
	})
	if len(attempts) < 2 {
		m.status = "此 execution 只有一个已记录 attempt"
		return m, nil
	}
	selectedAttemptID := state.attemptID
	if selectedAttemptID == "" {
		selectedAttemptID = row.run.AttemptID
	}
	current := 0
	for i, attempt := range attempts {
		if attempt.AttemptID == selectedAttemptID {
			current = i
			break
		}
	}
	if next {
		current = (current + 1) % len(attempts)
	} else {
		current = (current + len(attempts) - 1) % len(attempts)
	}
	state.attemptID = attempts[current].AttemptID
	clearReadingAnchor(state)
	m.saveProjectView(m.selected)
	var cmd tea.Cmd
	m, cmd = m.refreshSelectedLogs(m.selected)
	return m, cmd
}

func (m Model) toggleGroup() (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	view, _ := BuildConversationView(state.conversationEvents, state.view.Groups)
	selected := -1
	for i, group := range view.Groups {
		if group.ID == state.view.AnchorGroup {
			selected = i
			break
		}
	}
	if selected < 0 && len(view.Groups) > 0 {
		selected = len(view.Groups) - 1
	}
	if selected >= 0 {
		group := view.Groups[selected]
		if !group.Precise {
			m.status = "turn 能力未验证；保留当前执行过程视图，不按猜测折叠"
			return m, nil
		}
		ToggleFold(state.view.Groups, group)
		state.view.AnchorGroup = group.ID
		if Folded(group, state.view.Groups[group.ID]) && state.bodyRef != nil && turnGroupIDForBody(state.conversationEvents, state.bodyEvent) == group.ID {
			state.panel = 2
			state.detailScroll = 0
			m.status = "当前阅读正文所在轮次已收起；已定位到轮次标题"
		}
		if state.panel == 2 && state.view.ReadGroup != "" && restoreSessionAnchor(state, m.selectedRow(m.selected), activityPanelWidth(m)) {
			m.status = "会话阅读锚点已收起；已定位到轮次标题"
		}
		m.saveProjectView(m.selected)
		return m, nil
	}
	m.status = "当前会话没有稳定轮次组可折叠"
	return m, nil
}

func turnGroupIDForBody(events []contracts.ConversationEvent, bodyKey string) string {
	for _, event := range events {
		if event.Body != nil && conversationItemKey(event) == bodyKey {
			return turnGroupID(event)
		}
	}
	return ""
}

func (m Model) cycleGroup(delta int) (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	view, _ := BuildConversationView(state.conversationEvents, state.view.Groups)
	if len(view.Groups) == 0 {
		m.status = "当前会话没有轮次组"
		return m, nil
	}
	index := -1
	for i, group := range view.Groups {
		if group.ID == state.view.AnchorGroup {
			index = i
			break
		}
	}
	if index < 0 {
		if delta < 0 {
			index = 0
		} else {
			index = len(view.Groups) - 1
		}
	}
	index = (index + delta) % len(view.Groups)
	if index < 0 {
		index += len(view.Groups)
	}
	state.view.AnchorGroup = view.Groups[index].ID
	m.saveProjectView(m.selected)
	return m, nil
}

func (m Model) keepGroupOpen() (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	view, _ := BuildConversationView(state.conversationEvents, state.view.Groups)
	for _, group := range view.Groups {
		if group.Precise && (state.view.AnchorGroup == "" || state.view.AnchorGroup == group.ID) {
			SetKeepExpanded(state.view.Groups, group, true)
			state.view.AnchorGroup = group.ID
			m.saveProjectView(m.selected)
			return m, nil
		}
	}
	m.status = "当前没有可保持展开的轮次"
	return m, nil
}

func (m Model) openProcessBody() (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	view, _ := BuildConversationView(state.conversationEvents, state.view.Groups)
	var events []contracts.ConversationEvent
	for _, group := range view.Groups {
		if Folded(group, state.view.Groups[group.ID]) {
			continue
		}
		for _, event := range group.Process {
			if event.Body != nil {
				events = append(events, event)
			}
		}
	}
	if len(events) == 0 {
		m.status = "没有可分页读取的展开过程正文"
		return m, nil
	}
	selected := 0
	for i, event := range events {
		if conversationItemKey(event) == state.bodyEvent {
			selected = (i + 1) % len(events)
			break
		}
	}
	event := events[selected]
	state.bodyGeneration++
	state.bodyBusy = false
	state.panel = 3
	state.bodyRef = event.Body
	state.bodyText = ""
	state.bodyOffset = 0
	state.bodyViewOffset = 0
	state.bodyPages = []int64{0}
	state.bodyError = ""
	state.bodyEvent = conversationItemKey(event)
	state.bodyKind = "process"
	state.view.AnchorItem = event.Body.ItemKey
	state.view.AnchorRevision = event.Body.Revision
	state.view.AnchorGroup = turnGroupID(event)
	state.view.BodyOffset = 0
	state.view.BodyKind = "process"
	m.saveProjectView(m.selected)
	return m, m.readBody(m.selected, event, 0)
}

func (m Model) nextFinal() (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	view, _ := BuildConversationView(state.conversationEvents, state.view.Groups)
	if len(view.Groups) == 0 {
		m.status = "没有最终回复"
		return m, nil
	}
	var finals []contracts.ConversationEvent
	for _, group := range view.Groups {
		finals = append(finals, group.Finals...)
	}
	if len(finals) == 0 {
		m.status = "本轮最终回复尚未同步或来源未识别"
		return m, nil
	}
	if m.finalIndex >= len(finals) {
		m.finalIndex = 0
	}
	event := finals[m.finalIndex]
	m.finalIndex = (m.finalIndex + 1) % len(finals)
	state.bodyGeneration++
	state.bodyBusy = false
	state.panel = 3
	state.bodyRef = event.Body
	state.bodyText = ""
	state.bodyOffset = 0
	state.bodyViewOffset = 0
	state.bodyPages = []int64{0}
	state.bodyError = ""
	state.bodyEvent = conversationItemKey(event)
	state.bodyKind = "final"
	if event.Body != nil {
		state.view.AnchorItem = event.Body.ItemKey
		state.view.AnchorRevision = event.Body.Revision
		state.view.AnchorGroup = turnGroupID(event)
		state.view.BodyOffset = 0
		state.view.BodyKind = "final"
	}
	m.saveProjectView(m.selected)
	if event.Body == nil {
		state.bodyError = "最终回复正文缺失"
		return m, nil
	}
	return m, m.readBody(m.selected, event, 0)
}

func (m Model) pageBody(next bool) (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	if state.bodyRef == nil {
		m.status = "先在会话页选择最终回复"
		return m, nil
	}
	if state.bodyBusy {
		m.status = "正文页仍在读取"
		return m, nil
	}
	pageIndex := -1
	for i, offset := range state.bodyPages {
		if offset == state.bodyViewOffset {
			pageIndex = i
			break
		}
	}
	if pageIndex < 0 {
		m.status = "正文分页锚点无效；请重新打开该消息"
		return m, nil
	}
	if next {
		nextOffset := int64(-1)
		if pageIndex+1 < len(state.bodyPages) {
			nextOffset = state.bodyPages[pageIndex+1]
		} else if state.bodyOffset < state.bodyRef.Bytes {
			nextOffset = state.bodyOffset
			state.bodyPages = appendBodyPage(state.bodyPages, nextOffset)
		}
		if nextOffset >= 0 {
			state.bodyViewOffset = nextOffset
			state.bodyText = ""
			state.detailScroll = 0
			m.saveProjectView(m.selected)
			return m, m.readBodyByRef(m.selected, *state.bodyRef, nextOffset, state.bodyEvent)
		}
		m.status = "已到最终回复末尾"
	} else if pageIndex > 0 {
		state.bodyViewOffset = state.bodyPages[pageIndex-1]
		state.bodyText = ""
		state.detailScroll = 0
		m.saveProjectView(m.selected)
		return m, m.readBodyByRef(m.selected, *state.bodyRef, state.bodyViewOffset, state.bodyEvent)
	}
	m.saveProjectView(m.selected)
	return m, nil
}

func (m Model) loadMoreConversation() (tea.Model, tea.Cmd) {
	state := &m.projects[m.selected]
	if state.conversationInFlight {
		m.status = "会话读取仍在进行；完成后再次按 y 加载"
		return m, nil
	}
	if !state.conversationMore || state.conversationCursor == "" {
		m.status = "没有更多会话事件"
		return m, nil
	}
	cursor := state.conversationCursor
	return m, m.readConversation(m.selected, cursor, state.conversationKey, "page")
}

func (m Model) scrollPanel(delta int) (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[m.selected]
	if state.panel == 0 || state.panel == 1 || state.panel == 4 {
		lines := allActivityLines(*state, m.selectedRow(m.selected), activityPanelWidth(m))
		height := activityPanelHeight(m)
		if len(lines) <= height {
			return m, nil
		}
		if state.view.Follow {
			state.logScroll = max(0, len(lines)-height)
			state.view.Follow = false
		}
		state.logScroll -= delta
		maxStart := max(0, len(lines)-height)
		if state.logScroll < 0 {
			state.logScroll = 0
		}
		if state.logScroll > maxStart {
			state.logScroll = maxStart
		}
		m.saveProjectView(m.selected)
		return m, nil
	}
	if state.panel == 2 {
		state.detailScroll += delta
		if state.detailScroll < 0 {
			state.detailScroll = 0
		}
		lines := allSessionLines(*state, m.selectedRow(m.selected), activityPanelWidth(m))
		maxStart := max(0, len(lines)-activityPanelHeight(m))
		if state.detailScroll > maxStart {
			state.detailScroll = maxStart
		}
		captureSessionAnchor(state, m.selectedRow(m.selected), activityPanelWidth(m))
		m.saveProjectView(m.selected)
		return m, nil
	}
	state.detailScroll += delta
	if state.detailScroll < 0 {
		state.detailScroll = 0
	}
	return m, nil
}

func (m Model) scrollExpandedDetail(delta int) (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		return m, nil
	}
	state := m.projects[m.selected]
	state.detailScroll = 0
	lines := renderDetail(m, state, m.selectedRow(m.selected), m.width, 100000)
	maximum := max(0, len(lines)-consoleContentHeight(m))
	m.expandedDetailScroll = min(max(0, m.expandedDetailScroll+delta), maximum)
	return m, nil
}

func (m Model) readBodyByRef(index int, body contracts.ConversationBodyRef, offset int64, eventKey string) tea.Cmd {
	state := &m.projects[index]
	if state.bodyBusy {
		return nil
	}
	state.bodyBusy = true
	state.bodyGeneration++
	generation := state.bodyGeneration
	ref, query, ctx := state.ref, state.client, m.ctx
	return func() tea.Msg {
		page, err := query.Body(ctx, body, offset, 64<<10)
		return bodyMsg{projectKey: ref.Key, identity: ref.Identity, eventKey: eventKey, generation: generation, body: body, offset: offset, page: page, err: err}
	}
}

func validBodyPage(ref contracts.ConversationBodyRef, requestedOffset int64, page contracts.ConversationBodyPage) bool {
	if page.ItemKey != ref.ItemKey || page.Revision != ref.Revision || page.Offset != requestedOffset || page.TotalBytes != ref.Bytes || page.SHA256 != ref.SHA256 || page.Completeness != ref.Completeness || !utf8.Valid(page.Data) {
		return false
	}
	if page.NextOffset != page.Offset+int64(len(page.Data)) || page.NextOffset > page.TotalBytes || page.NextOffset < page.Offset {
		return false
	}
	return page.Complete == (page.NextOffset >= page.TotalBytes)
}

func findBodyEvent(events []contracts.ConversationEvent, itemKey string, revision int64) (contracts.ConversationEvent, bool) {
	for _, event := range events {
		if event.Body != nil && event.Body.ItemKey == itemKey && event.Body.Revision == revision {
			return event, true
		}
	}
	return contracts.ConversationEvent{}, false
}
