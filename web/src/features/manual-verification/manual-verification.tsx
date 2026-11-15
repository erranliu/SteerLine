import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { ChevronRight, ClipboardCheck, Play } from "lucide-react";
import { useWorkspace } from "../../app/context";
import { Button } from "../../shared/ui/button";
import { SafeLink } from "../../shared/ui/status";
import PRLaunch from "./pr-launch";
import {
  LaunchEntries,
  LaunchPanel,
  RunStatus,
  VerificationStatus,
  isActiveRun,
  runStatuses as statuses,
  type Launch,
} from "./verification-ui";
import "./manual-verification.css";

type Check = { id: string; title: string; steps: string[]; expected: string };
type Source = "pr" | "remote_head";
const sources: Record<Source, string> = {
  pr: "PR 位置",
  remote_head: "远端 Head",
};
type Attempt = {
  id: string;
  head: string;
  source: Source;
  status: string;
  started: boolean;
  removed?: boolean;
  worktree: string;
  program?: string;
  args?: string[];
  cwd?: string;
  url?: string;
  exit_code?: number;
  diagnostic?: string;
};
type Result = {
  head: string;
  source: Source;
  check_id: string;
  attempt_id: string;
  outcome: "passed" | "failed";
  notes: string;
  actor: string;
  at: string;
  evidence?: string[];
};
type ProjectRun = Pick<
  Attempt,
  "id" | "head" | "status" | "url" | "diagnostic" | "exit_code"
> & { name: string };
type ProjectLaunchView = {
  path: string;
  branch: string;
  head: string;
  ready: boolean;
  diagnostic?: string;
  entries: { id: string; name: string; launch: Launch }[];
  run?: ProjectRun;
};
type Review = {
  candidate_id: string;
  task_id: string;
  pr_url: string;
  head: string;
  revision: number;
  draft: boolean;
  merged: boolean;
  archived: boolean;
  closure?: { source: Source; head: string; actor: string; at: string };
  diagnostic?: string;
  configuration: {
    definition: {
      launch: Launch;
      checks: Check[];
    };
  };
  attempts: Attempt[];
  results: Result[];
};

const active = (a: Attempt) => isActiveRun(a.status);
const outcome = (result?: Result) =>
  result?.outcome === "passed"
    ? "通过"
    : result?.outcome === "failed"
      ? "失败"
      : "待核验";

function currentResults(record: Review, source: Source, head?: string) {
  return new Map(
    record.results
      .filter((r) => r.source === source && r.head === head)
      .map((r) => [r.check_id, r]),
  );
}

function reviewStatus(record: Review) {
  if (record.archived) return <VerificationStatus>已归档</VerificationStatus>;
  if (record.closure) return <VerificationStatus>已关闭</VerificationStatus>;
  if (record.merged)
    return <VerificationStatus tone="positive">PR 已合入</VerificationStatus>;
  return <VerificationStatus tone="attention">待核验</VerificationStatus>;
}

function prNumber(url: string) {
  return url.split("/").at(-1);
}

