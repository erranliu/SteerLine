import type { Candidate } from "../../shared/api/types";
export function sameNotice(a?: string, b?: string) {
  return (
    !!a?.trim() &&
    a.replace(/\r\n/g, "\n").trim() === b?.replace(/\r\n/g, "\n").trim()
  );
}

// Compare display copies only; retain the original text in the owner and editor.
export function candidateNotices(
  run: Candidate,
  answerVisible = !!run.decision,
) {
  const texts = [
    run.runner_delivery?.diagnostic,
    run.runner_delivery?.controls.reason,
    run.recovery_required ? "原执行需要恢复核验。" : undefined,
    run.recovery_required ? run.stage_diagnostic || run.error : undefined,
    run.environment_blocker
      ? run.environment_blocker.fact.Reason +
        " · " +
        run.environment_blocker.fact.Detail
      : undefined,
    run.error && (!run.recovery_required || run.stage_diagnostic)
      ? run.error
      : undefined,
  ];
  const seen = answerVisible
    ? [run.decision?.request.question, run.decision?.request.context]
    : [];
  if (answerVisible && run.decision?.request.interaction_kind === "runner_resume")
    seen.push(
      run.runner_delivery?.commands
        ?.filter((command) => command.command === "resume")
        .at(-1)?.diagnostic,
    );
  return texts.filter((text): text is string => {
    if (!text || seen.some((s) => sameNotice(s, text))) return false;
    seen.push(text);
    return true;
  });
}
const stages: Record<string, string> = {
  start: "准备执行",
  prepare: "准备执行",
  input: "准备输入",
  implement: "实施中",
  test: "测试验证",
  tests: "测试验证",
  audit: "审查中",
  review: "审查中",
  repair: "修复中",
  report: "整理报告",
  publish: "发布中",
  await_merge: "等待合并",
  closed: "执行已结束",
  completed: "执行已结束",
};
export function stageLabel(stage?: string) {
  return stages[stage?.split(".")[0] ?? ""] ?? "阶段待核实";
}
export function currentStage(run: Candidate) {
  return stageLabel(
    run.runner_delivery?.available && run.runner_delivery.step
      ? run.runner_delivery.step
      : run.workflow_stage,
  );
}
export function executionEnded(run: Candidate) {
  const terminal = [
    "completed",
    "succeeded",
    "failed",
    "cancelled",
    "stopped",
    "closed",
  ];
  return (
    terminal.includes(run.execution_status) ||
    (!!run.runner_delivery?.available &&
      (run.runner_delivery.closed ||
        terminal.includes(run.runner_delivery.native_outcome ?? "")))
  );
}

// Command outcomes are independent of the Run status and browser journal.
export function commandIssues(run: Candidate): string[] {
  const names: Record<string, string> = {
    submit: "提交",
    reply: "回答",
    message: "发送消息",
    resume: "恢复",
    stop: "停止",
    interrupt: "中断",
    close: "关闭会话",
  };
  const issues = new Set<string>();
  const commands = run.runner_delivery?.commands ?? [];
  const latestResume = commands.filter((command) => command.command === "resume").at(-1);
  for (const command of commands) {
    if (command.command === "resume" && command !== latestResume) continue;
    const name = names[command.command] || "执行";
    const states = [command.receipt_state, command.action_status];
    if (
      states.some((s) =>
        ["unknown", "outcome_unknown", "reconciling"].includes(s ?? ""),
      )
    )
      issues.add(`${name}操作结果待核实，请继续核实原操作。`);
    if (states.some((s) => ["failed", "rejected"].includes(s ?? "")))
      issues.add(`${name}操作未成功，请核对原操作结果。`);
  }
  return [...issues];
}
