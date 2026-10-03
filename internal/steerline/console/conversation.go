package console

import (
	"fmt"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
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
	Events        []contracts.ConversationEvent
	Process       []contracts.ConversationEvent
	Finals        []contracts.ConversationEvent
	Warnings      []contracts.ConversationEvent
	EndStatus     string
	EndReason     string
	FinalState    contracts.FinalAvailability
	Precise       bool
}

type ConversationView struct {
	Groups    []TurnGroup
	Summaries []contracts.ConversationEvent
	Warnings  []contracts.ConversationEvent
	Source    string
	Reason    string
}

func BuildConversationView(events []contracts.ConversationEvent, choices map[string]FoldChoice) (ConversationView, bool) {
	view := ConversationView{Groups: []TurnGroup{}, Summaries: []contracts.ConversationEvent{}, Warnings: []contracts.ConversationEvent{}}
	byID := map[string]int{}
	for _, event := range events {
		if event.Type == contracts.ConversationExecutionResult {
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
				Events: []contracts.ConversationEvent{}, Process: []contracts.ConversationEvent{}, Finals: []contracts.ConversationEvent{},
				Warnings: []contracts.ConversationEvent{}, FinalState: contracts.FinalUnknown,
			})
		}
		group := &view.Groups[index]
		group.Events = append(group.Events, event)
		if event.Type == contracts.ConversationTurnStarted {
			group.Precise = group.Precise || event.TurnID != "" && (event.TurnQuality == contracts.IdentityNative || event.TurnQuality == contracts.IdentityDerived)
			if event.FinalAvailability != "" {
				group.FinalState = event.FinalAvailability
			}
		}
		if event.Type == contracts.ConversationTurnCompleted || event.Type == contracts.ConversationTurnFailed || event.Type == contracts.ConversationTurnCancelled {
			group.EndStatus = string(event.Type)
			group.EndReason = firstText(event.Diagnostic, event.Status, attributeString(event.Attributes, "reason"))
			if event.FinalAvailability != "" {
				group.FinalState = event.FinalAvailability
			}
		}
		if event.Phase == contracts.MessagePhaseFinalAnswer && isMessageEvent(event) {
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
			if final.Body != nil && final.Body.Completeness != contracts.BodyMissing {
				group.FinalState = contracts.FinalAvailable
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

func upsertConversationItem(items *[]contracts.ConversationEvent, event contracts.ConversationEvent) {
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

func removeConversationItem(items *[]contracts.ConversationEvent, event contracts.ConversationEvent) {
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

func conversationItemCanReplace(items []contracts.ConversationEvent, event contracts.ConversationEvent) bool {
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

func conversationEventNotOlder(next, previous contracts.ConversationEvent) bool {
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

func stableConversationItemKey(event contracts.ConversationEvent) string {
	if event.ItemKey != "" {
		return event.ItemKey
	}
	if event.ItemID != "" && (event.ItemQuality == contracts.IdentityNative || event.ItemQuality == contracts.IdentityDerived) {
		return "item:" + event.ItemID
	}
	return ""
}

func conversationItemCategory(event contracts.ConversationEvent) string {
	switch event.Type {
	case contracts.ConversationMessageDelta, contracts.ConversationMessageSnapshot, contracts.ConversationMessageComplete:
		return "message"
	case contracts.ConversationToolStarted, contracts.ConversationToolDelta, contracts.ConversationToolCompleted:
		return "tool"
	default:
		return string(event.Type)
	}
}

func turnGroupID(event contracts.ConversationEvent) string {
	return strings.Join([]string{
		event.ProjectID, event.ExecutionID, event.AttemptID, event.Source,
		event.SourceSegment, event.SessionID, event.TurnID,
	}, "\x00")
}

func isProcessEvent(event contracts.ConversationEvent) bool {
	switch event.Type {
	case contracts.ConversationTurnStarted, contracts.ConversationMessageDelta, contracts.ConversationMessageSnapshot,
		contracts.ConversationMessageComplete, contracts.ConversationToolStarted, contracts.ConversationToolDelta,
		contracts.ConversationToolCompleted, contracts.ConversationUnknown, contracts.ConversationObservation:
		return true
	default:
		return false
	}
}

func isMessageEvent(event contracts.ConversationEvent) bool {
	switch event.Type {
	case contracts.ConversationMessageDelta, contracts.ConversationMessageSnapshot, contracts.ConversationMessageComplete:
		return true
	default:
		return false
	}
}

func isConversationWarning(event contracts.ConversationEvent) bool {
	return event.Type == contracts.ConversationGap || event.Type == contracts.ConversationInputWait || event.Diagnostic != "" || event.Status == "error" || event.Status == "failed"
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
	case string(contracts.ConversationTurnCompleted):
		return "completed"
	case string(contracts.ConversationTurnFailed):
		return "failed"
	case string(contracts.ConversationTurnCancelled):
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
		if event.Type == contracts.ConversationGap || event.Type == contracts.ConversationInputWait {
			warningCount++
		}
		if event.Status == "error" || event.Status == "failed" || event.Type == contracts.ConversationTurnFailed || event.Type == contracts.ConversationTurnCancelled {
			errorCount++
		}
	}
	final := string(group.FinalState)
	if final == "" {
		final = string(contracts.FinalUnknown)
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
