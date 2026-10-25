import type { Candidate } from "../../shared/api/types";
import { ExecutionWaitSummary, preparationWait } from "./execution-wait";

const names: Record<string, string> = {
  accepted: "已受理",
  pending: "待处理",
  unknown: "未核实",
  outcome_unknown: "结果未知",
  rejected: "已拒绝",
  cancelled: "已取消",
  running: "执行中",
  reconciling: "对账中",
  waiting_input: "等待输入",
  blocked: "已阻断",
  pr_ready: "PR 已就绪",
  completed: "已完成",
  succeeded: "成功",
  failed: "失败",
  unreported: "未提供结论",
  settling: "资源结算中",
  stopped: "已停止",
  closed: "已关闭",
  open: "待合并",
  merged: "已合并",
  closed_unmerged: "关闭未合并",
};
const label = (value?: string) => (value ? names[value] || value : "未核实");

// Pure rendering from one observed Run snapshot. No fetching, command dispatch
// or completion authority is attached to this display.
export function DeliverySummary({ run, stale = false, sampledAt }: { run: Candidate; stale?: boolean; sampledAt?: string }) {
  const v = run.runner_delivery;
  if (!v) return null;
  const wait = preparationWait(run);
  const actions = v.available
    ? [
        v.controls.can_reply && "回答",
        v.controls.can_resume && "恢复",
        v.controls.can_stop && "停止",
        v.controls.can_message && "发送消息",
        v.controls.can_interrupt && "中断本轮",
        v.controls.can_close && "关闭会话",
      ].filter(Boolean)
    : [];
  return (
    <section className="settings-panel" aria-label="交付进度">
      <h3>交付进度</h3>
      {wait && <ExecutionWaitSummary key={run.run_id + "/" + run.execution_id} wait={wait} stale={stale} sampledAt={sampledAt} />}
      <p>请求受理 · {label(v.submission)}</p>
      {v.available ? (
        <>
          <p>
            当前步骤 · {v.step || "未报告"} · {label(v.status)}
          </p>
          <p>
            原生执行 · {label(v.native_outcome)} · 最终输出
            {v.final_verified ? "已核实" : "未核实"}
          </p>
          <p>
            业务结果 ·{" "}
            {v.content_status ? label(v.content_status) : label(v.status)}
          </p>
          <p>PR 观察 · {v.pr_state ? label(v.pr_state) : "尚无已绑定 PR"}</p>
          {v.pr_state && (
            <p className="muted">
              PR 缓存观察 ·{" "}
              {v.pr_checked_at ? (
                <time dateTime={v.pr_checked_at}>
                  上次核实于{" "}
                  {new Date(v.pr_checked_at).toLocaleString("zh-CN", {
                    hour12: false,
                  })}
                </time>
              ) : (
                "未记录查询时间"
              )}
              ；非实时状态，本页刷新不查询外部 PR。
            </p>
          )}
          {v.pr_query_error && (
            <p className="notice amber">PR 查询未核实 · {v.pr_query_error}</p>
          )}
          <p>
            资源停止 · {v.stop_status ? label(v.stop_status) : "未取得停止证明"}
          </p>
          {v.stop_status === "stopped" && (
            <p className="muted">停止范围以 Runner 原回执为准。</p>
          )}
          {v.closed && <p>会话已关闭；业务结果与资源停止分别判断。</p>}
          {v.status === "waiting_input" && <p>待处理 · 等待明确用户输入。</p>}
          {v.resume_required && <p>待处理 · 核验原执行后显式恢复。</p>}
          {v.diagnostic && (
            <p className="notice amber">待核实事项 · {v.diagnostic}</p>
          )}
          <p>当前可用操作 · {actions.length ? actions.join("、") : "暂无"}</p>
          {v.controls.reason && <p className="muted">{v.controls.reason}</p>}
          <p className="muted">
            Run {v.run_id} · 观察 revision {v.revision} · 原始阶段{" "}
            {v.phase || "未报告"}
          </p>
        </>
      ) : (
        <p className="notice amber">
          {v.diagnostic || "尚无可核实的 Run 观察；受理不代表业务完成。"}
        </p>
      )}
      {!!v.commands.length && (
        <details>
          <summary>原命令回执</summary>
          <p className="muted">
            以下为各命令独立回执，命令完成不代表交付或资源结算完成。最多显示提交、最近四条控制及停止。
          </p>
          {v.commands.map((command, i) => (
            <p key={command.request_id + "/" + i}>
              {command.command} · {label(command.receipt_state)} · 动作{" "}
              {label(command.action_status)} · request {command.request_id}
            </p>
          ))}
        </details>
      )}
    </section>
  );
}
