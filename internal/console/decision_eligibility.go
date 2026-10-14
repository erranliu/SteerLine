package console

import tea "github.com/charmbracelet/bubbletea"

type decisionEligibilityCheck struct {
	ID, ProjectKey, RepositoryIdentity, CandidateIdentity, InstanceID string
	RunID, DecisionID, AttemptID                                      string
	RunRevision, DecisionVersion, RecoveryVersion                     uint64
}

type decisionEligibilityMsg struct {
	check decisionEligibilityCheck
	err   error
}

func (m Model) updateDecisionEligibility(msg decisionEligibilityMsg) (tea.Model, tea.Cmd) {
	if m.answerCheck == nil || *m.answerCheck != msg.check {
		return m, nil
	}
	m.answerCheck = nil
	index := m.projectIndex(msg.check.ProjectKey)
	if index < 0 || index != m.selected {
		return m, nil
	}
	state := &m.projects[index]
	row := m.selectedRow(index)
	if state.ref.Identity != msg.check.RepositoryIdentity || row == nil || row.run == nil || row.run.Decision == nil || state.snapshot.Connection.Snapshot.Page == nil {
		return m, nil
	}
	run := row.run
	if candidateIdentity(run) != msg.check.CandidateIdentity || state.snapshot.Connection.Snapshot.Page.Service.InstanceID != msg.check.InstanceID || run.RunID != msg.check.RunID || run.AttemptID != msg.check.AttemptID || run.ControlRevision != msg.check.RunRevision || run.RecoveryVersion != msg.check.RecoveryVersion || run.Decision.ID != msg.check.DecisionID || run.Decision.Version != msg.check.DecisionVersion {
		m.status = "原问题身份或版本已变化；请重新核对后打开反馈"
		return m, nil
	}
	if msg.err != nil && run.EnvironmentBlocker == nil {
		m.status = "回答资格核验失败：" + msg.err.Error()
		return m, nil
	}
	return m.prepareDecisionAnswer(true)
}