export default function ManualVerification() {
  const { api, identity, online, submit } = useWorkspace();
  const [params, setParams] = useSearchParams();
  const [history, setHistory] = useState(false);
  const [launch, setLaunch] = useState<"project" | "pr">();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const records = useQuery({
    queryKey: [
      "manual-verification",
      api.home,
      identity.project_id,
      identity.instance_id,
    ],
    queryFn: ({ signal }) => api.get<Review[]>("/manual-verification", signal),
    enabled: online,
    refetchInterval: 2500,
  });
  const rows = records.data ?? [];
  const visible = rows.filter(
    (row) => history || (!row.archived && !row.closure),
  );
  const selected = params.get("candidate");
  const record = selected
    ? rows.find((row) => row.candidate_id === selected)
    : visible[0];

  async function command(
    action: string,
    item: Review,
    payload: Record<string, unknown>,
  ) {
    setPending(true);
    setError("");
    try {
      const response = await submit(
        api.envelope(
          "manual_verification_" + action,
          item.candidate_id,
          item.revision,
          payload,
        ),
      );
      if (response?.status !== "applied") {
        setError(response?.diagnostic || "操作待核实，请刷新查看。");
        return false;
      }
      return true;
    } catch (err) {
      setError(String(err));
      return false;
    } finally {
      await records.refetch();
      setPending(false);
    }
  }

  return (
    <section className="manual-verification">
      <h1>人工核验</h1>
      <div className="mv-launch-grid">
        <ProjectLaunch
          expanded={launch === "project"}
          toggle={() => setLaunch(launch === "project" ? undefined : "project")}
        />
        <PRLaunch
          key={identity.project_id}
          expanded={launch === "pr"}
          toggle={() => setLaunch(launch === "pr" ? undefined : "pr")}
        />
      </div>
      <div
        className="mv-review-layout"
        data-split={!!record || visible.length > 0}
      >
        <section className="mv-panel mv-queue" aria-labelledby="mv-queue-title">
          <header className="mv-panel-header">
            <h2 id="mv-queue-title">核验待办</h2>
            {visible.length > 0 && (
              <span className="mv-count">{visible.length}</span>
            )}
          </header>
          {(error || records.error) && (
            <p className="notice amber" role="alert">
              {error || String(records.error)}
            </p>
          )}
          {visible.length === 0 && !records.error && (
            <div className="mv-empty">
              <ClipboardCheck size={20} aria-hidden="true" />
              <p>
                {!online
                  ? "项目连接离线。"
                  : records.isPending
                    ? "正在读取核验任务…"
                    : "暂无待核验 PR。"}
              </p>
            </div>
          )}
          <ul className="mv-queue-list">
            {visible.map((row) => {
              const completed = currentResults(
                row,
                row.closure?.source ?? "pr",
                row.closure?.head ?? row.head,
              ).size;
              return (
                <li key={row.candidate_id}>
                  <button
                    type="button"
                    className="mv-queue-button"
                    aria-current={
                      record?.candidate_id === row.candidate_id
                        ? "true"
                        : undefined
                    }
                    onClick={() => {
                      const next = new URLSearchParams(params);
                      next.set("candidate", row.candidate_id);
                      setParams(next);
                      setError("");
                    }}
                  >
                    <span className="mv-line">
                      <strong>PR #{prNumber(row.pr_url)}</strong>
                      {reviewStatus(row)}
                    </span>
                    <span className="mv-meta">
                      任务 {row.task_id} · {completed}/
                      {row.configuration.definition.checks.length} 已记录
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
          <label className="mv-history-toggle">
            <input
              type="checkbox"
              checked={history}
              onChange={(e) => setHistory(e.target.checked)}
            />
            显示已关闭或已归档的待办
          </label>
        </section>
        {record && (
          <ReviewDetail
            key={record.candidate_id}
            record={record}
            disabled={pending || !online}
            command={command}
          />
        )}
        {!record && visible.length > 0 && (
          <div className="mv-panel mv-empty">
            <p>选择一条待办查看核验详情。</p>
          </div>
        )}
      </div>
    </section>
  );
}

function ProjectLaunch({
  expanded,
  toggle,
}: {
  expanded: boolean;
  toggle: () => void;
}) {
  const { api, identity, online, submit } = useWorkspace();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [logsID, setLogsID] = useState("");
  const query = useQuery({
    queryKey: [
      "project-launch",
      api.home,
      identity.project_id,
      identity.instance_id,
    ],
    queryFn: ({ signal }) =>
      api.get<ProjectLaunchView>("/manual-verification/project", signal),
    enabled: online,
    refetchInterval: 5000,
  });
  const view = query.data;
  const run = view?.run;
  const running = !!run && isActiveRun(run.status);
  const logs = useQuery({
    queryKey: [
      "project-launch-logs",
      api.home,
      identity.project_id,
      identity.instance_id,
      logsID,
    ],
    queryFn: ({ signal }) =>
      api.get<{ stdout?: string; stderr?: string }>(
        "/manual-verification/project/logs/" + logsID,
        signal,
      ),
    enabled: online && !!run && run.id === logsID,
    refetchInterval: running ? 2500 : false,
  });

  async function command(
    action: "start" | "stop",
    payload: Record<string, string>,
  ) {
    setPending(true);
    setError("");
    try {
      const response = await submit(
        api.envelope(
          "project_launch_" + action,
          identity.project_id,
          1,
          payload,
        ),
      );
      if (response?.status !== "applied") {
        setError(response?.diagnostic || "操作待核实，请刷新查看。");
      }
    } catch (err) {
      setError(String(err));
    } finally {
      await query.refetch();
      setPending(false);
    }
  }

  return (
    <LaunchPanel
      id="mv-project-launch"
      title="启动当前项目"
      subtitle={view?.branch ? `纳管目录 · ${view.branch}` : "纳管目录"}
      icon={Play}
      expanded={expanded}
      toggle={toggle}
      status={
        run && running ? (
          <RunStatus
            status={run.status}
            label={run.status === "preparing" ? "正在准备启动" : undefined}
          />
        ) : (
          <VerificationStatus
            tone={
              !online || !view
                ? "neutral"
                : view.ready
                  ? "positive"
                  : "attention"
            }
          >
            {!online
              ? "离线"
              : view?.ready
                ? "项目就绪"
                : view
                  ? "项目未就绪"
                  : "待核实"}
          </VerificationStatus>
        )
      }
    >
      {view?.path && (
        <details className="mv-disclosure">
          <summary>纳管目录和提交</summary>
          <p>
            <code>{view.path}</code>
          </p>
          {view.head && (
            <p>
              <code>{view.head}</code>
            </p>
          )}
        </details>
      )}
      {!online && <p>项目连接离线。</p>}
      {online && query.isPending && <p>正在读取项目启动入口…</p>}
      {(error || query.error || view?.diagnostic) && (
        <p className="notice amber" role="alert">
          {error || (query.error ? String(query.error) : view?.diagnostic)}
        </p>
      )}
      {view && (
        <LaunchEntries
          entries={view.entries ?? []}
          disabled={
            !online || pending || !!query.error || !view.ready || running
          }
          start={(id) =>
            void command("start", { head: view.head, entry_id: id })
          }
        />
      )}
      {run && (
        <div className="mv-run">
          <div className="mv-line">
            <strong>{run.name}</strong>
            <RunStatus
              status={run.status}
              label={run.status === "preparing" ? "正在准备启动" : undefined}
            />
          </div>
          <p className="mv-meta">
            <code title={run.head}>{run.head.slice(0, 8)}</code>
            {run.exit_code !== undefined && <> · 退出码 {run.exit_code}</>}
          </p>
          {run.diagnostic && <p>{run.diagnostic}</p>}
          <div className="mv-actions">
            {run.status === "ready" && run.url && (
              <a href={run.url} target="_blank" rel="noreferrer">
                打开项目预览
              </a>
            )}{" "}
            {running && (
              <Button
                size="sm"
                type="button"
                disabled={!online || pending}
                onClick={() => void command("stop", { attempt_id: run.id })}
              >
                {run.status === "unknown" ? "核对项目进程" : "停止项目启动"}
              </Button>
            )}{" "}
            <Button
              size="sm"
              variant="ghost"
              type="button"
              aria-expanded={logsID === run.id}
              onClick={() => setLogsID(logsID === run.id ? "" : run.id)}
            >
              {logsID === run.id ? "隐藏项目启动日志" : "查看项目启动日志"}
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
      )}
    </LaunchPanel>
  );
}

function ReviewDetail({
  record,
  disabled,
  command,
}: {
  record: Review;
  disabled: boolean;
  command: (
    action: string,
    record: Review,
    payload: Record<string, unknown>,
  ) => Promise<boolean>;
}) {
  const { api, online, identity } = useWorkspace();
  const [logsID, setLogsID] = useState("");
  const [attemptID, setAttemptID] = useState("");
  const [selectedSource, setSource] = useState<Source>();
  const source = record.merged
    ? (selectedSource ?? record.closure?.source ?? "pr")
    : "pr";
  const definition = record.configuration.definition;
  const attempts = [...record.attempts].reverse();
  const available = attempts.filter(
    (a) =>
      a.source === source &&
      (source === "remote_head" || a.head === record.head),
  );
  const chosen =
    available.find((a) => a.id === attemptID) ??
    available.find((a) => a.head === record.closure?.head) ??
    available[0];
  const head = source === "pr" ? record.head : chosen?.head;
  const results = currentResults(record, source, head);
  const firstPending = definition.checks.find(
    (check) => !results.has(check.id),
  );
  const completed = !firstPending;
  const remaining = definition.checks.filter(
    (check) => !results.has(check.id),
  ).length;
  const inactive = disabled || record.archived || !!record.closure;
  const startDisabled = inactive || (source === "pr" && available.some(active));
  const canRecord =
    chosen?.started &&
    !chosen.removed &&
    chosen.status !== "failed" &&
    chosen.status !== "unknown";
  const logs = useQuery({
    queryKey: [
      "manual-verification-logs",
      api.home,
      identity.project_id,
      record.candidate_id,
      logsID,
    ],
    queryFn: ({ signal }) =>
      api.get<{ stdout?: string; stderr?: string }>(
        `/manual-verification/${record.candidate_id}/logs/${logsID}`,
        signal,
      ),
    enabled: online && !!logsID,
    refetchInterval: 2500,
  });
  return (
    <article className="mv-panel mv-review" aria-labelledby="mv-review-title">
      <header className="mv-review-header">
        <div className="mv-line">
          <h2 id="mv-review-title">
            PR #{prNumber(record.pr_url)} · 任务 {record.task_id}
          </h2>
          {reviewStatus(record)}
        </div>
        <div className="mv-actions mv-meta">
          <SafeLink url={record.pr_url}>打开 PR</SafeLink>
          <span>
            {record.merged ? "已合入" : record.draft ? "草稿" : "Ready"}
          </span>
        </div>
      </header>
      <section className="mv-review-section" aria-labelledby="mv-run-title">
        <h3 id="mv-run-title">运行现场</h3>
        {record.closure && (
          <p className="mv-meta">
            待办已关闭 · {sources[record.closure.source]} ·{" "}
            <code>{record.closure.head}</code>
            {" · "}
            {record.closure.actor} ·{" "}
            {new Date(record.closure.at).toLocaleString()}
          </p>
        )}
        {record.merged && (
          <label className="mv-fields">
            核验来源
            <select
              value={source}
              onChange={(e) => {
                setSource(e.target.value as Source);
                setAttemptID("");
              }}
            >
              <option value="pr">{sources.pr}</option>
              <option value="remote_head">{sources.remote_head}</option>
            </select>
          </label>
        )}
        {record.diagnostic && <p role="status">{record.diagnostic}</p>}
        <details className="mv-disclosure">
          <summary>
            {sources[source]} ·{" "}
            {head ? <code>{head.slice(0, 8)}</code> : "将在启动时读取"}
          </summary>
          {head && (
            <p>
              <code>{head}</code>
            </p>
          )}
          <p>启动参数</p>
          <pre>
            <code>
              {JSON.stringify([
                definition.launch.program,
                ...definition.launch.args,
              ])}
            </code>
          </pre>
          <p>
            工作目录 <code>{definition.launch.cwd || "."}</code>
          </p>
        </details>
        <Button
          size="sm"
          variant={startDisabled ? "outline" : "default"}
          disabled={startDisabled}
          onClick={async () => {
            if (await command("start", record, { head: record.head, source })) {
              setAttemptID("");
            }
          }}
        >
          {source === "remote_head"
            ? "在远端 Head 创建 worktree 并启动"
            : attempts.some((a) => a.source === "pr" && a.head !== record.head)
              ? "启动最新版本"
              : "创建 worktree 并启动"}
        </Button>
        <div className="mv-attempts">
          {attempts.map((a, index) => {
            const content = (
              <div key={a.id} className="mv-run">
                <div className="mv-line">
                  <strong>
                    {sources[a.source]} · <code>{a.head.slice(0, 8)}</code>
                  </strong>
                  <RunStatus status={a.status} />
                </div>
                <details className="mv-disclosure">
                  <summary>提交、目录和启动参数</summary>
                  <p className="mv-meta">
                    <code>{a.head}</code>
                    {a.removed && " · worktree 已移除"}
                    {a.exit_code !== undefined && ` · 退出码 ${a.exit_code}`}
                  </p>
                  <p>
                    <code>{a.worktree}</code>
                  </p>
                  {a.program && (
                    <>
                      <pre>
                        <code>
                          {JSON.stringify([a.program, ...(a.args || [])])}
                        </code>
                      </pre>
                      <p>
                        <code>{a.cwd}</code>
                      </p>
                    </>
                  )}
                </details>
                {a.diagnostic && <p>{a.diagnostic}</p>}
                <div className="mv-actions">
                  {a.url && a.status === "ready" && (
                    <SafeLink url={a.url}>打开预览</SafeLink>
                  )}
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-expanded={logsID === a.id}
                    onClick={() => setLogsID(logsID === a.id ? "" : a.id)}
                  >
                    {logsID === a.id ? "收起日志" : "查看日志"}
                  </Button>
                  {active(a) && a.status !== "unknown" && (
                    <Button
                      size="sm"
                      disabled={disabled}
                      onClick={() =>
                        void command("stop", record, {
                          head: a.head,
                          attempt_id: a.id,
                        })
                      }
                    >
                      停止
                    </Button>
                  )}
                  {!active(a) && !a.removed && (
                    <Button
                      size="sm"
                      disabled={disabled}
                      onClick={() =>
                        void command("remove", record, {
                          head: a.head,
                          attempt_id: a.id,
                        })
                      }
                    >
                      移除 worktree
                    </Button>
                  )}
                </div>
                {logsID === a.id && (
                  <div className="mv-logs">
                    <pre>
                      {[logs.data?.stdout, logs.data?.stderr]
                        .filter(Boolean)
                        .join("\n") || "暂无输出"}
                    </pre>
                    {logs.error && <p role="alert">{String(logs.error)}</p>}
                  </div>
                )}
              </div>
            );
            if (index === 0 || a.id === chosen?.id || active(a)) return content;
            return (
              <details key={a.id} className="mv-disclosure">
                <summary>
                  {sources[a.source]} · <code>{a.head.slice(0, 8)}</code> ·{" "}
                  {statuses[a.status] || a.status}
                </summary>
                {content}
              </details>
            );
          })}
        </div>
        {available.length > 1 && (
          <label className="mv-fields">
            核验现场
            <select
              value={chosen?.id}
              onChange={(e) => setAttemptID(e.target.value)}
            >
              {available.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.head} · {statuses[a.status] || a.status} · {a.worktree}
                </option>
              ))}
            </select>
          </label>
        )}
      </section>
      <section
        className="mv-review-section"
        aria-labelledby="mv-checklist-title"
      >
        <div className="mv-line">
          <h3 id="mv-checklist-title">核验清单</h3>
          <span className="mv-meta">
            {definition.checks.length - remaining} / {definition.checks.length}{" "}
            已记录
          </span>
        </div>
        {definition.checks.length > 0 && (
          <progress
            aria-label="已记录的核验项"
            value={definition.checks.length - remaining}
            max={definition.checks.length}
          />
        )}
        <div className="mv-checklist">
          {definition.checks.map((check) => (
            <CheckForm
              key={`${source}:${head}:${check.id}`}
              check={check}
              latest={results.get(check.id)}
              expanded={firstPending?.id === check.id}
              disabled={inactive || !canRecord}
              save={(value, notes, evidence) =>
                command("result", record, {
                  head,
                  attempt_id: chosen?.id,
                  check_id: check.id,
                  outcome: value,
                  notes,
                  evidence,
                })
              }
            />
          ))}
        </div>
      </section>
      <footer className="mv-review-footer">
        <div className="mv-line">
          {!record.closure && !record.archived && (
            <span className="mv-meta">
              {remaining ? `还剩 ${remaining} 项待记录` : "所有核验项均已记录"}
            </span>
          )}
          {record.closure ? (
            <Button
              disabled={disabled || record.archived}
              onClick={() =>
                void command("reopen", record, { head: record.closure?.head })
              }
            >
              重新打开核验待办
            </Button>
          ) : (
            !record.archived && (
              <Button
                size="sm"
                variant="default"
                disabled={inactive || !head || !completed}
                onClick={() => void command("close", record, { head, source })}
              >
                标记核验完成并关闭
              </Button>
            )
          )}
        </div>
        {record.results.length > 0 && (
          <details className="mv-disclosure mv-review-history">
            <summary>核验历史</summary>
            <ul>
              {[...record.results].reverse().map((result, i) => (
                <li key={i}>
                  {result.check_id} · {outcome(result)} ·{" "}
                  {sources[result.source]} · <code>{result.head}</code> ·{" "}
                  {result.actor} · {new Date(result.at).toLocaleString()}{" "}
                  {result.notes}{" "}
                  {result.evidence?.map((link) => (
                    <SafeLink key={link} url={link}>
                      证据
                    </SafeLink>
                  ))}
                </li>
              ))}
            </ul>
          </details>
        )}
      </footer>
    </article>
  );
}

function CheckForm({
  check,
  latest,
  expanded,
  disabled,
  save,
}: {
  check: Check;
  latest?: Result;
  expanded: boolean;
  disabled: boolean;
  save: (
    result: "passed" | "failed",
    notes: string,
    evidence: string[],
  ) => Promise<boolean>;
}) {
  const [notes, setNotes] = useState("");
  const [links, setLinks] = useState("");
  async function record(result: "passed" | "failed") {
    if (
      await save(
        result,
        notes,
        links
          .split(/\r?\n/)
          .map((link) => link.trim())
          .filter(Boolean),
      )
    ) {
      setNotes("");
      setLinks("");
    }
  }
  return (
    <details className="mv-check" open={expanded}>
      <summary>
        <h4 aria-label={`${check.id} ${check.title} · ${outcome(latest)}`}>
          <ChevronRight size={15} aria-hidden="true" />
          <span>
            <code>{check.id}</code> {check.title}
          </span>
        </h4>
        <VerificationStatus
          tone={
            latest?.outcome === "passed"
              ? "positive"
              : latest?.outcome === "failed"
                ? "negative"
                : "attention"
          }
        >
          {outcome(latest)}
        </VerificationStatus>
      </summary>
      <div className="mv-check-body">
        <ol>
          {check.steps.map((step, i) => (
            <li key={i}>{step}</li>
          ))}
        </ol>
        <p className="mv-expected">
          <strong>预期结果</strong>
          {check.expected}
        </p>
        <div className="mv-check-fields">
          <label className="mv-fields">
            核验备注
            <textarea
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
            />
          </label>
          <label className="mv-fields">
            证据链接（每行一个）
            <textarea
              value={links}
              onChange={(e) => setLinks(e.target.value)}
            />
          </label>
        </div>
        <div className="mv-actions">
          <Button
            size="sm"
            variant="default"
            disabled={disabled}
            onClick={() => void record("passed")}
          >
            记录通过
          </Button>
          <Button
            size="sm"
            disabled={disabled || !notes.trim()}
            onClick={() => void record("failed")}
          >
            记录失败
          </Button>
        </div>
      </div>
    </details>
  );
}
