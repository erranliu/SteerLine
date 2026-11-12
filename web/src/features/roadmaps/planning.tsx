import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, Navigate, useSearchParams } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import "./planning.css";

type Change = {
  path: string;
  before?: string;
  after?: string;
  mode?: string;
  old_mode?: string;
  unavailable?: string;
};
type PlanningQuestion = {
  id: string;
  text: string;
  options?: { id: string; label: string }[];
  allow_free_text?: boolean;
  native_questions?: PlanningQuestion[];
};
type Planning = {
  id: string;
  revision: number;
  title: string;
  path: string;
  status: string;
  body?: string;
  diagnostic?: string;
  source_id?: string;
  pr?: string;
  busy: boolean;
  can_rebase: boolean;
  can_message: boolean;
  question?: PlanningQuestion;
  conflict?: { files: string[] };
  review?: { digest: string; changes: Change[] };
};
type HistoryEntry = {
  role: string;
  text?: string;
  result?: { text?: string };
};
type HistoryPage = {
  entries: HistoryEntry[];
  has_more: boolean;
  next_cursor: string;
};

const route = "/roadmap-planning";
const statuses: Record<string, string> = {
  ready: "可继续",
  conflict: "Rebase 待处理",
  starting: "正在启动",
  running: "正在执行",
  waiting_input: "等待输入",
  awaiting_merge: "待合并",
  merged_pending_sync: "已合并，待同步",
};

function decode(body?: string) {
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(
      Uint8Array.from(atob(body || ""), (character) => character.charCodeAt(0)),
    );
  } catch {
    return "二进制文件";
  }
}

