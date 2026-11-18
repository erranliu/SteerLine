import { SafeLink } from "../../shared/ui/status";
import type { MaintenanceRecord } from "./refactoring-maintenance";

export const finishActions = {
  early_finish: {
    label: "提前结束并创建 PR",
    explanation:
      "停止开启新轮次，完成已有成果的验证和发布。没有有效改动时保存调查结论。",
  },
  resume_delivery: {
    label: "继续交付 PR",
    explanation: "继续原任务的验证和交付，使用已有成果创建或更新 PR。",
  },
  retry_publish: {
    label: "重试创建 PR",
    explanation: "从已有成果重试交付。",
  },
  retry_conflict: {
    label: "重试更新 PR",
    explanation: "继续处理原 PR 的冲突并重新验证。",
  },
  verify_pr: {
    label: "重新核验 PR",
    explanation: "重新查询已有发布结果，核验通过后继续评审。",
  },
  verify_publication: {
    label: "重新核验 PR",
    explanation: "原发布结果尚未核实，重新读取原执行结果以确认 PR。",
  },
  retry_review: {
    label: "重试评审",
    explanation: "重新核验原 PR 的当前版本并完成评审。",
  },
};

export type FinishAction = keyof typeof finishActions;
export type DeliveryStage = "" | "publish" | "verify" | "review" | "complete";

export function DeliveryResult({ record }: { record: MaintenanceRecord }) {
  if (record.phase === "stopping") return null;
  const report = record.reports?.at(-1);
  const completed = record.phase === "delivered";
  const stopped = record.phase === "stopped";
  const failed = record.phase === "blocked";
  const reviewCurrent =
    completed &&
    !record.report_stale &&
    report &&
    (!record.pr?.head_revision || record.pr.head_revision === report.head) &&
    (!record.pr?.base_revision || record.pr.base_revision === report.base);
  let title = "正在验证并创建 PR";
  if (stopped) title = "已停止";
  else if (failed) title = record.pr ? "PR 交付尚未完成" : "交付失败";
  else if (record.pr?.merged) title = "PR 已合并";
  else if (record.pr?.state && record.pr.state !== "open") title = "PR 已关闭";
  else if (completed) title = record.pr ? "已创建 PR" : "已完成 · 无需修改";
  else if (record.pr) title = "PR 已创建，正在评审";
  else if (!record.delivery_stage) return null;

  if (failed && !record.delivery_stage && !record.pr) return null;
  return (
    <section className="refactoring-result" aria-label="交付结果">
      <h3>{title}</h3>
      {record.pr && (
        <>
          <h4>
            {record.pr.number ? `PR #${record.pr.number}` : "PR"}
            {record.pr.title && ` · ${record.pr.title}`}
          </h4>
          <div className="refactoring-result-link">
            <SafeLink url={record.pr.url}>查看 PR</SafeLink>
          </div>
        </>
      )}
      {(record.delivery_summary || record.summary) && (
        <p className="refactoring-pre">
          {record.delivery_summary || record.summary}
        </p>
      )}
      {stopped && !record.finish_action && !record.pr && (
        <p>没有已接受的成果可交付。</p>
      )}
      {reviewCurrent ? (
        <div>
          <p>
            {report.recommendation === "merge"
              ? "评审建议合入"
              : report.recommendation === "revise"
                ? "评审建议修改后再审"
                : "评审证据不足"}
          </p>
          <details>
            <summary>查看评审结论</summary>
            <pre tabIndex={0}>{report.body}</pre>
          </details>
        </div>
      ) : (
        record.pr &&
        !record.pr.merged &&
        (!record.pr.state || record.pr.state === "open") && (
          <p className="muted">当前 PR 版本的评审待完成。</p>
        )
      )}
    </section>
  );
}
