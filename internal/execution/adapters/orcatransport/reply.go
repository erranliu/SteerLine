package orcatransport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
)

const replySource = "orca.terminal.rendered-reply.v1"
const maxReplyBytes = 32 << 10

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// These are Codex's rendered boundaries, never instructions sent to the model.
var completedLine = regexp.MustCompile(`^Worked for [0-9hms .]+ • [0-9]{2}:[0-9]{2}$`)

func completionBounds(lines []string) (user, end int) {
	user, end = -1, -1
	for i, line := range lines {
		if completedLine.MatchString(strings.Trim(strings.TrimSpace(line), "─ ")) {
			end = i
		}
	}
	if end >= 0 {
		for i := 0; i < end; i++ {
			if strings.HasPrefix(strings.TrimSpace(lines[i]), "› ") {
				user = i
			}
		}
	}
	return
}

func screenBaseline(lines []string) string {
	user, end := completionBounds(lines)
	if user < 0 || end < 0 {
		return digest(nil)
	}
	return digest([]byte(strings.Join(lines[user:end+1], "\n")))
}

func renderedReply(agent string, lines []string) (text, fingerprint, problem string) {
	if agent != "codex" {
		return "", "", "agent rendered reply boundaries are unsupported"
	}
	user, end := completionBounds(lines)
	if user < 0 || end < 0 {
		return "", "", "visible user-message and completion boundaries are missing"
	}
	last := -1
	for i := user + 1; i < end; i++ {
		if strings.HasPrefix(lines[i], "• ") || lines[i] == "•" {
			last = i
		}
	}
	if last < 0 {
		return "", "", "no assistant reply after the submitted input"
	}
	// The user-message marker delimits the reply, even when the TUI collapses
	// its text or clips preceding tool history. The lifecycle binds the input
	// through the original send/start receipt and rejects the pre-send baseline.
	body := []string{strings.TrimLeft(strings.TrimPrefix(lines[last], "•"), " ")}
	for _, line := range lines[last+1 : end] {
		if strings.HasPrefix(strings.TrimSpace(line), "└ Tip:") {
			break
		}
		body = append(body, strings.TrimPrefix(line, "  "))
	}
	text = strings.TrimSpace(strings.Join(body, "\n"))
	if text == "" || len(text) > maxReplyBytes || strings.HasPrefix(text, "Ran ") || strings.HasPrefix(text, "Explored ") || strings.Contains(text, "ctrl+t to expand") {
		return "", "", "final assistant body is missing, collapsed or oversized"
	}
	// The CLI omits soft-wrap metadata. Keep display line breaks as whitespace in
	// prose instead of silently concatenating words. JSON keys and URI tokens
	// are rejoined, while prose string values retain display breaks as \n.
	if strings.HasPrefix(text, "```json\n") && strings.HasSuffix(text, "\n```") {
		text = strings.TrimSuffix(strings.TrimPrefix(text, "```json\n"), "\n```")
	}
	if strings.HasPrefix(text, "{") {
		text = renderedJSON(text)
		if !json.Valid([]byte(text)) {
			return "", "", "rendered JSON reply is incomplete or invalid"
		}
	}

	if len(text) > maxReplyBytes {
		return "", "", "normalized reply is oversized"
	}
	return text, screenBaseline(lines), ""
}

func renderedJSON(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); {
		if text[i] != '"' {
			out.WriteByte(text[i])
			i++
			continue
		}
		end := i + 1
		escaped := false
		for ; end < len(text); end++ {
			c := text[end]
			if c == '\n' {
				continue
			}
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				break
			}
		}
		if end == len(text) {
			return text
		}
		body := text[i+1 : end]
		joined := strings.ReplaceAll(body, "\n", "")
		var decoded string
		_ = json.Unmarshal([]byte("\""+joined+"\""), &decoded)
		suffix := strings.TrimSpace(text[end+1:])
		token := strings.HasPrefix(suffix, ":") || strings.HasPrefix(decoded, "https://") || strings.HasPrefix(decoded, "http://")
		out.WriteByte('"')
		escaped = false
		for _, c := range body {
			if c == '\n' {
				if !token && !escaped {
					out.WriteString("\\n")
				}
				continue
			}
			out.WriteRune(c)
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			}
		}
		out.WriteByte('"')
		i = end + 1
	}
	return out.String()
}
