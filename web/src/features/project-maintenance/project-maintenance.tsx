import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { SafeLink, short } from "../../shared/ui/status";
import { testPreparationNames as names } from "../tasks/use-test-readiness";
import type { ProjectReadiness } from "../../shared/api/types";
import { projectBlockerLabel } from "./project-readiness-status";
import {
  CopyExecutionInfo,
  requestExecutionInfo,
} from "../activities/execution-info";
import { OperationResult } from "../activities/operation-result";
import {
  SubmissionStatus,
  useActivityFeedback,
} from "../activities/feedback/feedback";

export type Commit = {
  sha: string;
  title: string;
  author: string;
  time: string;
  url: string;
  prs: string[];
  pr_diagnostic?: string;
};
export type MaintenanceView = {
  readiness: ProjectReadiness;
  observation: { commit: string; last_success?: string; diagnostic?: string };
  remote_name: string;
  remote: Commit;
  local_info?: Commit;
  local: {
    commit: string;
    branch: string;
    status: string;
    ahead: number;
    behind: number;
    can_update: boolean;
    reason?: string;
  };
  tasks: {
    task_id: string;
    contract: { plan_ref: { path: string } };
    status: string;
    diagnostic?: string;
  }[];
};
const time = (value?: string) =>
  value ? new Date(value).toLocaleString() : "时间未知";

