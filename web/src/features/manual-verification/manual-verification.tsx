import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { Button } from "../../shared/ui/button";
import { SafeLink } from "../../shared/ui/status";

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
      launch: { program: string; args: string[]; cwd?: string; url?: string };
      checks: Check[];
    };
  };
  attempts: Attempt[];
  results: Result[];
};

const statuses: Record<string, string> = {
  preparing: "正在准备 worktree",
  running: "进程运行中",
  ready: "预览可访问",
  exited: "进程已退出",
  stopped: "已停止",
  failed: "启动或运行失败",
  unknown: "原进程待核实",
};
const active = (a: Attempt) =>
  ["preparing", "running", "ready", "unknown"].includes(a.status);
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

export default function ManualVerification() {
  const { api, identity, online, submit } = useWorkspace();
  const [params, setParams] = useSearchParams();
  const [history, setHistory] = useState(false);
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
  const record = rows.find((row) => row.candidate_id === selected);

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
    <section>
      <h1>人工核验</h1>
      <label>
        <input
          type="checkbox"
          checked={history}
          onChange={(e) => setHistory(e.target.checked)}
        />{" "}
        显示已关闭或已归档的待办
      </label>
      {(error || records.error) && (
        <p className="notice amber" role="alert">
          {error || String(records.error)}
        </p>
      )}
      {visible.length === 0 && (
        <p>{records.isPending ? "正在读取核验任务…" : "暂无待核验 PR。"}</p>
      )}
      <ul>
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
                onClick={() => {
                  setParams({ candidate: row.candidate_id });
                  setError("");
                }}
              >
                任务 {row.task_id} · PR #{row.pr_url.split("/").at(-1)} ·{" "}
                {completed}/{row.configuration.definition.checks.length} 已记录
              </button>
              {row.closure && " 已关闭"}
              {row.archived && " 已归档"}
              {row.merged && " · PR 已合入"}
            </li>
          );
        })}
      </ul>
      {record && (
        <ReviewDetail
          key={record.candidate_id}
          record={record}
          disabled={pending || !online}
          command={command}
        />
      )}
    </section>
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
  const completed = definition.checks.every((check) => results.has(check.id));
  const inactive = disabled || record.archived || !!record.closure;
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
    <article>
      <h2>任务 {record.task_id}</h2>
      <p>
        <SafeLink url={record.pr_url}>打开 PR</SafeLink> ·{" "}
        {record.merged ? "PR 已合入" : record.draft ? "草稿" : "Ready"}
        {record.archived && " · 待办已归档"}
      </p>
      {record.closure && (
        <p>
          待办已关闭 · {sources[record.closure.source]} ·{" "}
          <code>{record.closure.head}</code>
          {" · "}
          {record.closure.actor} ·{" "}
          {new Date(record.closure.at).toLocaleString()}
        </p>
      )}
      {record.merged && (
        <label>
          核验来源{" "}
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
      <p>
        {sources[source]} {head ? <code>{head}</code> : "将在启动时读取"}
      </p>
      {record.diagnostic && <p role="status">{record.diagnostic}</p>}
      <p>
        启动参数{" "}
        <code>
          {JSON.stringify([
            definition.launch.program,
            ...definition.launch.args,
          ])}
        </code>
      </p>
      <p>
        工作目录 <code>{definition.launch.cwd || "."}</code>
      </p>
      <Button
        disabled={inactive || (source === "pr" && available.some(active))}
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
      {attempts.map((a) => (
        <div key={a.id}>
          <p>
            {sources[a.source]} · {statuses[a.status] || a.status} ·{" "}
            <code>{a.head}</code>
            {a.removed && " · worktree 已移除"}
            {a.exit_code !== undefined && ` · 退出码 ${a.exit_code}`}
          </p>
          <p>
            <code>{a.worktree}</code>
          </p>
          {a.program && (
            <p>
              <code>{JSON.stringify([a.program, ...(a.args || [])])}</code> ·{" "}
              <code>{a.cwd}</code>
            </p>
          )}
          {a.diagnostic && <p>{a.diagnostic}</p>}
          {a.url && a.status === "ready" && (
            <SafeLink url={a.url}>打开预览</SafeLink>
          )}
          <Button onClick={() => setLogsID(logsID === a.id ? "" : a.id)}>
            {logsID === a.id ? "收起日志" : "查看日志"}
          </Button>
          {active(a) && a.status !== "unknown" && (
            <Button
              disabled={disabled}
              onClick={() =>
                void command("stop", record, { head: a.head, attempt_id: a.id })
              }
            >
              停止
            </Button>
          )}
          {!active(a) && !a.removed && (
            <Button
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
          {logsID === a.id && (
            <div>
              <pre>{logs.data?.stdout}</pre>
              <pre>{logs.data?.stderr}</pre>
              {logs.error && <p role="alert">{String(logs.error)}</p>}
            </div>
          )}
        </div>
      ))}
      {available.length > 1 && (
        <label>
          核验现场{" "}
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
      {definition.checks.map((check) => (
        <CheckForm
          key={`${source}:${head}:${check.id}`}
          check={check}
          latest={results.get(check.id)}
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
            disabled={inactive || !head || !completed}
            onClick={() => void command("close", record, { head, source })}
          >
            标记核验完成并关闭
          </Button>
        )
      )}
      {record.results.length > 0 && (
        <details>
          <summary>核验历史</summary>
          <ul>
            {[...record.results].reverse().map((result, i) => (
              <li key={i}>
                {result.check_id} · {outcome(result)} · {sources[result.source]}{" "}
                · <code>{result.head}</code> · {result.actor} ·{" "}
                {new Date(result.at).toLocaleString()} {result.notes}{" "}
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
    </article>
  );
}

function CheckForm({
  check,
  latest,
  disabled,
  save,
}: {
  check: Check;
  latest?: Result;
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
    <section>
      <h3>
        {check.id} {check.title} · {outcome(latest)}
      </h3>
      <ol>
        {check.steps.map((step, i) => (
          <li key={i}>{step}</li>
        ))}
      </ol>
      <p>预期结果 {check.expected}</p>
      <label>
        核验备注
        <textarea value={notes} onChange={(e) => setNotes(e.target.value)} />
      </label>
      <label>
        证据链接（每行一个）
        <textarea value={links} onChange={(e) => setLinks(e.target.value)} />
      </label>
      <Button disabled={disabled} onClick={() => void record("passed")}>
        记录通过
      </Button>
      <Button
        disabled={disabled || !notes.trim()}
        onClick={() => void record("failed")}
      >
        记录失败
      </Button>
    </section>
  );
}
