package console

import (
	"fmt"
	"sort"
	"strings"

	runewidth "github.com/mattn/go-runewidth"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

// RenderOptions controls human rendering of activity records. Rendering never
// affects the stored data and may change between releases; the JSONL contract in
// docs/activity.md is what stays stable.
type RenderOptions struct {
	Color      bool
	MaxLines   int // max detail lines; <=0 means default
	MaxColumns int // max columns per line; <=0 means default
}

const (
	ansiReset   = "\x1b[0m"
	ansiDim     = "\x1b[2m"
	ansiRed     = "\x1b[31m"
	ansiYellow  = "\x1b[33m"
	ansiBlue    = "\x1b[34m"
	ansiMagenta = "\x1b[35m"
	ansiCyan    = "\x1b[36m"
)

func statusMark(status string) string {
	switch status {
	case "started", "running":
		return ">"
	case "completed":
		return "+"
	case "failed":
		return "!"
	case "pending":
		return "*"
	case "info", "":

	}
	return "?"
}

func kindColor(kind string) string {
	switch kind {
	case "session", "turn":
		return ansiCyan
	case "reasoning", "note", "unknown":
		return ansiDim
	case "command":
		return ansiYellow
	case "file_change":
		return ansiMagenta
	case "decision":
		return ansiMagenta
	case "error":
		return ansiRed
	case "usage":
		return ansiBlue
	}
	return ""
}

func paint(s, color string, on bool) string {
	if !on || color == "" {
		return s
	}
	return color + s + ansiReset
}

// clipWidth truncates plain (uncolored) text to a display width, counting wide
// runes like CJK as two columns, so a rendered line can never overflow its pane.
func clipWidth(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= max {
		return s
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if w+rw > max-1 {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String() + "…"
}

// activityIndent is the fixed indent used for detail lines under a head line.
const activityIndent = "           "

// RenderActivity renders one record as a compact human line plus bounded detail.
func RenderActivity(ev api.ActivityEvent, o RenderOptions) string {
	maxLines := o.MaxLines
	if maxLines <= 0 {
		maxLines = 20
	}
	maxColumns := o.MaxColumns
	if maxColumns <= 0 {
		maxColumns = 240
	}
	var b strings.Builder
	ts := ev.TS.Local().Format("15:04:05")
	mark := statusMark(ev.Status)
	head := fmt.Sprintf("%s  %s %-11s", ts, mark, ev.Kind)
	if ev.Title != "" {
		head += " " + ev.Title
	}
	if suffix := activitySuffix(ev); suffix != "" {
		head += "  " + suffix
	}
	b.WriteString(paint(clipWidth(head, maxColumns), kindColor(ev.Kind), o.Color))
	if ev.Detail != "" {
		detailWidth := maxColumns - len(activityIndent)
		if detailWidth < 8 {
			detailWidth = 8
		}
		lines := strings.Split(strings.ReplaceAll(ev.Detail, "\r\n", "\n"), "\n")
		for i, line := range lines {
			if i >= maxLines {
				b.WriteString("\n" + paint(fmt.Sprintf(activityIndent+"… (%d more lines)", len(lines)-i), ansiDim, o.Color))
				break
			}
			trimmed := strings.TrimRight(line, " \t")
			if trimmed == "" {
				continue
			}
			b.WriteString("\n" + activityIndent + clipWidth(trimmed, detailWidth))
		}
	}
	return b.String()
}

func activitySuffix(ev api.ActivityEvent) string {
	var parts []string
	if ev.ItemID != "" {
		parts = append(parts, "#"+ev.ItemID)
	}
	if ev.Status == "started" || ev.Status == "running" {
		parts = append(parts, "running")
	}
	if ev.ElapsedMS > 0 {
		parts = append(parts, fmt.Sprintf("%.1fs", float64(ev.ElapsedMS)/1000))
	}
	if len(ev.Meta) > 0 {
		if v, ok := ev.Meta["exit_code"]; ok {
			parts = append(parts, fmt.Sprintf("exit %v", v))
		}
		if ev.Kind == "usage" {
			if u, ok := ev.Meta["usage"].(map[string]any); ok {
				parts = append(parts, formatUsage(u))
			}
		}
	}
	return strings.Join(parts, " · ")
}

func formatUsage(u map[string]any) string {
	keys := []string{"input_tokens", "cached_input_tokens", "output_tokens", "reasoning_output_tokens"}
	var parts []string
	for _, k := range keys {
		if v, ok := u[k]; ok {
			parts = append(parts, fmt.Sprintf("%s %v", usageLabel(k), v))
		}
	}
	if len(parts) == 0 {
		// Unknown usage shape: keep a deterministic, bounded summary.
		names := make([]string, 0, len(u))
		for k := range u {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			parts = append(parts, fmt.Sprintf("%s %v", k, u[k]))
		}
	}
	return "tokens " + strings.Join(parts, " ")
}

func usageLabel(key string) string {
	switch key {
	case "input_tokens":

	case "cached_input_tokens":

	case "output_tokens":

	case "reasoning_output_tokens":

	}
	return key
}
