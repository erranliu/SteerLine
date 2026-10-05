package console

import (
	"fmt"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"strings"
	"time"
)

func renderSession(state projectState, row *consoleRow, width, height int, now time.Time) []string {
	lines := []string{styleLabel.Render("会话 · Turn / Session")}
	if row == nil || row.run == nil {
		return append(lines, "选择一个 run 后查看会话。")
	}
	lines = append(lines, candidateLabel(state, row.run)+" · session "+shortSession(row.run.SessionID))
	if state.conversationError != "" {
		lines = append(lines, styleAlert.Render("会话读取错误："+safeTerminalText(state.conversationError)))
	}
	view, changed := BuildConversationView(state.conversationEvents, state.view.Groups)
	_ = changed
	if len(state.conversationEvents) == 0 {
		if len(state.activityEvents) > 0 {
			lines = append(lines, "能力降级：执行过程 / 执行摘要 · 来源 activity journal；尚未验证精确 turn/final。")
		} else {
			lines = append(lines, "没有可读会话记录。查看 activity 页确认执行日志状态。")
		}
	}
	if len(view.Groups) > 0 {
		lines = append(lines, "来源：版本化会话事件 · [ ] 切 attempt · e 折叠/展开 · = 保持展开 · n 阅读最终回复")
		for _, group := range view.Groups {
			choice := state.view.Groups[group.ID]
			heading := groupHeading(group, choice)
			if !group.Precise {
				heading = "执行过程 · turn 边界未知 · " + heading
			}
			if group.ID == state.view.AnchorGroup {
				heading = "● " + heading
			}
			lines = append(lines, safeTerminalText(heading))
			if !Folded(group, choice) {
				for _, event := range group.Process {
					lines = append(lines, renderConversationEvent(event, width-2)...)
				}
			}
			for _, event := range group.Finals {
				lines = append(lines, renderFinalEventLine(state, event))
			}
			if group.EndStatus != "" {
				switch group.FinalState {
				case api.FinalPending:
					lines = append(lines, "最终回复待同步")
				case api.FinalAbsent:
					lines = append(lines, "本轮已结束，未识别到最终回复")
				case api.FinalUnknown:
					lines = append(lines, "最终回复状态未知")
				}
				reason := group.EndReason
				if reason == "" {
					reason = "来源未提供终态原因"
				}
				lines = append(lines, "终态原因："+safeTerminalText(reason))
			}
		}
	}
	for _, event := range view.Summaries {
		lines = append(lines, "执行摘要 · "+safeTerminalText(event.Status)+" · "+safeTerminalText(event.Source)+" · "+safeTerminalText(event.ItemID))
	}
	for _, warning := range view.Warnings {
		lines = append(lines, styleAlert.Render("轮次外提示 · "+safeTerminalText(firstText(warning.Diagnostic, warning.Status, string(warning.Type)))))
	}
	if row.run.Decision != nil && row.run.Decision.Status == "pending" {
		if feedbackInProgress(state, row.run) {
			lines = append(lines, candidateHumanStatus(state, row.run))
		} else {
			lines = append(lines, "需要你的回复 · a/Enter 打开交互", "问题："+safeTerminalText(row.run.Decision.Request.Question))
		}
	} else if row.run.Decision != nil {
		lines = append(lines, renderDecisionIdentity(state, row.run, now)...)
		lines = append(lines, "决策阶段："+decisionStatusLabel(row.run.Decision.Status), "当前答案："+safeTerminalText(DecisionAnswerSummary(row.run)))
	}
	if state.conversationMore {
		lines = append(lines, "还有更多会话记录 · 按 y 加载下一页")
	}
	if state.capabilityError != "" {
		lines = append(lines, "能力报告读取失败："+safeTerminalText(state.capabilityError))
	}
	if len(state.capabilities) > 0 {
		for _, report := range state.capabilities {
			lines = append(lines, "能力："+safeTerminalText(string(report.Backend)+" / "+string(report.Protocol)+" / "+report.Agent)+" · 来源 "+safeTerminalText(report.Source+"/"+report.SourceIdentity)+" · 检查 "+report.CheckedAt.Local().Format(timeLayout))
			for _, capability := range report.Capabilities {
				if capability.Status != "verified" {
					lines = append(lines, "  "+safeTerminalText(capability.Name)+" = "+safeTerminalText(string(capability.Status))+" · "+safeTerminalText(capability.Evidence))
				}
			}
		}
		for _, event := range state.conversationEvents {
			if event.Backend == "" || event.Protocol == "" || event.TargetID == "" {
				continue
			}
			matched := false
			for _, report := range state.capabilities {
				if report.Backend == event.Backend && report.Protocol == event.Protocol && report.Target.ID == event.TargetID && report.ExecutionID == event.ExecutionID && report.AttemptID == event.AttemptID && report.Source == event.Source && report.SourceIdentity == event.SourceIdentity {
					matched = true
					break
				}
			}
			if !matched {
				lines = append(lines, "能力原因：来源 "+safeTerminalText(event.Source+"/"+event.SourceIdentity)+" 没有匹配的已验证报告；不推断 turn/final。")
			}
		}
	} else if len(state.conversationEvents) > 0 {
		lines = append(lines, "能力原因：没有匹配当前 execution/attempt 的已验证报告；不推断 turn 能力。")
	}
	return scrollLines(wrapLines(lines, width), state.detailScroll, height)
}

