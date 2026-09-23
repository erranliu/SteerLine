package viewer

import (
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline"
)

// A folded item keeps its identity when a later completion replaces its seq.
// Events without item IDs retain their journal sequence within the generation.
type logLineAnchor struct {
	itemID string
	seq    uint64
	line   int
}

func (m Model) logLineAnchors() []logLineAnchor {
	_, width := paneWidths(m.width)
	if width < 1 {
		width = 1
	}
	var anchors []logLineAnchor
	for _, event := range foldEvents(m.log, m.mode == modeConversation) {
		seq := event.Seq
		if event.ItemID != "" {
			seq = 0
		}
		lines := strings.Split(steerline.RenderActivity(event, steerline.RenderOptions{Color: m.opts.Color, MaxColumns: width}), "\n")
		for line := range lines {
			anchors = append(anchors, logLineAnchor{itemID: event.ItemID, seq: seq, line: line})
		}
	}
	return anchors
}

func (m Model) scrollAnchor() (logLineAnchor, bool) {
	if m.autoScroll || m.mode == modeRaw {
		return logLineAnchor{}, false
	}
	anchors := m.logLineAnchors()
	if len(anchors) == 0 {
		return logLineAnchor{}, false
	}
	index := len(anchors) - m.logScroll - 1
	if index < 0 {
		index = 0
	}
	return anchors[index], true
}

func (m *Model) restoreScrollAnchor(anchor logLineAnchor) bool {
	anchors := m.logLineAnchors()
	matched := -1
	for i, candidate := range anchors {
		if candidate.itemID == anchor.itemID && candidate.seq == anchor.seq {
			matched = i
			if candidate.line == anchor.line {
				break
			}
		}
	}
	if matched < 0 {
		return false // Retention removed the anchor; the caller clamps the view.
	}
	m.logScroll = len(anchors) - matched - 1
	return true
}
