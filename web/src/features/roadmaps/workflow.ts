import type { RoadmapRecord, RoadmapSource } from "./roadmaps";

export const roadmapSourceChanged = (
  saved: RoadmapSource,
  current: RoadmapSource,
) =>
  saved.digest !== current.digest &&
  saved.body.replace(/\r\n/g, "\n") !== current.body.replace(/\r\n/g, "\n");

const pending = new Set(["queued", "accepted", "unknown"]);
export const contentPending = (record: RoadmapRecord) =>
  record.generations?.length
    ? record.generations.some(
        (g) =>
          g.settled === false ||
          (g.settled === undefined && pending.has(g.state)),
      )
    : !record.cancelled && pending.has(record.state);

// A new parse starts a new flow; document attempts belong to their parse.
// Unresolved external work must remain visible even if a newer record exists.
export function currentRoadmapRecord(records: RoadmapRecord[]) {
  const ordered = [...records].sort(
    (a, b) =>
      b.created_at.localeCompare(a.created_at) || b.id.localeCompare(a.id),
  );
  const unresolved = ordered.find(
    (r) =>
      contentPending(r) ||
      (!!r.publication && r.state !== "admitted") ||
      r.state === "admission-unknown",
  );
  if (unresolved) return unresolved;
  const root = ordered.find((r) => r.purpose !== "documents");
  return root
    ? ordered.find((r) => r.id === root.id || r.parent_id === root.id)
    : ordered[0];
}

export const roadmapSteps = [
  "解析草稿",
  "确认任务依赖并生成文档",
  "审阅文档",
  "发布合并",
  "正式入图",
];

const labels: Record<string, string> = {
  review: "待确认任务与依赖",
  blocked: "待澄清",
  invalid: "生成结果无效",
  failed: "内容请求失败",
  rejected: "请求被拒绝",
  cancelled: "内容请求已停止",
  confirmed: "任务与依赖已确认",
  "documents-blocked": "文档生成待澄清",
  "documents-review": "待审阅正式文档",
  publishing: "文档发布中",
  "awaiting-merge": "等待核实合并",
  "awaiting-admission": "文档已合并 · 待入图",
  "admission-unknown": "入图回执待核实",
  admitted: "已正式入图",
};

export function roadmapProgress(record?: RoadmapRecord) {
  if (!record) return { label: "尚未解析", step: 0 };
  const documents = record.purpose === "documents";
  const label = pending.has(record.state)
    ? record.state === "unknown"
      ? documents
        ? "文档生成结果待核实"
        : "解析结果待核实"
      : documents
        ? "生成文档中"
        : "解析草稿中"
    : labels[record.state] || record.state;
  const step =
    record.state === "admitted"
      ? 5
      : ["awaiting-admission", "admission-unknown"].includes(record.state)
        ? 4
        : record.publication ||
            ["publishing", "awaiting-merge"].includes(record.state)
          ? 3
          : record.state === "documents-review"
            ? 2
            : documents || record.state === "confirmed"
              ? 1
              : record.state === "review"
                ? 1
                : 0;
  return { label, step };
}

export function roadmapNextAction(record?: RoadmapRecord, restart = false) {
  if (!record || restart) return "parse";
  if (contentPending(record))
    return record.state === "unknown" ? "observe" : "wait";
  if (
    (record.publication &&
      !["admitted", "awaiting-admission"].includes(record.state)) ||
    record.state === "admission-unknown"
  )
    return "resume";
  switch (record.state) {
    case "review":
    case "confirmed":
      return "generate";
    case "documents-review":
      return "publish";
    case "awaiting-admission":
      return "admit";
    case "admitted":
      return "dag";
    case "failed":
    case "invalid":
    case "rejected":
    case "cancelled":
      return record.purpose === "documents"
        ? record.generations?.length && !record.cancelled
          ? "observe"
          : "generate"
        : "parse";
    case "blocked":
    case "documents-blocked":
      return "parse";
    default:
      return "observe";
  }
}

export function roadmapGenerationLabel(
  g: NonNullable<RoadmapRecord["generations"]>[number],
  cancelled: boolean,
) {
  if (cancelled) return g.settled ? "已取消并结算" : "停止收尾中";
  if (g.state === "accepted") {
    if (g.resume_required) return "执行需要恢复核实";
    return (
      {
        queued: "排队中",
        running: "生成中",
        blocked: "生成受阻，需处理",
        waiting_input: "等待输入处理",
        waiting_external: "等待外部结果",
        completed: "结果收尾中",
        failed: "结果收尾中",
        stopped: "结果收尾中",
        closed: "结果收尾中",
      }[g.runner_status || ""] || "执行状态待核实"
    );
  }
  return (
    {
      queued: "排队中",
      unknown: "提交待核实",
      "documents-review": "生成完成",
      failed: "生成失败",
      invalid: "结果校验失败",
      rejected: "提交被拒绝",
      "documents-blocked": "需要重新规划",
    }[g.state] || g.state
  );
}
