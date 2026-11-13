import type { SteerAgentConfig } from "../../shared/api/execution-configs";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Navigate, useSearchParams } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { Button } from "../../shared/ui/button";
import { Roadmaps, roadmapQueryKey, type RoadmapTask } from "./roadmaps";
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
  kind?: "task";
  goal?: string;
  card?: TaskCard;
  dag_task_id?: string;
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
type TaskCard = {
  goal: string;
  scope: string;
  constraints: string;
  acceptance: string;
  predecessors?: string[];
  provides_tests?: string[];
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
  const [planningKind, setPlanningKind] = useState<"task" | "roadmap">("task");
  const [title, setTitle] = useState("");
  const [file, setFile] = useState("");
  const [goal, setGoal] = useState("");
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
        steer_agent: SteerAgentConfig;
        unavailable_reason?: string;
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
    <section className="planning-page" aria-label="任务规划">
      <header className="planning-header">
        <h1>任务规划</h1>
        <Button
          variant="default"
          disabled={!online || !list.data?.available}
          onClick={() => {
            setCreating(true);
            setParams({});
          }}
        >
          新建规划
        </Button>
        <Button
          disabled={
            sending ||
            !list.data?.tasks.some((item) => item.can_rebase)
          }
          onClick={() => void act("rebase_all", {}, true)}
        >
          全部 Rebase
        </Button>
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
        <p role="status">{list.data.unavailable_reason}</p>
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
                {item.kind === "task" ? "单个任务" : "Roadmap"} ·{" "}
                {item.busy ? "处理中" : statuses[item.status] || item.status}
              </small>
            </Link>
          ))}
        </nav>
        <div className="planning-detail">
          {creating ? (
            <form
              onSubmit={(event) => {
                event.preventDefault();
                void act(
                  "create",
                  {
                    title,
                    kind: planningKind,
                    file: planningKind === "roadmap" ? file : "",
                    goal,
                  },
                  true,
                );
              }}
            >
              <label>
                规划类型
                <select
                  value={planningKind}
                  onChange={(event) =>
                    setPlanningKind(
                      event.target.value === "task" ? "task" : "roadmap",
                    )
                  }
                >
                  <option value="task">规划单个任务</option>
                  <option value="roadmap">规划 Roadmap</option>
                </select>
              </label>
              <label>
                名称
                <input
                  required
                  value={title}
                  onChange={(event) => setTitle(event.target.value)}
                />
              </label>
              {planningKind === "roadmap" && (
                <label>
                  草稿文件名
                  <input
                    required
                    placeholder="roadmap-topic.md"
                    value={file}
                    onChange={(event) => setFile(event.target.value)}
                  />
                </label>
              )}
              <label>
                规划目标
                <textarea
                  required
                  value={goal}
                  onChange={(event) => setGoal(event.target.value)}
                />
              </label>
              {list.data?.steer_agent && (
                <p>
                  SteerAgent：{list.data.steer_agent.model} /{" "}
                  {list.data.steer_agent.reasoning_effort}
                </p>
              )}
              <button disabled={sending || !list.data?.available}>
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
                  {task.kind === "task" ? "复制规划链接" : "复制草稿链接"}
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
                <Link to={project + "/status"}>前往项目状态</Link>
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
                  {history.isError && (
                    <p className="notice amber" role="status">
                      {history.data
                        ? "对话历史刷新失败，显示上次结果。"
                        : "对话历史读取失败。"}
                    </p>
                  )}
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
                {task.kind === "task" ? (
                  <section aria-label="任务卡预览">
                    <h3>任务卡预览</h3>
                    {task.card ? (
                      <dl>
                        <dt>目标</dt>
                        <dd>{task.card.goal}</dd>
                        <dt>范围</dt>
                        <dd>{task.card.scope}</dd>
                        <dt>约束</dt>
                        <dd>{task.card.constraints || "无"}</dd>
                        <dt>验收条件</dt>
                        <dd>{task.card.acceptance}</dd>
                      </dl>
                    ) : (
                      <p>{task.goal}</p>
                    )}
                  </section>
                ) : (
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
                )}
              </div>
              {task.kind === "task" && (
                <Roadmaps
                  key={task.id}
                  planning={{
                    id: task.id,
                    cardForm: (disabled, confirmed) => (
                      <TaskCardForm
                        task={task}
                        disabled={disabled}
                        confirmed={confirmed}
                      />
                    ),
                  }}
                />
              )}
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
        </div>
      </div>
    </section>
  );
}

