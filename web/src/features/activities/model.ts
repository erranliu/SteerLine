import type { Envelope, Receipt, StoredRequest } from "../../shared/api/types";
import type { CandidateStart } from "../../app/use-candidate-starts";
import type { ExecutionReference } from "./execution-info";

export type ActivityFilter = "ongoing" | "attention" | "recent";
export type ActivityListFilter = ActivityFilter | "history";
export type Activity = {
  activity_id: string;
  project_id: string;
  source: string;
  source_id: string;
  kind: string;
  steer_agent?: { type: string; operation: string };
  request_ids: string[];
  execution_ref?: ExecutionReference;
  title: string;
  state: string;
  phase?: string;
  summary?: string;
  diagnostic?: string;
  needs_attention: boolean;
  created_at?: string;
  finished_at?: string;
  source_observed_at?: string;
  observation: string;
  provisional?: boolean;
  message?: {
    kind: string;
    text: string;
    at?: string;
    observed_at?: string;
    truncated: boolean;
    diagnostic?: string;
  };
  progress?: {
    label: string;
    total: number;
    succeeded: number;
    failed: number;
    pending: number;
    diagnostic?: string;
  };
  details_ref: {
    kind: string;
    target: string;
    task_id?: string;
    candidate_id?: string;
    historical?: boolean;
  };
};
export type ActivityPage = {
  project_id: string;
  instance_id: string;
  sampled_at: string;
  items: Activity[];
  request_links: Record<string, Activity>;
  counts: Record<ActivityFilter, number> & { history?: number };
  counts_known: boolean;
  sources: { source: string; complete: boolean; diagnostic?: string }[];
  next_cursor?: string;
  has_more: boolean;
};
export type ActivityLinks = Pick<
  ActivityPage,
  "project_id" | "instance_id" | "sampled_at" | "request_links" | "sources"
>;
export const ended = (a: Activity) =>
  ["succeeded", "failed", "cancelled"].includes(a.state);
export function steerAgentLabel(a: Activity): string | undefined {
  switch (a.steer_agent?.type) {
    case "material":
      return "材料生成";
    case "continuation":
      return "对话续作";
    case "planning":
      return "Roadmap 规划";
    case "maintenance":
      return "持续维护";
  }
}
export const matchesActivity = (a: Activity, filter: ActivityFilter) =>
  filter === "attention"
    ? a.needs_attention
    : filter === "recent"
      ? ended(a)
      : !ended(a);
export const activityActions = new Set([
  "environment_start",
  "maintenance_check",
  "start_task",
  "start_candidate",
  "roadmap_parse",
  "roadmap_generate_documents",
  "roadmap_publish",
  "roadmap_admit",
  "roadmap_resume",
]);

// Existing starts and journal rows are observations. This function cannot send
// a command, create an envelope, or turn an old-instance request into new work.
export function provisionalActivities(
  project: string,
  instance: string,
  requests: StoredRequest[],
  starts: CandidateStart[],
  links: Record<string, Activity>,
  official: Activity[],
) {
  const known = new Set(
    [...official, ...Object.values(links)].flatMap((a) => [
      a.source_id,
      ...a.request_ids,
    ]),
  );
  const result = new Map<string, Activity>();
  for (const r of requests) {
    const e = r.envelope;
    if (
      e.project_id !== project ||
      e.instance_id !== instance ||
      !activityActions.has(e.action) ||
      known.has(e.request_id)
    )
      continue;
    const rejected = r.response?.status === "rejected";
    result.set(e.request_id, {
      activity_id: "provisional:" + instance + ":" + e.request_id,
      source_id: e.request_id,
      project_id: project,
      source: "provisional",
      kind: "submission",
      request_ids: [e.request_id],
      provisional: true,
      title:
        e.action === "roadmap_parse"
          ? "提交 Roadmap 草稿解析"
          : e.action === "roadmap_generate_documents"
            ? "提交 Spec / Plan / 验收文档生成"
            : e.action === "roadmap_publish"
              ? "提交 Roadmap 文档发布"
              : e.action === "roadmap_admit"
                ? "提交 Roadmap 入图"
                : e.action === "roadmap_resume"
                  ? "继续原 Roadmap 操作"
                  : e.action === "environment_start"
                    ? "提交环境检测"
                    : e.action.startsWith("maintenance_")
                      ? "提交项目准备操作"
                      : "启动任务候选",
      state: rejected ? "failed" : "waiting",
      needs_attention: false,
      created_at: r.created_at,
      phase: rejected
        ? "未启动"
        : r.response?.status === "applied"
          ? "等待工作信息"
          : r.error || r.response?.status === "outcome_unknown"
            ? "提交结果待核实"
            : "正在提交",
      diagnostic: r.response?.diagnostic || r.error,
      observation: "unverified",
      details_ref: {
        kind:
          e.action === "roadmap_parse"
            ? "roadmap-source"
            : e.action.startsWith("roadmap_")
              ? "roadmap"
              : e.action === "environment_start"
                ? "environment-profile"
                : e.action.startsWith("maintenance_")
                  ? "maintenance"
                  : "task",
        target:
          e.action === "environment_start"
            ? String(e.payload?.profile_id || "")
            : e.target_id,
      },
    });
  }
  for (const s of starts) {
    if (known.has(s.id)) continue;
    const previous = result.get(s.id);
    result.set(s.id, {
      ...previous,
      activity_id: "provisional:" + instance + ":" + s.id,
      source_id: s.id,
      project_id: project,
      source: "provisional",
      kind: "submission",
      request_ids: [s.id],
      provisional: true,
      title: `任务 #${s.taskID} · ${s.profile || "候选"}`,
      state: s.rejected || !s.participating ? "failed" : "waiting",
      phase: s.status,
      diagnostic: s.diagnostic,
      needs_attention: false,
      observation: "unverified",
      details_ref: { kind: "task", target: s.taskID },
    });
  }
  return [...result.values()];
}

