package console

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

type nativeSessionOpener interface {
	OpenNativeSession(context.Context, api.NativeSessionTarget) error
}

type nativeSessionOpenedMsg struct {
	projectKey, runID string
	err               error
}

func nativeSessionAvailable(run *api.CandidateSummary) bool {
	return run != nil && run.Host == "local" && run.Agent == "codex" && run.SessionID != "" && run.SessionGeneration != "" && run.CurrentTurnID != ""
}

func (m Model) openNativeSession() (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[m.selected]
	row := m.selectedRow(m.selected)
	if row == nil || !nativeSessionAvailable(row.run) {
		m.status = "Codex CLI 打开不可用：请选择已开始原生执行轮次的本机 Codex 候选"
		return m, nil
	}
	opener, ok := state.client.(nativeSessionOpener)
	if !ok {
		m.status = "当前客户端不支持打开 Codex CLI"
		return m, nil
	}
	if state.nativeSessionOpening {
		m.status = "正在打开 Codex CLI"
		return m, nil
	}
	run := row.run
	target := api.NativeSessionTarget{RunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, SessionID: run.SessionID, SessionGeneration: run.SessionGeneration}
	key := state.ref.Key
	state.nativeSessionOpening = true
	m.status = "正在核验原会话并打开 Codex CLI"
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		return nativeSessionOpenedMsg{projectKey: key, runID: target.RunID, err: opener.OpenNativeSession(ctx, target)}
	}
}

func (m Model) applyNativeSessionOpened(msg nativeSessionOpenedMsg) (tea.Model, tea.Cmd) {
	index := m.projectIndex(msg.projectKey)
	if index < 0 {
		return m, nil
	}
	m.projects[index].nativeSessionOpening = false
	if index == m.selected {
		if msg.err != nil {
			m.status = "Codex CLI 未打开：" + safeTerminalInlineText(msg.err.Error())
		} else {
			m.status = "已打开 Codex CLI · 候选 " + msg.runID + " · 任务操作请通过 SteerLine"
		}
	}
	return m, nil
}
