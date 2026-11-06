import { useEffect, useRef, useState } from "react";
import { useLocation } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { SafeLink } from "../../shared/ui/status";
import {
  GenerationProgress,
  type GenerationProgressValue,
} from "./generation-progress";
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
    generation_id: string;
    needs_attention?: boolean;
    can_retry?: boolean;
  }[];
  profiles: { id: string; name: string }[];
  profile: string;
  generation_progress?: GenerationProgressValue;
};
const time = (value?: string) =>
  value ? new Date(value).toLocaleString() : "时间未知";

function GenerationDetails({
  id,
  onStop,
  busy,
}: {
  id: string;
  onStop: () => void;
  busy: boolean;
}) {
  const { api, online, identity } = useWorkspace();
  const [open, setOpen] = useState(false);
  const detail = useQuery({
    queryKey: [
      "maintenance-generation",
      identity.project_id,
      identity.instance_id,
      id,
    ],
    queryFn: () =>
      api.get<{
        state: string;
        run_id?: string;
        diagnostic?: string;
        stop_status?: string;
        settled: boolean;
        needs_attention: boolean;
        result?: unknown;
      }>("/tests/generations/" + id),
    enabled: online && open,
    retry: false,
    refetchInterval: online && open ? 2000 : false,
  });
  return (
    <div>
      <button aria-expanded={open} onClick={() => setOpen(!open)}>
        查看生成执行
      </button>
      {open && (
        <div>
          {detail.isFetching && !detail.data && <p>正在读取生成执行…</p>}
          {detail.error && <p role="alert">{String(detail.error)}</p>}
          {detail.data && (
            <>
              <p>
                {detail.data.state === "cancelled" && !detail.data.settled
                  ? "等待停止确认"
                  : detail.data.needs_attention
                    ? "生成受阻，需人工处理"
                    : names[detail.data.state] || detail.data.state}
              </p>
              {detail.data.diagnostic && (
                <p role="alert">{detail.data.diagnostic}</p>
              )}
              {detail.data.result != null && (
                <pre>{JSON.stringify(detail.data.result, null, 2)}</pre>
              )}
              {detail.data.stop_status !== "stopped" &&
                !["rejected", "cancelled"].includes(detail.data.state) && (
                  <button disabled={!online || busy} onClick={onStop}>
                    取消生成
                  </button>
                )}
            </>
          )}
        </div>
      )}
    </div>
  );
}

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
    [scope, setScope] = useState("history"),
    [profile, setProfile] = useState("");
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
  const problems = v?.tasks.filter((t) => t.status !== "ready") || [];
  const pending =
    requests?.filter(
      (r) =>
        r.envelope.action.startsWith("maintenance_") &&
        !["applied", "rejected"].includes(r.response?.status || ""),
    ) || [];
  const preparing = ["queued", "preparing", "generating"].includes(
    v?.generation_progress?.status || "",
  );
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
      <GenerationProgress
        value={v?.generation_progress}
        online={online}
        scope={identity.project_id}
      />
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
          <h4>准备材料与测试安排</h4>
          <button
            disabled={
              !online || busy || preparing || view.isPending || view.isError
            }
            onClick={() =>
              void action("maintenance_generate_all", identity.project_id)
            }
          >
            {preparing ? "准备进行中" : "准备材料与测试安排"}
          </button>
          <p className="muted">
            本地同步且所有未完成任务的材料与测试安排就绪后，才允许新执行。准备在后台执行；已有候选保留其执行版本。
          </p>
          {v?.generation_progress?.diagnostic && (
            <p role="alert">{v.generation_progress.diagnostic}</p>
          )}
          {v?.tasks.map((t) => (
            <article key={t.task_id}>
              <strong>
                #{t.task_id} ·{" "}
                {t.needs_attention
                  ? "生成受阻，需人工处理"
                  : names[t.status] || t.status}
              </strong>
              <p>{t.contract?.plan_ref?.path}</p>
              {t.diagnostic && <p>{t.diagnostic}</p>}
              {t.generation_id && (
                <GenerationDetails
                  id={t.generation_id}
                  busy={busy}
                  onStop={() =>
                    void action(
                      "maintenance_stop_generation",
                      t.generation_id,
                      { generation_id: t.generation_id },
                    )
                  }
                />
              )}
              {["failed", "invalid", "rejected", "cancelled"].includes(
                t.status,
              ) && (
                <button
                  disabled={!online || busy || !t.can_retry}
                  onClick={() =>
                    void action("maintenance_retry", t.task_id, {
                      task_id: t.task_id,
                    })
                  }
                >
                  重试生成
                </button>
              )}
              {["failed", "invalid", "cancelled"].includes(t.status) &&
                !t.can_retry && (
                  <p className="muted">等待原生成停止并核实资源后重试</p>
                )}
            </article>
          ))}
          {(problems.some(
            (t) =>
              t.status === "configuration_required" ||
              t.status === "unprepared",
          ) ||
            !v?.tasks.length) && (
            <div>
              <label>
                测试生成方案{" "}
                <select
                  aria-label="测试生成方案"
                  value={profile || v?.profile || ""}
                  onChange={(e) => setProfile(e.target.value)}
                >
                  <option value="">选择本机 Codex 方案</option>
                  {v?.profiles.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
              </label>
              <button
                disabled={!online || busy || !(profile || v?.profile)}
                onClick={() =>
                  void action("maintenance_profile", identity.project_id, {
                    profile: profile || v?.profile,
                  })
                }
              >
                保存测试生成方案
              </button>
            </div>
          )}
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
