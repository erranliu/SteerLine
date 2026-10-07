package console

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

type finalPreviewMsg struct {
	projectKey, identity, key string
	generation                uint64
	event                     api.ConversationEvent
	text                      string
	err                       error
}

func previewPrefix(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) > 6 {
		lines = lines[:6]
	}
	runes := []rune(strings.Join(lines, "\n"))
	if len(runes) > 800 {
		runes = runes[:800]
	}
	return string(runes)
}

func (m Model) readFinalPreview(index int, event api.ConversationEvent) tea.Cmd {
	s := &m.projects[index]
	if s.previewEvent != nil && conversationItemKey(*s.previewEvent) == conversationItemKey(event) && s.previewEvent.SourceRevision == event.SourceRevision && s.previewEvent.Body != nil && event.Body != nil && *s.previewEvent.Body == *event.Body && s.previewError == "" {
		return nil
	}
	s.previewGeneration++
	s.previewText = ""
	s.previewError = ""
	s.previewEvent = &event
	body := event.Body
	if body == nil || body.ExecutionID != event.ExecutionID || body.AttemptID != event.AttemptID || body.Bytes < 0 || body.Bytes > 16<<20 || body.Completeness != api.BodyComplete || event.Type != api.ConversationMessageComplete {
		s.previewError = "final 正文或轮次尚未完整采集"
		return nil
	}
	generation, key, ref, client, ctx := s.previewGeneration, s.finalKey, s.ref, s.client, m.ctx
	return func() tea.Msg {
		page, err := client.Body(ctx, *body, 0, 4096)
		if err == nil && (!validBodyPage(*body, 0, page) || len(page.Data) > 4096) {
			err = fmt.Errorf("final 预览身份/版本/大小不匹配")
		}
		if err == nil && page.Complete {
			digest := sha256.Sum256([]byte(page.Data))
			if hex.EncodeToString(digest[:]) != body.SHA256 {
				err = fmt.Errorf("final 正文摘要不匹配")
			}
		}
		return finalPreviewMsg{ref.Key, ref.Identity, key, generation, event, previewPrefix(string(page.Data)), err}
	}
}

func (m Model) updateFinalPreview(msg finalPreviewMsg) (tea.Model, tea.Cmd) {
	i := m.projectIndex(msg.projectKey)
	if i < 0 {
		return m, nil
	}
	s := &m.projects[i]
	if msg.identity != s.ref.Identity || msg.key != s.finalKey || msg.generation != s.previewGeneration {
		return m, nil
	}
	row := m.selectedRow(i)
	if row == nil || row.run == nil || candidateIdentity(row.run) != s.finalKey || row.run.AttemptID != msg.event.AttemptID {
		return m, nil
	}
	if row.run.CurrentTurnID != "" && row.run.CurrentTurnID != msg.event.TurnID {
		return m, nil
	}
	if msg.err != nil {
		s.previewError = msg.err.Error()
		s.previewText = ""
	} else {
		s.previewText = msg.text
		s.previewError = ""
	}
	return m, nil
}

func (m Model) readSelectedFinalFull() tea.Cmd {
	return m.readFinalReplies(m.selected)
}
