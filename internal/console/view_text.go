package console

// Terminal text escaping, width handling and scrolling shared by console views.

import (
	"fmt"

	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func padDisplay(value string, width int) string {
	return value + strings.Repeat(" ", max(0, width-lipgloss.Width(value)))
}

func firstText(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func fitLines(lines []string, width, height int) []string {
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		if i < len(lines) {
			out = append(out, truncate(lines[i], width))
		} else {
			out = append(out, "")
		}
	}
	return out
}

func scrollLines(lines []string, offset, height int) []string {
	if height < 1 {
		return nil
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(lines) {
		offset = max(0, len(lines)-height)
	}
	end := offset + height
	if end > len(lines) {
		end = len(lines)
	}
	return lines[offset:end]
}

func scrollLinesFromEnd(lines []string, offset, height int) []string {
	if height < 1 {
		return nil
	}
	if offset < 0 {
		offset = 0
	}
	end := len(lines) - offset
	if end < 0 {
		end = 0
	}
	start := end - height
	if start < 0 {
		start = 0
	}
	return lines[start:end]
}

func wrapLines(lines []string, width int) []string {
	if width < 1 {
		return lines
	}
	style := lipgloss.NewStyle().Width(width).MaxWidth(width)
	var wrapped []string
	for _, line := range lines {
		if line == "" {
			wrapped = append(wrapped, "")
			continue
		}
		wrapped = append(wrapped, strings.Split(style.Render(line), "\n")...)
	}
	return wrapped
}

func truncate(value string, width int) string {
	value = strings.ReplaceAll(value, "\r", "␍")
	if width < 1 {
		return ""
	}
	return ansi.Truncate(value, width, "…")
}

func safeTerminalText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r == '\n' || r == '\t':
			builder.WriteRune(r)
		case r == 0x1b:
			builder.WriteString("␛")
		case unicode.IsControl(r) || isBidiControl(r):
			builder.WriteString(fmt.Sprintf("\\u%04x", r))
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func safeTerminalInlineText(value string) string {
	return strings.NewReplacer("\n", "\\n", "\t", "\\t").Replace(safeTerminalText(value))
}

func isBidiControl(r rune) bool {
	return r == '\u061c' || r == '\u200e' || r == '\u200f' || r >= '\u202a' && r <= '\u202e' || r >= '\u2066' && r <= '\u2069'
}
