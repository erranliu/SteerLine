package console

import (
	"fmt"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
)

type TurnGroup struct {
	ID            string
	ProjectID     string
	ExecutionID   string
	AttemptID     string
	Source        string
	SourceSegment string
	SessionID     string
	TurnID        string
	Events        []steerline.ConversationEvent
	Process       []steerline.ConversationEvent
	Finals        []steerline.ConversationEvent
	Warnings      []steerline.ConversationEvent
	EndStatus     string
	EndReason     string
	FinalState    steerline.FinalAvailability
	Precise       bool
}

type ConversationView struct {
	Groups    []TurnGroup
	Summaries []steerline.ConversationEvent
	Warnings  []steerline.ConversationEvent
	Source    string
	Reason    string
}

func BuildConversationView(events []steerline.ConversationEvent, choices map[string]FoldChoice) (ConversationView, bool) {
	view := ConversationView{Groups: []TurnGroup{}, Summaries: []steerline.ConversationEvent{}, Warnings: []steerline.ConversationEvent{}}
	byID := map[string]int{}
	for _, event := range events {
		if event.Type == steerline.ConversationExecutionResult {
			view.Summaries = append(view.Summaries, event)
			continue
		}
		if isConversationWarning(event) {
			view.Warnings = append(view.Warnings, event)
		}
		id := turnGroupID(event)
		index, exists := byID[id]
		if !exists {
			index = len(view.Groups)
			byID[id] = index
			view.Groups = append(view.Groups, TurnGroup{
				ID: id, ProjectID: event.ProjectID, ExecutionID: event.ExecutionID, AttemptID: event.AttemptID,
				Source: event.Source, SourceSegment: event.SourceSegment, SessionID: event.SessionID, TurnID: event.TurnID,
				Events: []steerline.ConversationEvent{}, Process: []steerline.ConversationEvent{}, Finals: []steerline.ConversationEvent{},
				Warnings: []steerline.ConversationEvent{}, FinalState: steerline.FinalUnknown,
			})
		}
		group := &view.Groups[index]
		group.Events = append(group.Events, event)
		if event.Type == steerline.ConversationTurnStarted {
			group.Precise = group.Precise || event.TurnID != "" && (event.TurnQuality == steerline.IdentityNative || event.TurnQuality == steerline.IdentityDerived)
			if event.FinalAvailability != "" {
				group.FinalState = event.FinalAvailability
			}
		}
		if event.Type == steerline.ConversationTurnCompleted || event.Type == steerline.ConversationTurnFailed || event.Type == steerline.ConversationTurnCancelled {
			group.EndStatus = string(event.Type)
			group.EndReason = firstText(event.Diagnostic, event.Status, attributeString(event.Attributes, "reason"))
			if event.FinalAvailability != "" {
				group.FinalState = event.FinalAvailability
			}
		}
		if event.Phase == steerline.MessagePhaseFinalAnswer && isMessageEvent(event) {
			if conversationItemCanReplace(group.Process, event) {
				removeConversationItem(&group.Process, event)
				upsertConversationItem(&group.Finals, event)
			}
		} else if isProcessEvent(event) {
			if conversationItemCanReplace(group.Finals, event) {
				removeConversationItem(&group.Finals, event)
				upsertConversationItem(&group.Process, event)
			}
		}
		if isConversationWarning(event) {
			group.Warnings = append(group.Warnings, event)
		}
	}
	changed := false
	for i := range view.Groups {
		group := &view.Groups[i]
		for _, final := range group.Finals {
			if final.Body != nil && final.Body.Completeness != steerline.BodyMissing {
				group.FinalState = steerline.FinalAvailable
				break
			}
		}
		if group.EndStatus == "" || !group.Precise {
			continue
		}
		choice, exists := choices[group.ID]
		if !exists || !choice.AutoProcessed {
			choice.AutoProcessed = true
			choices[group.ID] = choice
			changed = true
		}
	}
	return view, changed
}

func upsertConversationItem(items *[]steerline.ConversationEvent, event steerline.ConversationEvent) {
	key := stableConversationItemKey(event)
	if key == "" {
		*items = append(*items, event)
		return
	}
	category := conversationItemCategory(event)
	for i := len(*items) - 1; i >= 0; i-- {
		previous := (*items)[i]
		if stableConversationItemKey(previous) == key && conversationItemCategory(previous) == category {
			if conversationEventNotOlder(event, previous) {
				(*items)[i] = event
			}
			return
		}
	}
	*items = append(*items, event)
}

func removeConversationItem(items *[]steerline.ConversationEvent, event steerline.ConversationEvent) {
	key := stableConversationItemKey(event)
	if key == "" {
		return
	}
	category := conversationItemCategory(event)
	kept := (*items)[:0]
	for _, previous := range *items {
		if stableConversationItemKey(previous) == key && conversationItemCategory(previous) == category {
			continue
		}
		kept = append(kept, previous)
	}
	*items = kept
}

func conversationItemCanReplace(items []steerline.ConversationEvent, event steerline.ConversationEvent) bool {
	key := stableConversationItemKey(event)
	if key == "" {
		return true
	}
	category := conversationItemCategory(event)
	for _, previous := range items {
		if stableConversationItemKey(previous) == key && conversationItemCategory(previous) == category {
			return conversationEventNotOlder(event, previous)
		}
	}
	return true
}