export function activityDestination(project: string, activity: Activity) {
  const root = `/projects/${encodeURIComponent(project)}`;
  const d = activity.details_ref;
  if (d.kind === "advisory")
    return `${root}/runs?task=${encodeURIComponent(d.task_id || "")}&run=${encodeURIComponent(d.candidate_id || "")}&advisory=${encodeURIComponent(d.target)}`;
  if (d.kind === "roadmap")
    return `${root}/roadmaps?record=${encodeURIComponent(d.target)}${activity.execution_ref?.action && activity.execution_ref.request_id ? `&request_id=${encodeURIComponent(activity.execution_ref.request_id)}` : ""}`;
  if (d.kind === "roadmap-source")
    return `${root}/roadmaps?source=${encodeURIComponent(d.target)}`;
  if (d.kind === "candidate")
    return `${root}/runs?task=${encodeURIComponent(d.task_id || "")}&run=${encodeURIComponent(d.target)}`;
  if (d.kind === "task")
    return `${root}/runs?task=${encodeURIComponent(d.target)}`;
  if (d.kind === "environment-profile")
    return `${root}/environment${d.target ? `?profile=${encodeURIComponent(d.target)}` : ""}`;
  if (d.kind === "environment")
    return `${root}/environment?activity=${encodeURIComponent(d.target)}`;
  return `${root}/dag?maintenance=preparation&request_id=${encodeURIComponent(d.target)}#project-maintenance`;
}

export function requestDestination(e: Envelope, receipt?: Receipt) {
  const root = `/projects/${encodeURIComponent(e.project_id)}`;
  const request = `request_id=${encodeURIComponent(e.request_id)}`;
  const record = (receipt?.roadmap || receipt?.environment) as
    { id?: string } | undefined;
  if (e.action.startsWith("roadmap_")) {
    if (record?.id)
      return `${root}/roadmaps?record=${encodeURIComponent(record.id)}&${request}`;
    if (e.action === "roadmap_parse")
      return `${root}/roadmaps?source=${encodeURIComponent(e.target_id)}&${request}`;
    if (e.action === "roadmap_prepare_static")
      return `${root}/roadmaps?${request}`;
    return `${root}/roadmaps?record=${encodeURIComponent(e.target_id)}&${request}`;
  }
  if (e.action.startsWith("environment_")) {
    const id =
      record?.id || (e.action === "environment_stop" ? e.target_id : "");
    return id
      ? `${root}/environment?activity=${encodeURIComponent(id)}&${request}`
      : undefined;
  }
  if (e.action.startsWith("maintenance_"))
    return `${root}/dag?maintenance=preparation&${request}#project-maintenance`;
  if (["start_task", "start_candidate"].includes(e.action))
    return `${root}/runs?task=${encodeURIComponent(e.target_id)}&run=${encodeURIComponent(e.request_id)}`;
  return undefined;
}
