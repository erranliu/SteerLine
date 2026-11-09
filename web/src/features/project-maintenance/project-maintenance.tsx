import { useEffect, useRef, useState } from "react";
import { useLocation } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { SafeLink } from "../../shared/ui/status";
import { testPreparationNames as names } from "../tasks/use-test-readiness";
import type { ProjectReadiness } from "../../shared/api/types";

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
          <code>{commit.sha}</code>
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
  const { api, identity, online, refresh, requests } = useWorkspace();
  const [open, setOpen] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [logs, setLogs] = useState(false),
    [page, setPage] = useState(0),
    [scope, setScope] = useState("history");
  const location = useLocation();
  const panel = useRef<HTMLElement>(null);
  useEffect(() => {
    if (
      new URLSearchParams(location.search).get("maintenance") === "preparation"
    ) {
      setOpen(true);
      panel.current?.scrollIntoView?.({ block: "start", behavior: "smooth" });
    }
  }, [location.search, location.key]);
  const base = ["maintenance", identity.project_id, identity.instance_id];
  const view = useQuery({
    queryKey: base,
    queryFn: () => api.get<MaintenanceView>("/maintenance"),
    enabled: online,
    refetchInterval: online ? 5000 : false,
    retry: false,
  });
  const log = useQuery({
    queryKey: [...base, "log", view.data?.observation.commit, page, scope],
    queryFn: () =>
      api.get<{ commits: Commit[]; more: boolean }>(
        "/maintenance/log?page=" + page + "&scope=" + scope,
      ),
    enabled: online && open && logs,
    retry: false,
  });
  async function action(
    name: string,
    target: string,
    payload: Record<string, unknown> = {},
  ) {
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
  const ready = online && view.isSuccess && v?.readiness.status === "ready";
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
        r.envelope.action.startsWith("maintenance_") &&
        !["applied", "rejected"].includes(r.response?.status || ""),
    ) || [];
  return (
    <section
      ref={panel}
      id="project-maintenance"
      aria-label="项目维护"
      className="settings-panel maintenance-panel"
    >
      <button onClick={() => setOpen(!open)} aria-expanded={open}>
        项目维护 · {ready ? "已就绪" : "未就绪"}
      </button>
      {v?.readiness && (
        <p className="muted">
          {localLabel} · 测试安排 {v.readiness.ready}/{v.readiness.total} 就绪
        </p>
      )}
      {v?.observation.last_success && (
        <small> · 最近成功检查 {time(v.observation.last_success)}</small>
      )}
      {open && (
        <div>
          {!online && (
            <p className="notice amber">
              离线 · 以下为最后观察，当前状态待核实
            </p>
          )}
          {(error || view.error) && (
            <p role="alert">{error || String(view.error)}</p>
          )}
          {v?.readiness.blockers.map((reason) => (
            <p className="notice amber" key={reason}>
              {reason}
            </p>
          ))}
          {v?.observation.diagnostic && (
            <p role="alert">
              检查远端失败：{v.observation.diagnostic}
              。请核对网络、认证及目标分支后重试。
            </p>
          )}
          <h4>远端基线 · {v?.remote_name}</h4>
          {v?.remote?.sha ? (
            <CommitDetails commit={v.remote} />
          ) : (
            <p>尚无成功观察</p>
          )}
          <h4>
            本地基线 · {l?.branch || "分支待核实"} · {localLabel}
          </h4>
          {v?.local_info?.sha ? (
            <CommitDetails commit={v.local_info} />
          ) : (
            l?.commit && <code>{l.commit.slice(0, 12)}</code>
          )}
          {l?.reason && <p className="notice amber">{l.reason}</p>}
          <p>
            <button
              disabled={!online || busy}
              onClick={() =>
                void action("maintenance_check", identity.project_id)
              }
            >
              检查更新
            </button>{" "}
            <button
              disabled={!online || busy || !l?.can_update}
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
          <h4>Git 任务材料与测试安排</h4>
          <p className="muted">检查更新后读取已提交的 Plan 和同名 .tests.json。材料错误通过正常 Git 修改处理。</p>
          {v?.tasks.map((t) => (
            <article key={t.task_id}>
              <p>任务 #{t.task_id} · {names[t.status] || t.status}</p>
              <code>{t.contract.plan_ref.path.replace(/\.plan\.md$/, ".tests.json")}</code>
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
      )}
    </section>
  );
}
