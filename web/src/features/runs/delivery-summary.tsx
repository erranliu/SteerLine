import type { Candidate, Task } from "../../shared/api/types";
import { Link } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { SafeLink } from "../../shared/ui/status";
import { executionEnded } from "./presentation";
const names: Record<string, string> = {
  accepted: "已受理",
  pending: "待处理",
  unknown: "未核实",
  outcome_unknown: "结果未知",
  rejected: "已拒绝",
  cancelled: "已取消",
  running: "执行中",
  reconciling: "核实中",
  waiting_input: "等待输入",
  blocked: "已阻断",
  pr_ready: "PR 已就绪",
  completed: "本轮完成",
  succeeded: "本轮成功",
  failed: "失败",
  unreported: "未提供结论",
  settling: "资源结算中",
  stopped: "已停止",
  closed: "已关闭",
  open: "待合并",
  merged: "已合并",
  closed_unmerged: "关闭未合并",
};
export const deliveryLabel = (value?: string) =>
  value ? names[value] || "待核实" : "未核实";
export function DeliverySummary({
  run,
  task,
  stale = false,
}: {
  run: Candidate;
  task?: Task;
  stale?: boolean;
  sampledAt?: string;
}) {
  const v = run.runner_delivery?.available ? run.runner_delivery : undefined;
  const pr = run.pr.state && run.pr.state !== "none" ? run.pr.state : "";
  const conflict = !!pr && !!v?.pr_state && pr !== v.pr_state;
  const state = conflict
    ? "PR 状态待核实"
    : task?.delivery
      ? "任务已交付"
      : (pr || v?.pr_state) === "merged"
        ? "已合并，交付待核实"
        : deliveryLabel(
            pr ||
              v?.pr_state ||
              (v?.status === "pr_ready" ? "pr_ready" : undefined),
          );
  return (
    <div aria-label="交付摘要">
      {stale && <p className="notice amber">上次已知交付状态 · 当前待核实</p>}
      <p>{task?.delivery ? "任务已交付" : state}</p>
      <SafeLink url={task?.delivery?.pr_url || run.pr.url}>
        {task?.delivery?.pr_url || run.pr.url ? "查看 PR" : "尚未观察到 PR"}
      </SafeLink>
      {v?.manual_verification && (
        <ManualVerificationLink candidate={run.run_id} />
      )}
      {conflict && (
        <p className="notice amber">
          PR 观察不一致：{deliveryLabel(pr)} / {deliveryLabel(v?.pr_state)}
        </p>
      )}
      {(pr || v?.pr_state) && (
        <p className="muted">
          PR 缓存观察 ·{" "}
          {v?.pr_checked_at ? (
            <time dateTime={v.pr_checked_at}>
              上次核实于{" "}
              {new Date(v.pr_checked_at).toLocaleString("zh-CN", {
                hour12: false,
              })}
            </time>
          ) : (
            "核实时间未知"
          )}{" "}
          · 非实时状态
        </p>
      )}
      {(run.pr.query_error || v?.pr_query_error) && (
        <p className="notice amber">
          PR 查询未核实 ·{" "}
          {[
            ...new Set([run.pr.query_error, v?.pr_query_error].filter(Boolean)),
          ].join("；")}
        </p>
      )}
      {v?.available && executionEnded(run) && (
        <p>
          原生执行 · {deliveryLabel(v.native_outcome)} · 最终输出
          {v.final_verified ? "已核实" : "未核实"}
        </p>
      )}
      {v?.content_status && <p>业务结果 · {deliveryLabel(v.content_status)}</p>}
      {(executionEnded(run) ||
        run.stop_requested ||
        (v?.stop_status && v.stop_status !== "active")) && (
        <p>
          资源停止 ·{" "}
          {v?.stop_status ? deliveryLabel(v.stop_status) : "未取得停止证明"}
        </p>
      )}
      {v?.stop_status === "stopped" && (
        <p className="muted">停止范围以 Runner 原回执为准。</p>
      )}
      {v?.closed && (
        <p className="muted">会话已关闭；业务结果与资源停止分别判断。</p>
      )}
    </div>
  );
}

function ManualVerificationLink({ candidate }: { candidate: string }) {
  const { identity } = useWorkspace();
  return (
    <p>
      <Link
        to={`/projects/${identity.project_id}/manual-verification?candidate=${encodeURIComponent(candidate)}`}
      >
        人工核验
      </Link>
    </p>
  );
}
