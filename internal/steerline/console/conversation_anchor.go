package console

// Stable conversation tokens and scroll anchors shared by rendering and updates.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func allSessionLines(state projectState, row *consoleRow, width int) []string {
	state.detailScroll = 0
	return renderSession(state, row, width, 1<<20, time.Now())
}

func captureSessionAnchor(state *projectState, row *consoleRow, width int) {
	if state == nil {
		return
	}
	lines := allSessionLines(*state, row, width)
	if len(lines) == 0 {
		return
	}
	top := state.detailScroll
	if top < 0 {
		top = 0
	}
	if top >= len(lines) {
		top = len(lines) - 1
	}
	view, _ := BuildConversationView(state.conversationEvents, state.view.Groups)
	selected, headerLine := -1, -1
	for i, group := range view.Groups {
		needle := "[" + groupToken(group.ID) + "]"
		for lineIndex := 0; lineIndex <= top && lineIndex < len(lines); lineIndex++ {
			if strings.Contains(lines[lineIndex], needle) {
				selected, headerLine = i, lineIndex
			}
		}
	}
	if selected < 0 {
		return
	}
	group := view.Groups[selected]
	state.view.ReadGroup = group.ID
	state.view.ReadItem = ""
	state.view.ReadRevision = 0
	itemLine := -1
	events := append(append([]contracts.ConversationEvent(nil), group.Process...), group.Finals...)
	for _, event := range events {
		needle := "[" + itemToken(event) + "]"
		for lineIndex := headerLine; lineIndex <= top && lineIndex < len(lines); lineIndex++ {
			if strings.Contains(lines[lineIndex], needle) {
				itemLine = lineIndex
				state.view.ReadItem = sessionItemAnchorKey(event)
				if event.Body != nil {
					state.view.ReadRevision = event.Body.Revision
				}
			}
		}
	}
	if itemLine >= 0 {
		state.view.ReadLine = top - itemLine
	} else {
		state.view.ReadLine = top - headerLine
	}
}

func restoreSessionAnchor(state *projectState, row *consoleRow, width int) bool {
	if state == nil || state.view.ReadGroup == "" {
		return false
	}
	lines := allSessionLines(*state, row, width)
	view, _ := BuildConversationView(state.conversationEvents, state.view.Groups)
	groupIndex := -1
	for i, group := range view.Groups {
		if group.ID == state.view.ReadGroup {
			groupIndex = i
			break
		}
	}
	if groupIndex < 0 {
		return false
	}
	group := view.Groups[groupIndex]
	headerLine := -1
	needle := "[" + groupToken(group.ID) + "]"
	for i, line := range lines {
		if strings.Contains(line, needle) {
			headerLine = i
			break
		}
	}
	if headerLine < 0 {
		return false
	}
	anchorLine := headerLine
	itemHidden := false
	if state.view.ReadItem != "" {
		foundItem := false
		itemVisible := false
		events := append(append([]contracts.ConversationEvent(nil), group.Process...), group.Finals...)
		for _, event := range events {
			if sessionItemAnchorKey(event) != state.view.ReadItem {
				continue
			}
			foundItem = true
			if event.Body != nil && state.view.ReadRevision != event.Body.Revision {
				itemHidden = true
				continue
			}
			itemNeedle := "[" + itemToken(event) + "]"
			for i := headerLine; i < len(lines); i++ {
				if strings.Contains(lines[i], itemNeedle) {
					anchorLine = i
					itemVisible = true
					break
				}
			}
			if itemVisible {
				break
			}
		}
		if !foundItem {
			return false
		}
		itemHidden = !itemVisible
	}
	if itemHidden {
		state.detailScroll = headerLine
		return true
	}
	state.detailScroll = anchorLine + state.view.ReadLine
	if state.detailScroll < 0 {
		state.detailScroll = 0
	}
	return false
}

func shortStableToken(prefix, value string) string {
	hash := sha256.Sum256([]byte(value))
	return prefix + hex.EncodeToString(hash[:4])
}

func groupToken(groupID string) string { return shortStableToken("g:", groupID) }

func itemToken(event contracts.ConversationEvent) string {
	return shortStableToken("i:", conversationItemKey(event))
}

func sessionItemAnchorKey(event contracts.ConversationEvent) string {
	if event.Body != nil {
		return "body:" + event.Body.ItemKey
	}
	if event.ItemKey != "" {
		return "item:" + event.ItemKey
	}
	if event.ItemID != "" && (event.ItemQuality == contracts.IdentityNative || event.ItemQuality == contracts.IdentityDerived) {
		return "item:" + event.ItemID
	}
	return conversationItemKey(event)
}

func conversationItemKey(event contracts.ConversationEvent) string {
	if event.Body != nil {
		return fmt.Sprintf("%s\x00%s\x00%s\x00%d", event.ExecutionID, event.AttemptID, event.Body.ItemKey, event.Body.Revision)
	}
	return strings.Join([]string{event.ExecutionID, event.AttemptID, event.Source, event.SourceSegment, event.SessionID, event.TurnID, event.ItemID, event.SourceEventID}, "\x00")
}
