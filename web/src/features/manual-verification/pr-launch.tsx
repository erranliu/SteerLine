import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { Button } from "../../shared/ui/button";
import { SafeLink } from "../../shared/ui/status";

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
    launch: { program: string; args: string[]; cwd?: string };
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
const statuses: Record<string, string> = {
  preparing: "正在准备 worktree",
  running: "进程运行中",
  ready: "预览可访问",
  exited: "进程已退出",
  stopped: "已停止",
  failed: "启动或运行失败",
  unknown: "原进程待核实",
};

export default function PRLaunch() {
  const { api, identity, online, submit } = useWorkspace();
  const [expanded, setExpanded] = useState(false);
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
  const active =
    !!run && ["preparing", "running", "ready", "unknown"].includes(run.status);
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
    <section aria-labelledby="pr-launch-title">
      <h2 id="pr-launch-title">从 PR 启动</h2>
      <Button
        type="button"
        disabled={!online}
        onClick={() => {
          setExpanded(true);
          if (expanded) void recent.refetch();
        }}
      >
        {expanded ? "刷新最近 PR" : "选择项目 PR"}
      </Button>
      {expanded && (
        <>
          <label htmlFor="recent-launch-pr">最近更新的开放 PR</label>{" "}
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
              {recent.isFetching ? "正在读取…" : "选择 PR"}
            </option>
            {(recent.data ?? []).slice(0, 10).map((pr) => (
              <option key={pr.number} value={pr.number}>
                #{pr.number} {pr.title}
              </option>
            ))}
          </select>
          {recent.error && <p role="alert">{String(recent.error)}</p>}
          <form
            onSubmit={(event) => {
              event.preventDefault();
              choose(lookup.trim());
            }}
          >
            <label htmlFor="lookup-launch-pr">PR 编号</label>{" "}
            <input
              id="lookup-launch-pr"
              inputMode="numeric"
              value={lookup}
              onChange={(event) => setLookup(event.target.value)}
            />{" "}
            <Button
              type="submit"
              disabled={
                !online || pending || !/^[1-9][0-9]{0,9}$/.test(lookup.trim())
              }
            >
              查找 PR
            </Button>
          </form>
        </>
      )}
      {detail.isFetching && <p>正在读取 PR 提交和启动入口…</p>}
      {(error || detail.error || available?.diagnostic) && (
        <p role="alert">
          {error ||
            (detail.error ? String(detail.error) : available?.diagnostic)}
        </p>
      )}
      {available && (
        <>
          <p>
            <SafeLink url={available.pr.url}>
              #{available.pr.number} {available.pr.title}
            </SafeLink>
            {" · "}
            {available.pr.branch} · <code>{available.pr.head}</code>
          </p>
          <ul>
            {(available.entries ?? []).map((entry) => (
              <li key={entry.id}>
                <Button
                  type="button"
                  disabled={blocked || detail.isFetching || !!detail.error}
                  onClick={() =>
                    void command("start", {
                      pr_number: available.pr.number,
                      head: available.pr.head,
                      entry_id: entry.id,
                    })
                  }
                >
                  {entry.name}
                </Button>{" "}
                <code>
                  {[entry.launch.program, ...entry.launch.args]
                    .map((arg) => (/\s/.test(arg) ? JSON.stringify(arg) : arg))
                    .join(" ")}
                </code>
                {entry.launch.cwd && (
                  <>
                    {" "}
                    · <code>{entry.launch.cwd}</code>
                  </>
                )}
              </li>
            ))}
          </ul>
        </>
      )}
      {runs.map((run) => (
        <div key={run.id}>
          <p>
            PR #{run.pr_number} · {run.name} · {statuses[run.status]} ·{" "}
            <code>{run.head}</code>
            {run.exit_code !== undefined && <> · 退出码 {run.exit_code}</>}
          </p>
          <p>
            <code>{run.worktree}</code>
            {run.removed && " · 已清理"}
          </p>
          {run.diagnostic && <p>{run.diagnostic}</p>}
          {run.cleanup_diagnostic && (
            <p role="status">{run.cleanup_diagnostic}</p>
          )}
          {run.status === "ready" && run.url && (
            <SafeLink url={run.url}>打开 PR 预览</SafeLink>
          )}{" "}
          {active && run.id === runQuery.data?.latest?.id && (
            <Button
              type="button"
              disabled={!online || pending}
              onClick={() => void command("stop", { attempt_id: run.id })}
            >
              {run.status === "unknown" ? "核对 PR 进程" : "停止 PR 启动"}
            </Button>
          )}{" "}
          {!!run.cleanup_diagnostic && (
            <Button
              type="button"
              disabled={!online || pending}
              onClick={() => void command("cleanup", { attempt_id: run.id })}
            >
              重试清理
            </Button>
          )}{" "}
          <Button
            type="button"
            onClick={() => setLogsID(logsID === run.id ? "" : run.id)}
          >
            {logsID === run.id ? "隐藏 PR 日志" : "查看 PR 日志"}
          </Button>
          {logsID === run.id && (
            <pre>
              {logs.error
                ? String(logs.error)
                : [logs.data?.stdout, logs.data?.stderr]
                    .filter(Boolean)
                    .join("\n") || "暂无输出"}
            </pre>
          )}
        </div>
      ))}
    </section>
  );
}
