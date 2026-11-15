import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { GitPullRequest, RefreshCw } from "lucide-react";
import { useWorkspace } from "../../app/context";
import { Button } from "../../shared/ui/button";
import { SafeLink } from "../../shared/ui/status";
import {
  LaunchEntries,
  LaunchPanel,
  RunStatus,
  VerificationStatus,
  isActiveRun,
  type Launch,
} from "./verification-ui";

type Pull = {
  number: number;
  title: string;
  url: string;
  branch: string;
  head: string;
  state: string;
};
type PullDetail = {
  pr: Pull;
  entries?: {
    id: string;
    name: string;
    launch: Launch;
  }[];
  diagnostic?: string;
};
type Run = {
  id: string;
  head: string;
  pr_number: number;
  name: string;
  status: string;
  worktree: string;
  removed: boolean;
  diagnostic?: string;
  cleanup_diagnostic?: string;
  url?: string;
  exit_code?: number;
};
const base = "/manual-verification/pulls";
export default function PRLaunch({
  expanded,
  toggle,
}: {
  expanded: boolean;
  toggle: () => void;
}) {
  const { api, identity, online, submit } = useWorkspace();
  const [lookup, setLookup] = useState("");
  const [selected, select] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [logsID, setLogsID] = useState("");
  const key = [
    "pr-launch",
    api.home,
    identity.project_id,
    identity.instance_id,
  ];
  const recent = useQuery({
    queryKey: [...key, "recent"],
    queryFn: ({ signal }) => api.get<Pull[]>(base, signal),
    enabled: online && expanded,
    refetchOnWindowFocus: false,
  });
  const detail = useQuery({
    queryKey: [...key, "detail", selected],
    queryFn: ({ signal }) => api.get<PullDetail>(base + "/" + selected, signal),
    enabled: online && !!selected,
    refetchOnWindowFocus: false,
  });
  const runQuery = useQuery({
    queryKey: [...key, "run"],
    queryFn: ({ signal }) =>
      api.get<{ latest?: Run; retained?: Run[] }>(base + "/run", signal),
    enabled: online,
    refetchInterval: 2500,
  });
  const run = runQuery.data?.latest;
  const runs = [...(run ? [run] : []), ...(runQuery.data?.retained ?? [])];
  const active = !!run && isActiveRun(run.status);
  const logs = useQuery({
    queryKey: [...key, "logs", logsID],
    queryFn: ({ signal }) =>
      api.get<{ stdout?: string; stderr?: string }>(
        base + "/logs/" + logsID,
        signal,
      ),
    enabled: online && runs.some((item) => logsID === item.id),
    refetchInterval: active && run?.id === logsID ? 2500 : false,
  });
  const available = detail.data;
  const blocked = !online || pending || active;

  function choose(number: string) {
    setError("");
    select(number);
    if (number && number === selected) void detail.refetch();
  }

  async function command(
    action: "start" | "stop" | "cleanup",
    payload: Record<string, string | number>,
  ) {
    setPending(true);
    setError("");
    try {
      const response = await submit(
        api.envelope(
          "project_launch_pr_" + action,
          identity.project_id,
          1,
          payload,
        ),
      );
      if (response?.status !== "applied") {
        setError(response?.diagnostic || "操作结果待核实");
        if (action === "start") await detail.refetch();
      }
    } catch (err) {
      setError(String(err));
    } finally {
      await runQuery.refetch();
      setPending(false);
    }
  }

  return (
    <LaunchPanel
      id="mv-pr-launch"
      title="从 PR 启动"
      subtitle="选择提交 · 临时 worktree"
      icon={GitPullRequest}
      expanded={expanded}
      toggle={toggle}
      status={
        active && run ? (
          <RunStatus status={run.status} />
        ) : runs.some((item) => item.cleanup_diagnostic) ? (
          <VerificationStatus tone="attention">现场待清理</VerificationStatus>
        ) : undefined
      }
    >
      {!online && <p className="mv-meta">项目连接离线。</p>}
      <div className="mv-fields">
        <label htmlFor="recent-launch-pr">最近更新的开放 PR</label>
        <div className="mv-input-row">
          <select
            id="recent-launch-pr"
            value={
              (recent.data ?? []).some((pr) => String(pr.number) === selected)
                ? selected
                : ""
            }
            disabled={!online || pending}
            onChange={(event) => choose(event.target.value)}
          >
            <option value="">
              {recent.isFetching
                ? "正在读取…"
                : recent.data?.length === 0
                  ? "暂无开放 PR"
                  : "选择 PR"}
            </option>
            {(recent.data ?? []).slice(0, 10).map((pr) => (
              <option key={pr.number} value={pr.number}>
                #{pr.number} {pr.title}
              </option>
            ))}
          </select>
          <Button
            size="sm"
            variant="ghost"
            type="button"
            aria-label="刷新最近 PR"
            disabled={!online || recent.isFetching}
            onClick={() => void recent.refetch()}
          >
            <RefreshCw size={15} aria-hidden="true" />
          </Button>
        </div>
        {recent.error && (
          <p className="notice amber" role="alert">
            {String(recent.error)}
          </p>
        )}
        <form
          className="mv-fields"
          onSubmit={(event) => {
            event.preventDefault();
            choose(lookup.trim());
          }}
        >
          <label htmlFor="lookup-launch-pr">PR 编号</label>
          <div className="mv-input-row">
            <input
              id="lookup-launch-pr"
              inputMode="numeric"
              value={lookup}
              onChange={(event) => setLookup(event.target.value)}
            />
            <Button
              type="submit"
              disabled={
                !online || pending || !/^[1-9][0-9]{0,9}$/.test(lookup.trim())
              }
            >
              查找 PR
            </Button>
          </div>
        </form>
      </div>
      {detail.isFetching && (
        <p className="mv-meta" role="status">
          正在读取 PR 提交和启动入口…
        </p>
      )}
      {(error || detail.error || available?.diagnostic) && (
        <p className="notice amber" role="alert">
          {error ||
            (detail.error ? String(detail.error) : available?.diagnostic)}
        </p>
      )}
      {available && (
        <div className="mv-run">
          <strong>
            <SafeLink url={available.pr.url}>
              #{available.pr.number} {available.pr.title}
            </SafeLink>
          </strong>
          <details className="mv-disclosure">
            <summary>
              {available.pr.branch} ·{" "}
              <code>{available.pr.head.slice(0, 8)}</code>
            </summary>
            <p>
              <code>{available.pr.head}</code>
            </p>
          </details>
          <LaunchEntries
            entries={available.entries ?? []}
            disabled={blocked || detail.isFetching || !!detail.error}
            start={(id) =>
              void command("start", {
                pr_number: available.pr.number,
                head: available.pr.head,
                entry_id: id,
              })
            }
          />
        </div>
      )}
      {runs.map((run) => (
        <div key={run.id} className="mv-run">
          <div className="mv-line">
            <strong>
              PR #{run.pr_number} · {run.name}
            </strong>
            <RunStatus status={run.status} />
          </div>
          <p className="mv-meta">
            <code title={run.head}>{run.head.slice(0, 8)}</code>
            {run.exit_code !== undefined && <> · 退出码 {run.exit_code}</>}
          </p>
          <details className="mv-disclosure">
            <summary>Worktree 路径{run.removed && " · 已清理"}</summary>
            <p>
              <code>{run.worktree}</code>
            </p>
          </details>
          {run.diagnostic && <p>{run.diagnostic}</p>}
          {run.cleanup_diagnostic && (
            <p role="status">{run.cleanup_diagnostic}</p>
          )}
          <div className="mv-actions">
            {run.status === "ready" && run.url && (
              <SafeLink url={run.url}>打开 PR 预览</SafeLink>
            )}{" "}
            {active && run.id === runQuery.data?.latest?.id && (
              <Button
                size="sm"
                type="button"
                disabled={!online || pending}
                onClick={() => void command("stop", { attempt_id: run.id })}
              >
                {run.status === "unknown" ? "核对 PR 进程" : "停止 PR 启动"}
              </Button>
            )}{" "}
            {!!run.cleanup_diagnostic && (
              <Button
                size="sm"
                type="button"
                disabled={!online || pending}
                onClick={() => void command("cleanup", { attempt_id: run.id })}
              >
                重试清理
              </Button>
            )}{" "}
            <Button
              size="sm"
              variant="ghost"
              type="button"
              aria-expanded={logsID === run.id}
              onClick={() => setLogsID(logsID === run.id ? "" : run.id)}
            >
              {logsID === run.id ? "隐藏 PR 日志" : "查看 PR 日志"}
            </Button>
          </div>
          {logsID === run.id && (
            <pre className="mv-logs">
              {logs.error
                ? String(logs.error)
                : [logs.data?.stdout, logs.data?.stderr]
                    .filter(Boolean)
                    .join("\n") || "暂无输出"}
            </pre>
          )}
        </div>
      ))}
    </LaunchPanel>
  );
}
