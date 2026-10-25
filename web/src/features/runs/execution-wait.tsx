import { useEffect, useMemo, useState } from "react";
import type { Candidate, ExecutionWait } from "../../shared/api/types";
import { usePageVisible } from "../../shared/query-visibility";

export function preparationWait(run: Candidate): ExecutionWait | null {
  const view = run.runner_delivery;
  if (!view?.available || view.closed || run.stop_requested || (view.stop_status && view.stop_status !== "active") ||
      view.submission === "unknown" || view.commands.some((command) =>
        ["unknown", "outcome_unknown"].includes(command.receipt_state) ||
        ["unknown", "outcome_unknown", "reconciling"].includes(command.action_status ?? "")) ||
      run.participation !== "active" || run.environment_blocker ||
      ["closed", "completed", "discarded"].includes(run.task_status) ||
      run.decision?.status === "pending" && run.decision.request.interaction_kind !== "runner_resume") return null;
  const wait = view.execution_wait;
  if (!wait || !["queued", "running", "preparing", "blocked"].includes(view.status ?? "") ||
      view.status === "blocked" && wait.stage !== "exhausted") return null;
  return wait;
}

export function executionWaitTitle(wait: ExecutionWait): string {
  switch (wait.stage) {
    case "queue": return "等待 Orca 调用通道";
    case "calling": return wait.query === "repository_inventory" ? "正在查询 Orca 仓库" : "正在查询 Orca 运行状态";
    case "backoff": return wait.query === "repository_inventory" ? "正在重试仓库查询" : "正在重试运行状态查询";
    case "exhausted": return wait.reason_code === "query_attempts_exhausted" ? "查询重试已耗尽" : "等待通道超时";
  }
}

export function ExecutionWaitSummary({ wait, stale = false, sampledAt }: {
  wait: ExecutionWait; stale?: boolean; sampledAt?: string;
}) {
  const visible = usePageVisible();
  const [, tick] = useState(0);
  const anchor = useMemo(() => ({ source: sampledAt ? Date.parse(sampledAt) : NaN, monotonic: performance.now() }),
    [sampledAt, wait.operation_id, wait.updated_at]);
  const clocked = Number.isFinite(anchor.source);
  const active = wait.stage !== "exhausted" && !stale;
  useEffect(() => {
    if (!active || !visible || !clocked) return;
    const timer = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(timer);
  }, [active, visible, clocked, anchor]);
  const now = active ? anchor.source + Math.max(0, performance.now() - anchor.monotonic) : Date.parse(wait.updated_at);
  const elapsed = Math.max(0, Math.floor((now - Date.parse(wait.started_at)) / 1000));
  const deadlineReached = clocked && now >= Date.parse(wait.deadline_at);
  const next = wait.next_attempt_at ? Math.max(0, Math.ceil((Date.parse(wait.next_attempt_at) - now) / 1000)) : null;
  const localTime = (at: string) => new Date(at).toLocaleString("zh-CN", { hour12: false });
  return (
    <section aria-label="准备查询等待" className="notice amber">
      <strong>{executionWaitTitle(wait)}</strong>
      {stale ? <p>等待信息暂时不可核实，以下为最后已知状态。</p> : <>
        {clocked && <p>本次查询已等待 {elapsed} 秒</p>}
        {active && (deadlineReached || wait.stage === "backoff" && next === 0) ? <p>等待 Runner 更新</p> : <>
          {wait.stage === "queue" && <p>Runner 将自动继续。</p>}
          {wait.stage === "backoff" && <p>已尝试 {wait.calls_started}/{wait.max_calls} 次{clocked && next !== null ? `，约 ${next} 秒后再次查询` : ""}。</p>}
        </>}
        {wait.stage === "exhausted" && <p>本次自动等待已结束，后续操作以当前执行资格为准。</p>}
      </>}
      <details>
        <summary>等待详情</summary>
        <p>原准备操作 {wait.operation_id} · 查询 {wait.query}</p>
        <p>开始于 <time dateTime={wait.started_at}>{localTime(wait.started_at)}</time></p>
        <p>预算截止 <time dateTime={wait.deadline_at}>{localTime(wait.deadline_at)}</time>；此时间不是完成预估。</p>
        <p>调用尝试 {wait.calls_started}/{wait.max_calls}，排队不计调用；此计数不代表模型重跑。</p>
        {wait.next_attempt_at && <p>计划读取于 <time dateTime={wait.next_attempt_at}>{localTime(wait.next_attempt_at)}</time></p>}
        <p>{wait.diagnostic}</p>
      </details>
    </section>
  );
}