func renderConversationEvent(event api.ConversationEvent, width int) []string {
	identity := firstText(event.ItemID, event.SourceEventID, string(event.Type))
	phase := string(event.Phase)
	status := event.Status
	line := fmt.Sprintf("  [%s] %s · %s · %s · %s", itemToken(event), safeTerminalText(phase), safeTerminalText(string(event.Type)), safeTerminalText(status), safeTerminalText(identity))
	if event.OccurredAt != nil {
		line = "  " + event.OccurredAt.Local().Format("15:04:05") + line
	}
	if event.Body != nil {
		line += " · 正文 " + safeTerminalText(string(event.Body.Completeness)) + fmt.Sprintf(" %d bytes · 按 o 阅读", event.Body.Bytes)
	}
	if len(line) > width {
		line = truncate(line, width)
	}
	return []string{line}
}

func renderFinalEventLine(state projectState, event api.ConversationEvent) string {
	key := conversationItemKey(event)
	line := "  [" + itemToken(event) + "] final_answer · " + safeTerminalText(firstText(event.ItemID, event.SourceEventID)) + " · 正文缺失"
	if event.Body != nil {
		line = fmt.Sprintf("  [%s] final_answer · %s · %s · %d bytes · 按 n 阅读", itemToken(event), safeTerminalText(firstText(event.ItemID, event.SourceEventID)), safeTerminalText(string(event.Body.Completeness)), event.Body.Bytes)
	}
	if state.bodyEvent == key && state.bodyText != "" {
		line += " · 已打开"
	}
	return line
}

func renderFinal(state projectState, row *consoleRow, width, height int) []string {
	title := "最终回复 · 纯文本正文"
	if state.bodyKind == "process" {
		title = "会话消息正文 · 纯文本"
	}
	lines := []string{styleLabel.Render(title)}
	if row == nil || row.run == nil {
		return append(lines, "选择一个 run 后阅读正文。")
	}
	if state.bodyRef == nil {
		return append(lines, "按 n 选择并读取最终回复。过程消息不会转成 final。", "")
	}
	lines = append(lines, fmt.Sprintf("execution %s · attempt %s · body %s r%d · offset %d/%d", safeTerminalText(state.bodyRef.ExecutionID), safeTerminalText(state.bodyRef.AttemptID), safeTerminalText(state.bodyRef.ItemKey), state.bodyRef.Revision, state.bodyViewOffset, state.bodyRef.Bytes))
	if state.bodyError != "" {
		lines = append(lines, styleAlert.Render("正文读取错误："+safeTerminalText(state.bodyError)))
	}
	if state.bodyText != "" {
		lines = append(lines, strings.Split(safeTerminalText(state.bodyText), "\n")...)
	} else if state.bodyBusy {
		lines = append(lines, "正在读取正文…")
	}
	if state.bodyMore {
		lines = append(lines, "正文尚有后续 · > 下一页 · < 上一页")
	}
	if state.bodyRef.Completeness != api.BodyComplete {
		lines = append(lines, "正文完整性："+safeTerminalText(string(state.bodyRef.Completeness)))
	}
	return scrollLines(wrapLines(lines, width), state.detailScroll, height)
}
