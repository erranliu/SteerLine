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
	viewer            string
	err               error
}

func nativeSessionAvailable(run *api.CandidateSummary) bool {
	return run != nil && (run.Host == "local" && run.Agent == "codex" || run.Host == "orca") && run.SessionID != "" && run.SessionGeneration != "" && (run.Host == "orca" || run.CurrentTurnID != "")
}

func nativeSessionLabel(run *api.CandidateSummary) string {
	if run != nil && run.Host == "orca" {
		return "Orca"
	}
	return "Codex CLI"
}

func (m Model) openNativeSession() (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.projects) {
		return m, nil
	}
	state := &m.projects[m.selected]
	row := m.selectedRow(m.selected)
	if row == nil || !nativeSessionAvailable(row.run) {
		m.status = "会话查看不可用：请选择已有原生会话的 Codex 或 Orca 候选"
		return m, nil
	}
	opener, ok := state.client.(nativeSessionOpener)
	if !ok {
		m.status = "当前客户端不支持查看原执行会话"
		return m, nil
	}
	if state.nativeSessionOpening {
		m.status = "正在打开 " + nativeSessionLabel(row.run)
		return m, nil
	}
	run := row.run
	target := api.NativeSessionTarget{RunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, SessionID: run.SessionID, SessionGeneration: run.SessionGeneration}
	key := state.ref.Key
	state.nativeSessionOpening = true
	viewer := nativeSessionLabel(run)
	m.status = "正在核验原会话并打开 " + viewer
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 45*time.Second)
		defer cancel()
		return nativeSessionOpenedMsg{projectKey: key, runID: target.RunID, viewer: viewer, err: opener.OpenNativeSession(ctx, target)}
	}
}

func (m Model) applyNativeSessionOpened(msg nativeSessionOpenedMsg) (tea.Model, tea.Cmd) {
	index := m.projectIndex(msg.projectKey)
	if index < 0 {
		return m, nil
	}
	m.projects[index].nativeSessionOpening = false
	if index == m.selected {
		if msg.viewer == "" {
			msg.viewer = "Codex CLI"
		}
		if msg.err != nil {
			m.status = msg.viewer + " 未打开：" + safeTerminalInlineText(msg.err.Error())
		} else {
			m.status = "已打开 " + msg.viewer + " · 候选 " + msg.runID + " · 任务操作请通过 SteerLine"
		}
	}
	return m, nil
}