function CommitDetails({ commit }: { commit: Commit }) {
  const { api, online, identity } = useWorkspace();
  const [expanded, setExpanded] = useState(false);
  const detail = useQuery({
    queryKey: [
      "commit-details",
      identity.project_id,
      identity.instance_id,
      commit.sha,
    ],
    queryFn: () => api.get<Commit>("/maintenance/commits/" + commit.sha),
    enabled: online && expanded,
    staleTime: 60000,
    retry: false,
  });
  const value = detail.data || commit;
  return (
    <div className="maintenance-commit">
      <span>
        {time(commit.time)} · {commit.title || "正在读取提交"} · {commit.author}
      </span>{" "}
      {commit.url && <SafeLink url={commit.url}>查看提交</SafeLink>}{" "}
      <button onClick={() => setExpanded(!expanded)} aria-expanded={expanded}>
        提交详情 / 关联 PR
      </button>
      {expanded && (
        <div>
          {detail.isFetching && <p>正在查询关联 PR…</p>}
          {detail.error && <p role="alert">{String(detail.error)}</p>}
          {value.prs?.map((url) => (
            <p key={url}>
              <SafeLink url={url}>{url}</SafeLink>
            </p>
          ))}
          {value.pr_diagnostic ? (
            <p className="muted">{value.pr_diagnostic}</p>
          ) : (
            !detail.isFetching && (
              <p className="muted">
                {value.prs?.length ? "" : "该提交没有关联 PR"}
              </p>
            )
          )}
        </div>
      )}
    </div>
  );
}
export function ProjectMaintenance() {
  const { api, identity, online, refresh, requests, submit, pendingRequests } =
    useWorkspace();
  const feedback = useActivityFeedback();
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [logs, setLogs] = useState(false),
    [page, setPage] = useState(0),
    [scope, setScope] = useState("history");
  const [params] = useSearchParams();
  const requestID = params.get("request_id");
  const taskID = params.get("task");
  const base = [
    api.home,
    identity.project_id,
    identity.instance_id,
    "maintenance",
  ];
  const view = useQuery({
    queryKey: base,
    queryFn: () => api.get<MaintenanceView>("/maintenance"),
    enabled: online,
    refetchInterval: online ? 5000 : false,
    retry: false,
  });
  const taskVisible = view.data?.tasks.some((task) => task.task_id === taskID);
  useEffect(() => {
    if (taskVisible)
      document
        .getElementById("project-status-task-" + taskID)
        ?.scrollIntoView?.();
  }, [taskID, taskVisible]);
  const log = useQuery({
    queryKey: [...base, "log", view.data?.observation.commit, page, scope],
    queryFn: () =>
      api.get<{ commits: Commit[]; more: boolean }>(
        "/maintenance/log?page=" + page + "&scope=" + scope,
      ),
    enabled: online && logs,
    retry: false,
  });
  async function action(
    name: string,
    target: string,
    payload: Record<string, unknown> = {},
  ) {
    if (name === "maintenance_check") {
      if (
        !online ||
        busy ||
        checking() ||
        pending.some((r) => r.envelope.action === name)
      )
        return;
      setError("");
      const operation = submit(api.envelope(name, target, 1, payload));
      feedback.notify("检查更新", document.activeElement as HTMLElement);
      // The original request owner retains/reconciles the receipt and error;
      // don't freeze an unknown result in a second local error string.
      await operation;
      void view.refetch({ cancelRefetch: false });
      return;
    }
    setBusy(true);
    setError("");
    try {
      const r = await api.submit(api.envelope(name, target, 1, payload));
      if (r.status !== "applied")
        throw new Error(r.diagnostic || "原操作结果待核实");
      const operation = r.maintenance as
        { status?: string; diagnostic?: string } | undefined;
      if (operation?.status === "outcome_unknown")
        setError(operation.diagnostic || "原更新结果待核实，请人工核对");
      await view.refetch();
      refresh();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  const v = view.data;
  const verified = online && view.isSuccess;
  const l = v?.local;
  const localLabel = !l
    ? "正在读取本地版本"
    : l.status === "synced"
      ? "已同步"
      : l.status === "behind"
        ? "落后 " + l.behind + " 个提交"
        : l.status === "ahead"
          ? "领先 " + l.ahead + " 个提交"
          : l.status === "diverged"
            ? "已分叉 · 领先 " + l.ahead + " / 落后 " + l.behind + " 个提交"
            : "状态待核实";
  const pending =
    requests?.filter(
      (r) =>
        r.envelope.project_id === identity.project_id &&
        r.envelope.instance_id === identity.instance_id &&
        r.envelope.action.startsWith("maintenance_") &&
        !["applied", "rejected"].includes(r.response?.status || ""),
    ) || [];
  const checking = () =>
    (pendingRequests?.() || []).some((e) => e.action === "maintenance_check");
  const latestCheck = (requests || [])
    .filter(
      (r) =>
        r.envelope.project_id === identity.project_id &&
        r.envelope.instance_id === identity.instance_id &&
        r.envelope.action === "maintenance_check",
    )
    .sort((a, b) => b.created_at.localeCompare(a.created_at))[0];
  const checkLabel = checking()
    ? "正在检查更新…"
    : latestCheck?.response?.status === "applied"
      ? "本次检查完成"
      : latestCheck?.response?.status === "rejected"
        ? "检查未启动 · " + (latestCheck.response.diagnostic || "请查看原请求")
        : pending.some((r) => r.envelope.action === "maintenance_check")
          ? "检查结果待核实 · 请查看原请求"
          : "";
  return (
    <section aria-label="项目状态" className="settings-panel maintenance-panel">
      <h1>项目状态</h1>
      <p className="path">{identity.repository_identity}</p>
      <p className="muted">
        项目 {short(identity.project_id)} · 服务实例{" "}
        {short(identity.instance_id)}
      </p>
      <div>
        {!verified && (
          <p className="notice amber">
            {online ? "当前状态待核实" : "离线 · 当前状态待核实"}
            {v &&
              ` · 上次状态核实 ${time(new Date(view.dataUpdatedAt).toISOString())}`}
          </p>
        )}
        {(error || view.error) && (
          <p role="alert">{error || String(view.error)}</p>
        )}
        {requestID && (
          <OperationResult
            requestID={requestID}
            targetID={identity.project_id}
          />
        )}
        {!!v?.readiness.blockers.length && (
          <section aria-labelledby="project-blockers-heading">
            <h2 id="project-blockers-heading">
              {verified ? "未就绪原因" : "上次核实的原因"}
            </h2>
            <ul className="project-status-reasons">
              {v.readiness.blockers.map((reason) => (
                <li
                  className="notice amber"
                  key={JSON.stringify([
                    reason.code,
                    reason.task_id,
                    reason.path,
                  ])}
                >
                  <strong>{projectBlockerLabel(reason)}</strong>
                  <p>{reason.diagnostic}</p>
                  {reason.path && <code>{reason.path}</code>}{" "}
                  {reason.task_id ? (
                    <a
                      href={`#project-status-task-${encodeURIComponent(reason.task_id)}`}
                    >
                      查看任务 #{reason.task_id} 材料
                    </a>
                  ) : (
                    <a href="#project-status-git">查看 Git 基线</a>
                  )}
                </li>
              ))}
            </ul>
          </section>
        )}
        <h2 id="project-status-git">远端基线 · {v?.remote_name}</h2>
        <p className="muted">
          {v?.observation.last_success ? (
            <time dateTime={v.observation.last_success}>
              最近成功检查 {time(v.observation.last_success)}
            </time>
          ) : (
            "上次成功检查时间未知"
          )}
        </p>
        {v?.remote?.sha ? (
          <CommitDetails commit={v.remote} />
        ) : (
          <p>尚无成功观察</p>
        )}
        <h2>
          本地基线 · {l?.branch || "分支待核实"} · {localLabel}
        </h2>
        {v?.local_info?.sha ? (
          <CommitDetails commit={v.local_info} />
        ) : (
          l?.commit && <code>{l.commit.slice(0, 12)}</code>
        )}
        <p>
          <button
            disabled={
              !online ||
              busy ||
              checking() ||
              pending.some((r) => r.envelope.action === "maintenance_check")
            }
            onClick={() =>
              void action("maintenance_check", identity.project_id)
            }
          >
            {checking() ? "正在检查更新…" : "检查更新"}
          </button>{" "}
          <button
            disabled={
              !online ||
              busy ||
              checking() ||
              pending.some((r) => r.envelope.action === "maintenance_check") ||
              !l?.can_update
            }
            onClick={() =>
              void action("maintenance_update", identity.project_id)
            }
          >
            更新本地
          </button>{" "}
          <button onClick={() => setLogs(!logs)} aria-expanded={logs}>
            查看提交日志
          </button>
        </p>
        <SubmissionStatus
          label={checkLabel}
          busy={checking()}
          showActivity={!!latestCheck}
        />
        {latestCheck && (
          <CopyExecutionInfo
            key={latestCheck.envelope.request_id}
            info={{
              ...requestExecutionInfo(
                latestCheck.envelope,
                latestCheck.response,
              ),
              project: identity.repository_identity,
            }}
          />
        )}
        {logs && (
          <section aria-label="提交日志">
            <label>
              日志范围{" "}
              <select
                value={scope}
                onChange={(e) => {
                  setScope(e.target.value);
                  setPage(0);
                }}
              >
                <option value="history">远端提交历史</option>
                <option value="behind">本地缺少的提交</option>
                <option value="ahead">本地独有的提交</option>
              </select>
            </label>
            {log.error && <p role="alert">{String(log.error)}</p>}
            {log.data?.commits.map((c) => (
              <CommitDetails key={c.sha} commit={c} />
            ))}
            {log.data?.commits.length === 0 && <p>没有对应提交</p>}
            <button disabled={page === 0} onClick={() => setPage(page - 1)}>
              上一页
            </button>{" "}
            <button
              disabled={!log.data?.more}
              onClick={() => setPage(page + 1)}
            >
              下一页
            </button>
          </section>
        )}
        <h2>Git 任务材料与测试安排</h2>
        {v?.readiness && (
          <p className="muted">
            测试安排 {v.readiness.ready}/{v.readiness.total} 就绪
            {!!v.readiness.waiting &&
              ` · ${v.readiness.waiting} 项等待前置测试`}
          </p>
        )}
        <p className="muted">
          检查更新后读取已提交的 Plan 和同名 .tests.json。材料错误通过正常 Git
          修改处理。
        </p>
        {v?.tasks.map((t) => (
          <article key={t.task_id} id={`project-status-task-${t.task_id}`}>
            <p>
              任务 #{t.task_id} · {names[t.status] || t.status}
            </p>
            <code>
              {t.contract.plan_ref.path.replace(/\.plan\.md$/, ".tests.json")}
            </code>
            {t.diagnostic && <p role="alert">{t.diagnostic}</p>}
          </article>
        ))}
        {pending.map((r) => (
          <p key={r.envelope.request_id}>
            有更新操作结果待核实。
            <button
              disabled={!online || busy}
              onClick={() =>
                void api
                  .reconcile(r.envelope)
                  .then(() => {
                    refresh();
                    void view.refetch();
                  })
                  .catch((e) => setError(String(e)))
              }
            >
              核对更新结果
            </button>
          </p>
        ))}
      </div>
    </section>
  );
}