function Markdown({ body }: { body: string }) {
  return (
    <div className="planning-markdown">
      {body.split("\n").map((line, i) => {
        const heading = /^(#{1,6})\s+(.+)$/.exec(line);
        if (heading) return <strong key={i}>{heading[2]}</strong>;
        return <div key={i}>{line || "\u00a0"}</div>;
      })}
    </div>
  );
}

export default function RoadmapPlanning() {
  const { api, identity, online, submit } = useWorkspace();
  const [params, setParams] = useSearchParams();
  const [creating, setCreating] = useState(false);
  const [title, setTitle] = useState("");
  const [file, setFile] = useState("");
  const [goal, setGoal] = useState("");
  const [profile, setProfile] = useState("");
  const [message, setMessage] = useState("");
  const [answer, setAnswer] = useState<{
    question: string;
    values: Record<string, string>;
  }>();
  const [error, setError] = useState("");
  const [batch, setBatch] = useState<{
    queued?: number;
    skipped?: Record<string, string>;
  }>();
  const [sending, setSending] = useState(false);
  const [selection, setSelection] = useState<{
    digest: string;
    paths: string[];
  }>();
  const [historyExtra, setHistoryExtra] = useState<{
    task: string;
    entries: HistoryEntry[];
    next: string;
    more: boolean;
  }>();
  const key = [api.home, identity.project_id, identity.instance_id, route];
  const list = useQuery({
    queryKey: key,
    queryFn: () =>
      api.get<{
        tasks: Planning[];
        available: boolean;
        profiles: { id: string; name: string }[];
      }>(route),
    enabled: online,
    refetchInterval: online ? 2500 : false,
  });
  const id =
    params.get("task") || (creating ? "" : list.data?.tasks[0]?.id) || "";
  const detail = useQuery({
    queryKey: [...key, id],
    queryFn: () => api.get<Planning>(route + "/" + id),
    enabled: online && !!id,
    refetchInterval: online ? 2000 : false,
  });
  const task = detail.data;
  const question = task?.question;
  const fields = question?.native_questions?.length
    ? question.native_questions
    : question
      ? [question]
      : [];
  const values = answer?.question === question?.id ? answer?.values || {} : {};
  const history = useQuery({
    queryKey: [...key, id, "history"],
    queryFn: () => api.get<HistoryPage>(route + "/" + id + "/history"),
    enabled: online && !!task && !task.source_id,
    refetchInterval: online && task && !task.source_id ? 3000 : false,
  });
  const extra = historyExtra?.task === id ? historyExtra : undefined;
  const review = task?.review;
  const selected =
    review && selection?.digest === review.digest
      ? selection.paths
      : task
        ? [task.path]
        : [];
  const project = "/projects/" + identity.project_id;

  async function act(
    action: string,
    payload: Record<string, unknown> = {},
    projectAction = false,
  ) {
    if (!online || sending || (!projectAction && !task)) return;
    setSending(true);
    setError("");
    try {
      const result = await submit(
        api.envelope(
          "roadmap_planning_" + action,
          projectAction ? identity.project_id : task!.id,
          projectAction ? 1 : task!.revision,
          payload,
        ),
      );
      if (result?.status !== "applied") {
        setError(result?.diagnostic || "原请求待核实");
      } else if (action === "create") {
        const created = result.roadmap as Planning | undefined;
        if (created?.id) setParams({ task: created.id });
        setCreating(false);
        setTitle("");
        setGoal("");
        setFile("");
      } else if (action === "rebase_all") {
        const summary = result.roadmap as
          { queued?: number; skipped?: Record<string, string> } | undefined;
        setBatch(summary);
      } else if (["message", "resolve", "answer"].includes(action)) {
        setMessage("");
      }
      await Promise.all([
        list.refetch(),
        id && detail.refetch(),
        id && history.refetch(),
      ]);
    } catch (failure) {
      setError(String(failure));
    } finally {
      setSending(false);
    }
  }

  async function moreHistory() {
    const cursor = extra?.next || history.data?.next_cursor;
    if (!cursor) return;
    try {
      const page = await api.get<HistoryPage>(
        route + "/" + id + "/history?after=" + encodeURIComponent(cursor),
      );
      setHistoryExtra({
        task: id,
        entries: [...(extra?.entries || []), ...page.entries],
        next: page.next_cursor,
        more: page.has_more,
      });
    } catch (failure) {
      setError(String(failure));
    }
  }

  if (task?.source_id)
    return (
      <Navigate
        replace
        to={project + "/roadmaps?source=" + encodeURIComponent(task.source_id)}
      />
    );

  return (
    <section className="planning-page" aria-label="Roadmap 规划">
      <header className="planning-header">
        <h1>Roadmap 规划</h1>
        <button
          disabled={!online || !list.data?.available}
          onClick={() => {
            setCreating(true);
            setParams({});
          }}
        >
          新建规划
        </button>
        <button
          disabled={
            sending || !list.data?.tasks.some((item) => item.can_rebase)
          }
          onClick={() => void act("rebase_all", {}, true)}
        >
          全部 Rebase
        </button>
      </header>
      {error && <p role="alert">{error}</p>}
      {(list.error || detail.error) && (
        <p role="alert">{String(list.error || detail.error)}</p>
      )}
      {batch && (
        <div role="status">
          <p>已提交 {batch.queued ?? 0} 项。</p>
          {Object.entries(batch.skipped || {}).length > 0 && (
            <ul>
              {Object.entries(batch.skipped || {}).map(([taskID, reason]) => (
                <li key={taskID}>
                  {list.data?.tasks.find((item) => item.id === taskID)?.title ||
                    taskID}{" "}
                  · {reason}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
      {list.data && !list.data.available && (
        <p role="status">当前 Runner 缺少可写原生会话与 worktree 写入能力。</p>
      )}
      <div className="planning-layout">
        <nav aria-label="规划任务">
          {(list.data?.tasks || []).map((item) => (
            <Link
              key={item.id}
              to={"?task=" + item.id}
              aria-current={item.id === id ? "page" : undefined}
              onClick={() => {
                setCreating(false);
                setMessage("");
                setError("");
              }}
            >
              {item.title}
              <small>
                {item.busy ? "处理中" : statuses[item.status] || item.status}
              </small>
            </Link>
          ))}
        </nav>
        <main>
          {creating ? (
            <form
              onSubmit={(event) => {
                event.preventDefault();
                void act(
                  "create",
                  {
                    title,
                    file,
                    goal,
                    profile: profile || list.data?.profiles[0]?.id || "",
                  },
                  true,
                );
              }}
            >
              <label>
                名称
                <input
                  required
                  value={title}
                  onChange={(event) => setTitle(event.target.value)}
                />
              </label>
              <label>
                草稿文件名
                <input
                  required
                  placeholder="roadmap-topic.md"
                  value={file}
                  onChange={(event) => setFile(event.target.value)}
                />
              </label>
              <label>
                规划目标
                <textarea
                  required
                  value={goal}
                  onChange={(event) => setGoal(event.target.value)}
                />
              </label>
              <label>
                本机 Codex 方案
                <select
                  value={profile || list.data?.profiles[0]?.id || ""}
                  onChange={(event) => setProfile(event.target.value)}
                >
                  {(list.data?.profiles || []).map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.name}
                    </option>
                  ))}
                </select>
              </label>
              <button disabled={sending || !list.data?.profiles.length}>
                开始规划
              </button>
            </form>
          ) : task ? (
            <>
              <header className="planning-header">
                <h2>{task.title}</h2>
                <span role="status">
                  {task.busy ? "处理中" : statuses[task.status] || task.status}
                </span>
                <button
                  onClick={() => {
                    const url = new URL(window.location.href);
                    url.searchParams.set("task", task.id);
                    void navigator.clipboard
                      .writeText(url.href)
                      .catch((failure: unknown) => setError(String(failure)));
                  }}
                >
                  复制草稿链接
                </button>
                <button
                  disabled={!task.can_rebase || sending}
                  onClick={() => void act("rebase")}
                >
                  Rebase
                </button>
                {task.pr && (
                  <a href={task.pr} target="_blank" rel="noreferrer">
                    查看 PR
                  </a>
                )}
              </header>
              {task.diagnostic && <p role="status">{task.diagnostic}</p>}
              {task.status === "merged_pending_sync" && (
                <Link to={project + "/settings"}>前往项目维护</Link>
              )}
              {task.conflict && (
                <section aria-label="Rebase 冲突">
                  <ul>
                    {task.conflict.files.map((path) => (
                      <li key={path}>{path}</li>
                    ))}
                  </ul>
                  <button
                    disabled={!task.can_message || sending}
                    onClick={() => void act("resolve", { text: message })}
                  >
                    让 SteerAgent 解决冲突
                  </button>
                  <button
                    disabled={!task.can_message || sending}
                    onClick={() => void act("abort")}
                  >
                    中止 Rebase
                  </button>
                </section>
              )}
              <div className="planning-content">
                <section aria-label="规划对话">
                  <h3>规划对话</h3>
                  <div className="planning-history">
                    {[
                      ...(history.data?.entries || []),
                      ...(extra?.entries || []),
                    ]
                      .filter((entry) => entry.text || entry.result?.text)
                      .map((entry, i) => (
                        <article key={i}>
                          <small>
                            {entry.role === "user" ? "你" : "SteerAgent"}
                          </small>
                          <p>{entry.result?.text || entry.text}</p>
                        </article>
                      ))}
                  </div>
                  {(extra?.more ?? history.data?.has_more) && (
                    <button onClick={() => void moreHistory()}>更多消息</button>
                  )}
                  {question ? (
                    <>
                      {fields.map((field) => {
                        const update = (value: string) =>
                          setAnswer({
                            question: question.id,
                            values: { ...values, [field.id]: value },
                          });
                        return (
                          <label key={field.id}>
                            {field.text}
                            {field.options?.length && !field.allow_free_text ? (
                              <select
                                value={values[field.id] || ""}
                                onChange={(event) => update(event.target.value)}
                              >
                                <option value="">选择回答</option>
                                {field.options.map((option) => (
                                  <option key={option.id} value={option.label}>
                                    {option.label}
                                  </option>
                                ))}
                              </select>
                            ) : (
                              <textarea
                                value={values[field.id] || ""}
                                onChange={(event) => update(event.target.value)}
                              />
                            )}
                          </label>
                        );
                      })}
                      <button
                        disabled={
                          sending ||
                          fields.some((field) => !values[field.id]?.trim())
                        }
                        onClick={() =>
                          void act("answer", {
                            question_id: question.id,
                            text:
                              fields.length === 1
                                ? values[fields[0].id]
                                : JSON.stringify(
                                    Object.fromEntries(
                                      fields.map((field) => [
                                        field.id,
                                        [values[field.id]],
                                      ]),
                                    ),
                                  ),
                          })
                        }
                      >
                        提交回答
                      </button>
                    </>
                  ) : (
                    <>
                      <label>
                        {task.conflict ? "补充冲突取舍" : "消息"}
                        <textarea
                          value={message}
                          onChange={(event) => setMessage(event.target.value)}
                        />
                      </label>
                      <button
                        disabled={
                          sending ||
                          !message.trim() ||
                          !task.can_message ||
                          !!task.conflict
                        }
                        onClick={() => void act("message", { text: message })}
                      >
                        发送
                      </button>
                    </>
                  )}
                </section>
                <section aria-label="草稿预览">
                  <h3>草稿预览</h3>
                  <small>{task.path}</small>
                  {task.body ? (
                    <Markdown body={task.body} />
                  ) : (
                    <p>等待草稿生成。</p>
                  )}
                  <button
                    disabled={sending || !task.can_rebase || !task.body}
                    onClick={() => void act("review")}
                  >
                    审阅文件差异
                  </button>
                </section>
              </div>
              {review && (
                <section aria-label="发布审阅">
                  <h3>发布审阅</h3>
                  {review.changes.map((change) => (
                    <details key={change.path}>
                      <summary>
                        {change.path}
                        {change.unavailable && "（不可发布）"}
                      </summary>
                      {change.unavailable ? (
                        <p>{change.unavailable}</p>
                      ) : (
                        <>
                          <label>
                            <input
                              type="checkbox"
                              checked={selected.includes(change.path)}
                              disabled={task.busy || change.path === task.path}
                              onChange={(event) =>
                                setSelection({
                                  digest: review.digest,
                                  paths: event.target.checked
                                    ? [...selected, change.path]
                                    : selected.filter(
                                        (path) => path !== change.path,
                                      ),
                                })
                              }
                            />
                            纳入 PR
                          </label>
                          <div className="planning-content">
                            <pre aria-label="修改前">
                              {decode(change.before)}
                            </pre>
                            <pre aria-label="修改后">
                              {decode(change.after)}
                            </pre>
                          </div>
                        </>
                      )}
                    </details>
                  ))}
                  <button
                    disabled={
                      sending ||
                      !task.can_rebase ||
                      !review.changes.some(
                        (change) =>
                          change.path === task.path && !change.unavailable,
                      )
                    }
                    onClick={() =>
                      void act("publish", {
                        digest: review.digest,
                        paths: selected,
                      })
                    }
                  >
                    {task.pr ? "确认并更新 PR" : "确认并创建 PR"}
                  </button>
                </section>
              )}
            </>
          ) : (
            <p>
              {id
                ? "正在读取规划任务…"
                : "创建规划任务，开始与 SteerAgent 讨论。"}
            </p>
          )}
        </main>
      </div>
    </section>
  );
}