function TaskCardForm({
  task,
  disabled,
  confirmed,
}: {
  task: Planning;
  disabled: boolean;
  confirmed?: RoadmapTask;
}) {
  const { api, identity, online, submit } = useWorkspace();
  const client = useQueryClient();
  const key = roadmapQueryKey(
    api.home,
    identity.project_id,
    identity.instance_id,
  );
  const [draft, setDraft] = useState<TaskCard>();
  const [testIDs, setTestIDs] = useState<string>();
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const card = draft ??
    (confirmed && {
      goal: confirmed.goal,
      scope: confirmed.scope,
      constraints: confirmed.constraints,
      acceptance: confirmed.acceptance,
      predecessors: confirmed.proposed_dependencies.formal,
      provides_tests: confirmed.provides_tests,
    }) ??
    task.card ?? {
      goal: task.goal || "",
      scope: "",
      constraints: "",
      acceptance: "",
    };
  const predecessors = useQuery({
    queryKey: [
      api.home,
      identity.project_id,
      identity.instance_id,
      route,
      task.id,
      "predecessors",
    ],
    queryFn: () =>
      api.get<{ id: string; title: string }[]>(
        `${route}/${task.id}/predecessors`,
      ),
    enabled: online,
    refetchInterval: online ? 5000 : false,
  });
  const available = predecessors.data || [];
  const selected = card.predecessors || [];
  const testText = testIDs ?? (card.provides_tests || []).join("\n");
  const missing = selected.filter(
    (id) => !available.some((option) => option.id === id),
  );
  const ready =
    ["ready", "closed"].includes(task.status) && !task.busy && !task.conflict;

  async function confirm() {
    if (
      disabled ||
      !ready ||
      !online ||
      sending ||
      !predecessors.isSuccess ||
      missing.length
    )
      return;
    setSending(true);
    setError("");
    try {
      const result = await submit(
        api.envelope("roadmap_task_generate", task.id, task.revision, {
          ...card,
          predecessors: selected,
          provides_tests: testText
            .split(/\r?\n/)
            .map((id) => id.trim())
            .filter(Boolean),
        }),
      );
      if (result?.status !== "applied")
        setError(result?.diagnostic || "原请求待核实");
    } catch (failure) {
      setError(String(failure));
    } finally {
      await client.invalidateQueries({ queryKey: key });
      setSending(false);
    }
  }

  return (
    <form
      aria-label="任务卡"
      onSubmit={(event) => {
        event.preventDefault();
        void confirm();
      }}
    >
      {(
        [
          ["goal", "目标"],
          ["scope", "范围"],
          ["constraints", "约束"],
          ["acceptance", "验收条件"],
        ] as const
      ).map(([field, label]) => (
        <label key={field}>
          {label}
          <textarea
            required={field !== "constraints"}
            value={card[field]}
            onChange={(event) =>
              setDraft({ ...card, [field]: event.target.value })
            }
          />
        </label>
      ))}
      {(draft || confirmed) && task.card && (
        <button
          type="button"
          onClick={() =>
            setDraft({
              ...task.card!,
              predecessors: selected,
              provides_tests: card.provides_tests,
            })
          }
        >
          采用最新任务卡
        </button>
      )}
      <fieldset>
        <legend>已有 DAG 前置</legend>
        {predecessors.isPending && <p>正在读取 DAG…</p>}
        {predecessors.isError && (
          <p role="alert">
            读取 DAG 前置失败。
            <button type="button" onClick={() => void predecessors.refetch()}>
              重试
            </button>
          </p>
        )}
        {predecessors.isSuccess && !available.length && (
          <p>当前没有可选前置。</p>
        )}
        {[
          ...available,
          ...missing.map((id) => ({ id, title: "已不可用" })),
        ].map((option) => (
          <label key={option.id}>
            <input
              type="checkbox"
              checked={selected.includes(option.id)}
              onChange={(event) =>
                setDraft({
                  ...card,
                  predecessors: event.target.checked
                    ? [...selected, option.id]
                    : selected.filter((id) => id !== option.id),
                })
              }
            />
            #{option.id} {option.title}
          </label>
        ))}
      </fieldset>
      <details>
        <summary>新增测试登记</summary>
        <label>
          本任务将新增的测试 ID（每行一个）
          <textarea
            value={testText}
            onChange={(event) => setTestIDs(event.target.value)}
          />
        </label>
      </details>
      {error && <p role="alert">{error}</p>}
      <button
        disabled={
          disabled ||
          !ready ||
          !online ||
          sending ||
          !predecessors.isSuccess ||
          !!missing.length
        }
      >
        {sending ? "正在提交…" : "确认并生成 Spec / Plan"}
      </button>
    </form>
  );
}
