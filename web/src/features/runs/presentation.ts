import type { Candidate } from "../../shared/api/types";
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
  for (const command of run.runner_delivery?.commands ?? []) {
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
