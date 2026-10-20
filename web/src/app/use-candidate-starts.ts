import { useEffect, useRef, useState } from "react";
import type { Api } from "../shared/api/client";
import { requestByID } from "../shared/api/journal";
import type {
  Envelope,
  Profile,
  Receipt,
  StoredRequest,
  Task,
  TaskPage,
} from "../shared/api/types";

export interface CandidateStart {
  id: string;
  taskID: string;
  title: string;
  profile: string;
  status: string;
  diagnostic?: string;
  participating: boolean;
}
interface LocalStart {
  id: string;
  task: Task;
  profile: Profile;
  graphRevision: number;
  controlRevision: number;
  envelope?: Envelope;
  first?: Envelope;
  receipt?: Receipt;
  error?: string;
  unsent?: boolean;
}
export interface CandidateStarts {
  items: CandidateStart[];
  start: (
    task: Task,
    profile: Profile,
    graphRevision: number,
    controlRevision: number,
  ) => string | undefined;
}

// Only the initial claim needs ordering. Sent commands keep their immutable
// journal envelopes; this hook never retries or changes an already sent start.
export function useCandidateStarts(
  api: Api,
  online: boolean,
  requests: StoredRequest[] | undefined,
  submit: (envelope: Envelope) => Promise<Receipt | undefined>,
): CandidateStarts {
  const [local, setLocal] = useState<LocalStart[]>([]);
  const current = useRef({ api, online, entries: local });
  if (current.current.api !== api)
    current.current = { api, online, entries: [] };
  current.current.online = online;
  useEffect(() => {
    current.current.online = online;
    return () => {
      current.current.online = false;
    };
  }, [api, online]);
  const resolving = useRef(new Set<string>());
  const update = (id: string, patch: Partial<LocalStart>) => {
    if (current.current.api !== api) return;
    current.current.entries = current.current.entries.map((entry) =>
      entry.id === id ? { ...entry, ...patch } : entry,
    );
    setLocal(current.current.entries);
  };
  const send = async (entry: LocalStart, task: Task, candidate: boolean) => {
    if (current.current.api !== api || !current.current.online) return;
    const envelope = {
      ...api.envelope(
        candidate ? "start_candidate" : "start_task",
        task.task_id,
        task.version,
        { execution_profile_id: entry.profile.id },
        {
          expected_control_revision: entry.controlRevision,
          ...(candidate
            ? { expected_graph_revision: entry.graphRevision }
            : {}),
        },
      ),
      request_id: entry.id,
    };
    update(entry.id, { envelope, first: undefined });
    try {
      const receipt = await submit(envelope);
      if (receipt) update(entry.id, { receipt });
      else {
        const stored = await requestByID(entry.id);
        update(entry.id, {
          unsent: !stored,
          error: stored?.error || "提交结果待核实，请查询原请求",
        });
      }
    } catch (error) {
      // Storage failure proves unsent only when the original row is absent.
      let unsent = false;
      try {
        unsent = !(await requestByID(entry.id));
      } catch {
        /* preserve unknown */
      }
      update(entry.id, { unsent, error: String(error) });
    }
  };
  const rows = (requests ?? []).filter(
    (row) =>
      row.envelope.project_id === api.identity.project_id &&
      ["start_task", "start_candidate"].includes(row.envelope.action),
  );
  const entries = current.current.entries;
  const receiptFor = (entry: LocalStart) => {
    const saved = rows.find(
      (row) => row.envelope.request_id === entry.id,
    )?.response;
    return saved?.status === "applied" || saved?.status === "rejected"
      ? saved
      : entry.receipt || saved;
  };
  useEffect(() => {
    if (!online) return;
    for (const entry of entries) {
      if (!entry.first || entry.error || resolving.current.has(entry.id))
        continue;
      const first = entries.find(
        (other) => other.id === entry.first!.request_id,
      );
      const receipt = first
        ? receiptFor(first)
        : rows.find(
            (row) => row.envelope.request_id === entry.first!.request_id,
          )?.response;
      if (receipt?.status === "rejected" || first?.unsent) {
        update(entry.id, {
          error: "首次启动未成功，本候选未提交",
          unsent: true,
        });
        continue;
      }
      if (receipt?.status !== "applied") continue;
      resolving.current.add(entry.id);
      void (async () => {
        try {
          const page = await api.get<TaskPage>(
            "/tasks/" + encodeURIComponent(entry.task.task_id),
          );
          if (current.current.api !== api || !current.current.online) return;
          const task = page.tasks.find(
            (task) => task.task_id === entry.task.task_id,
          );
          if (
            !task ||
            task.status !== "in_progress" ||
            task.version !== entry.first!.expected_version + 1 ||
            page.graph_revision !== entry.graphRevision ||
            !task.workflow_runs.some(
              (run) => run.workflow_run_id === entry.first!.request_id,
            )
          ) {
            throw new Error("任务或依赖已变化，请核对后重新选择方案");
          }
          await send(entry, task, true);
        } catch (error) {
          update(entry.id, { error: String(error), unsent: true });
        } finally {
          resolving.current.delete(entry.id);
        }
      })();
    }
  }, [api, online, requests, local]);

  const items: CandidateStart[] = rows.map((row) => ({
    id: row.envelope.request_id,
    taskID: row.envelope.target_id,
    title: "",
    profile: String(row.envelope.payload.execution_profile_id || ""),
    status:
      row.response?.status === "rejected"
        ? "启动被拒绝"
        : row.response?.status === "applied"
          ? "等待执行信息"
          : row.error || row.response?.status === "outcome_unknown"
            ? "提交结果待核实"
            : "正在提交",
    diagnostic: row.response?.diagnostic || row.error,
    participating: row.response?.status !== "rejected",
  }));
  for (const entry of entries) {
    const receipt = receiptFor(entry);
    const view: CandidateStart = {
      id: entry.id,
      taskID: entry.task.task_id,
      title: entry.task.title,
      profile: entry.profile.name,
      status: entry.unsent
        ? "未提交"
        : receipt?.status === "rejected"
          ? "启动被拒绝"
          : entry.first
            ? "等待首次启动确认"
            : receipt?.status === "applied"
              ? "等待执行信息"
              : entry.error || receipt?.status === "outcome_unknown"
                ? "提交结果待核实"
                : "正在提交",
      diagnostic:
        receipt?.status === "applied"
          ? undefined
          : receipt?.diagnostic || entry.error,
      participating: !entry.unsent && receipt?.status !== "rejected",
    };
    const index = items.findIndex((item) => item.id === entry.id);
    if (index < 0) items.push(view);
    else items[index] = view;
  }
  return {
    items,
    start(task, profile, graphRevision, controlRevision) {
      if (
        !online ||
        task.delivery ||
        !["open", "in_progress"].includes(task.status)
      )
        return;
      const known = new Set(
        task.workflow_runs.map((run) => run.workflow_run_id),
      );
      const active = new Set(
        task.workflow_runs
          .filter((run) => run.participation === "active")
          .map((run) => run.workflow_run_id),
      );
      for (const item of items)
        if (
          item.taskID === task.task_id &&
          item.participating &&
          !known.has(item.id)
        )
          active.add(item.id);
      // Include confirmations from this event before React has rendered them.
      for (const entry of current.current.entries)
        if (
          entry.task.task_id === task.task_id &&
          !known.has(entry.id) &&
          !entry.unsent &&
          receiptFor(entry)?.status !== "rejected"
        )
          active.add(entry.id);
      if (active.size >= 5) return;
      const first =
        task.status === "open"
          ? current.current.entries.find(
              (entry) =>
                entry.task.task_id === task.task_id &&
                entry.envelope?.action === "start_task" &&
                entry.envelope.expected_version === task.version &&
                !entry.unsent &&
                receiptFor(entry)?.status !== "rejected",
            )?.envelope ||
            rows.find(
              (row) =>
                row.envelope.instance_id === api.identity.instance_id &&
                row.envelope.action === "start_task" &&
                row.envelope.target_id === task.task_id &&
                row.envelope.expected_version === task.version &&
                row.response?.status !== "rejected",
            )?.envelope
          : undefined;
      const entry: LocalStart = {
        id: crypto.randomUUID().replaceAll("-", ""),
        task: structuredClone(task),
        profile: structuredClone(profile),
        graphRevision,
        controlRevision,
        first,
      };
      current.current.entries = [...current.current.entries, entry];
      setLocal(current.current.entries);
      if (!first) void send(entry, task, task.status === "in_progress");
      return entry.id;
    },
  };
}