func conversationEventNotOlder(next, previous steerline.ConversationEvent) bool {
	if next.SourceRevision > 0 && previous.SourceRevision > 0 && next.SourceRevision != previous.SourceRevision {
		return next.SourceRevision > previous.SourceRevision
	}
	if next.SourceSeq > 0 && previous.SourceSeq > 0 && next.SourceSeq != previous.SourceSeq {
		return next.SourceSeq > previous.SourceSeq
	}
	if next.EventID > 0 && previous.EventID > 0 {
		return next.EventID >= previous.EventID
	}
	return true
}

func stableConversationItemKey(event steerline.ConversationEvent) string {
	if event.ItemKey != "" {
		return event.ItemKey
	}
	if event.ItemID != "" && (event.ItemQuality == steerline.IdentityNative || event.ItemQuality == steerline.IdentityDerived) {
		return "item:" + event.ItemID
	}
	return ""
}

func conversationItemCategory(event steerline.ConversationEvent) string {
	switch event.Type {
	case steerline.ConversationMessageDelta, steerline.ConversationMessageSnapshot, steerline.ConversationMessageComplete:
		return "message"
	case steerline.ConversationToolStarted, steerline.ConversationToolDelta, steerline.ConversationToolCompleted:
		return "tool"
	default:
		return string(event.Type)
	}
}

func turnGroupID(event steerline.ConversationEvent) string {
	return strings.Join([]string{
		event.ProjectID, event.ExecutionID, event.AttemptID, event.Source,
		event.SourceSegment, event.SessionID, event.TurnID,
	}, "\x00")
}

func isProcessEvent(event steerline.ConversationEvent) bool {
	switch event.Type {
	case steerline.ConversationTurnStarted, steerline.ConversationMessageDelta, steerline.ConversationMessageSnapshot,
		steerline.ConversationMessageComplete, steerline.ConversationToolStarted, steerline.ConversationToolDelta,
		steerline.ConversationToolCompleted, steerline.ConversationUnknown, steerline.ConversationObservation:
		return true
	default:
		return false
	}
}

func isMessageEvent(event steerline.ConversationEvent) bool {
	switch event.Type {
	case steerline.ConversationMessageDelta, steerline.ConversationMessageSnapshot, steerline.ConversationMessageComplete:
		return true
	default:
		return false
	}
}

func isConversationWarning(event steerline.ConversationEvent) bool {
	return event.Type == steerline.ConversationGap || event.Type == steerline.ConversationInputWait || event.Diagnostic != "" || event.Status == "error" || event.Status == "failed"
}

func Folded(group TurnGroup, choice FoldChoice) bool {
	if choice.KeepExpanded {
		return false
	}
	if choice.Manual != nil {
		return *choice.Manual
	}
	return group.EndStatus != "" && choice.AutoProcessed
}

func ToggleFold(choices map[string]FoldChoice, group TurnGroup) {
	choice := choices[group.ID]
	fold := !Folded(group, choice)
	choice.Manual = &fold
	choice.KeepExpanded = false
	choices[group.ID] = choice
}

func SetKeepExpanded(choices map[string]FoldChoice, group TurnGroup, keep bool) {
	choice := choices[group.ID]
	choice.KeepExpanded = keep
	if keep {
		open := false
		choice.Manual = &open
	}
	choices[group.ID] = choice
}

func statusLabel(group TurnGroup) string {
	switch group.EndStatus {
	case string(steerline.ConversationTurnCompleted):
		return "completed"
	case string(steerline.ConversationTurnFailed):
		return "failed"
	case string(steerline.ConversationTurnCancelled):
		return "cancelled"
	default:
		return "active"
	}
}

func groupDuration(group TurnGroup) string {
	var first, last time.Time
	for _, event := range group.Events {
		if event.OccurredAt == nil {
			continue
		}
		if first.IsZero() || event.OccurredAt.Before(first) {
			first = *event.OccurredAt
		}
		if event.OccurredAt.After(last) {
			last = *event.OccurredAt
		}
	}
	if first.IsZero() || last.IsZero() {
		return "耗时未知"
	}
	d := last.Sub(first)
	if d < 0 {
		return "耗时未知"
	}
	return "耗时 " + d.Round(time.Second).String()
}

func groupHeading(group TurnGroup, choice FoldChoice) string {
	warningCount, errorCount := 0, 0
	for _, event := range group.Warnings {
		if event.Type == steerline.ConversationGap || event.Type == steerline.ConversationInputWait {
			warningCount++
		}
		if event.Status == "error" || event.Status == "failed" || event.Type == steerline.ConversationTurnFailed || event.Type == steerline.ConversationTurnCancelled {
			errorCount++
		}
	}
	final := string(group.FinalState)
	if final == "" {
		final = string(steerline.FinalUnknown)
	}
	if Folded(group, choice) {
		return fmt.Sprintf("▸ [%s] %s · %s · %s · %d 项 · 警告 %d/错误 %d · final %s", groupToken(group.ID), statusLabel(group), groupDuration(group), group.TurnID, len(group.Process), warningCount, errorCount, final)
	}
	return fmt.Sprintf("▾ [%s] %s · %s · %s · %d 项 · 警告 %d/错误 %d · final %s", groupToken(group.ID), statusLabel(group), groupDuration(group), group.TurnID, len(group.Process), warningCount, errorCount, final)
}

func firstText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func attributeString(attributes map[string]any, key string) string {
	if value, ok := attributes[key].(string); ok {
		return value
	}
	return ""
}
