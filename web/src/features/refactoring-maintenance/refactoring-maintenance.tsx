import { useEffect, useRef, useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { Button } from "../../shared/ui/button";
import { SafeLink } from "../../shared/ui/status";
import { useActivityFeedback } from "../activities/feedback/feedback";
import { NewMaintenance } from "./new-maintenance";
import "./refactoring-maintenance.css";

type Input = {
  direction: string;
  requirements: string;
  profile: string;
  strategy: string;
  budget: { dollars?: string; minutes?: string; rounds?: number };
};
export type MaintenanceRecord = {
  id: string;
  revision: number;
  phase: string;
  input: Input;
  direction_name: string;
  direction_prompt?: string;
  round: number;
  active: boolean;
  source_commit: string;
  source_branch?: string;
  environment_name?: string;
  profile_name?: string;
  wait_reason?: string;
  enqueued_at?: string;
  updated_at?: string;
  created_at?: string;
  slot?: boolean;
  summary?: string;
  diagnostic?: string;
  elapsed_seconds: number;
  amount?: string;
  cost_complete: boolean;
  wall_seconds?: number;
  cost_breakdown?: {
    progress: { amount?: string };
    closing: { amount?: string };
  };
  report_stale?: boolean;
  question?: {
    id: string;
    prompt: string;
    facts: string[];
    attempts: string[];
    options: { id: string; label: string; impact: string }[];
    recommended_option_id: string;
    recommendation_reason: string;
  };
  pr?: { url: string };
  can_finish: boolean;
  can_stop: boolean;
  can_conflict: boolean;
  can_review?: boolean;
  operations?: {
    id: string;
    purpose: string;
    round: number;
    state: string;
    feedback: string;
    prompt: string;
    run_id?: string;
  }[];
  reports?: {
    body: string;
    head: string;
    base: string;
    created_at: string;
    recommendation: "merge" | "revise" | "insufficient_evidence";
  }[];
};
type Summary = Pick<
  MaintenanceRecord,
  | "id"
  | "direction_name"
  | "phase"
  | "environment_name"
  | "round"
  | "slot"
  | "enqueued_at"
  | "updated_at"
  | "created_at"
  | "summary"
  | "wait_reason"
  | "elapsed_seconds"
  | "wall_seconds"
>;

type Index = {
  ongoing: Summary[];
  history: Summary[];
  history_next?: string;
  slots_used: number;
  slot_limit: number;
};

const phases: Record<string, string> = {
  queued: "排队中",
  submission_unknown: "提交结果待核实",
  first: "首轮调查中",
  select: "准备中",
  report: "PR 合入评审中",
  execute: "Runner 执行中",
  audit: "独立审计中",
  finish: "收尾中",
  verifying: "核验交付",
  waiting_input: "待答复",
  stopping: "停止待结算",
  stopped: "已停止",
  blocked: "已阻断",
  delivered: "已交付",
};

const narrowScreen = "(max-width: 760px)";

export function RefactoringMaintenance() {
  const { api, identity, online, submit, requests, pendingRequests } =
    useWorkspace();
  const feedback = useActivityFeedback();
  const [params, setParams] = useSearchParams();
  const selected = params.get("action") ?? "";
  const [opened, setOpened] = useState(false);
  const [startReceipt, setStartReceipt] = useState("");
  const [startFrom, setStartFrom] = useState("");
  const [accepted, setAccepted] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [controlRequests, setControlRequests] = useState<
    Record<string, string>
  >({});
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [sending, setSending] = useState<string[]>([]);
  const inflight = useRef(new Set<string>());
  const newButton = useRef<HTMLButtonElement>(null);
  const list = useRef<HTMLElement>(null);
  const detailHeading = useRef<HTMLDivElement>(null);
  const listPosition = useRef(0);
  const initialSelection = useRef(false);
  const scope = [
    api.home,
    identity.project_id,
    identity.instance_id,
    "refactoring",
  ];
  const index = useInfiniteQuery({
    queryKey: scope,
    initialPageParam: "",
    queryFn: ({ signal, pageParam }) =>
      api.get<Index>(
        "/refactoring" +
          (pageParam
            ? "?" + new URLSearchParams({ history_before: pageParam })
            : ""),
        signal,
      ),
    getNextPageParam: (page) => page.history_next || undefined,
    enabled: online,
    refetchInterval: 2000,
  });
  const detail = useQuery({
    queryKey: [...scope, selected],
    queryFn: ({ signal }) =>
      api.get<MaintenanceRecord>("/refactoring/" + selected, signal),
    enabled: online && /^[a-f0-9]{32}$/.test(selected),
    retry: false,
    refetchInterval: 1500,
  });
  const first = index.data?.pages[0];
  const ongoing = first?.ongoing ?? [];
  const active = new Set(ongoing.map(({ id }) => id));
  const waiting = ongoing
    .filter((r) => r.phase === "waiting_input")
    .sort(
      (a, b) =>
        (a.updated_at ?? "").localeCompare(b.updated_at ?? "") ||
        a.id.localeCompare(b.id),
    );
  const progressing = ongoing
    .filter((r) => r.phase !== "waiting_input")
    .sort((a, b) => {
      if (!!a.slot !== !!b.slot) return a.slot ? -1 : 1;
      return a.slot
        ? (b.updated_at ?? "").localeCompare(a.updated_at ?? "") ||
            a.id.localeCompare(b.id)
        : (a.enqueued_at ?? "").localeCompare(b.enqueued_at ?? "") ||
            a.id.localeCompare(b.id);
    });
  const history = [
    ...new Map(
      (index.data?.pages ?? [])
        .flatMap((page) => page.history)
        .filter((r) => !active.has(r.id))
        .map((r) => [r.id, r] as const),
    ).values(),
  ];
  const suggested = waiting[0]?.id ?? progressing[0]?.id ?? history[0]?.id;

  useEffect(() => {
    if (initialSelection.current) return;
    if (
      selected ||
      (typeof matchMedia === "function" && matchMedia(narrowScreen).matches)
    ) {
      initialSelection.current = true;
      return;
    }
    if (suggested) {
      initialSelection.current = true;
      setParams({ action: suggested }, { replace: true });
    }
  }, [selected, suggested, setParams]);

  useEffect(() => {
    if (!selected && list.current)
      list.current.scrollTop = listPosition.current;
  }, [selected]);

  const journalReceipt = requests?.find(
    (row) =>
      row.envelope.request_id === startReceipt &&
      row.envelope.instance_id === identity.instance_id &&
      row.envelope.project_id === identity.project_id,
  )?.response;

  const openAction = (id: string) => {
    listPosition.current = list.current?.scrollTop ?? listPosition.current;
    initialSelection.current = true;
    setParams({ action: id });
    if (typeof matchMedia === "function" && matchMedia(narrowScreen).matches) {
      requestAnimationFrame(() => detailHeading.current?.focus());
    }
  };
  const closeSheet = () => {
    setOpened(false);
    requestAnimationFrame(() => newButton.current?.focus());
  };
  const start = async (payload: Record<string, unknown>) => {
    const envelope = api.envelope(
      "refactoring_start",
      identity.repository_identity,
      1,
      payload,
    );
    setStartReceipt(envelope.request_id);
    setStartFrom(selected);
    feedback.notify("重构与维护");
    return submit(envelope);
  };
  const onAccepted = (id: string) => {
    setStartReceipt("");
    void index.refetch();
    if (opened && selected === startFrom) {
      setOpened(false);
      openAction(id);
    } else {
      setOpened(false);
      setAccepted(id);
    }
  };
  const current = detail.data;
  const questionKey = current?.question
    ? current.id + "/" + current.question.id
    : "";
  const command = async (
    action: string,
    id: string,
    revision: number,
    payload: Record<string, unknown>,
  ) => {
    if (
      !online ||
      inflight.current.has(id) ||
      !detail.isSuccess ||
      current?.id !== id
    )
      return;
    inflight.current.add(id);
    setSending((items) => [...items, id]);
    setErrors((items) => ({ ...items, [id]: "" }));
    feedback.notify("重构与维护");
    try {
      const envelope = api.envelope(action, id, revision, payload);
      setControlRequests((items) => ({ ...items, [id]: envelope.request_id }));
      const result = await submit(envelope);
      if (result?.status !== "applied") {
        setErrors((items) => ({
          ...items,
          [id]: result?.diagnostic || "操作结果待核实，请查看原请求",
        }));
      }
      void index.refetch();
      void detail.refetch();
    } catch (err) {
      setErrors((items) => ({ ...items, [id]: String(err) }));
    } finally {
      inflight.current.delete(id);
      setSending((items) => items.filter((item) => item !== id));
    }
  };
  const pending = pendingRequests?.().some((r) => r.target_id === selected);
  const controlReceipt = requests?.find(
    (row) => row.envelope.request_id === controlRequests[selected],
  )?.response;
  const actionError =
    controlReceipt?.status === "applied"
      ? ""
      : controlReceipt?.diagnostic || errors[selected];
  const actionDisabled =
    !online || !detail.isSuccess || sending.includes(selected) || !!pending;
  const renderRows = (rows: Summary[]) =>
    rows.length ? (
      <ul className="refactoring-list">
        {rows.map((r) => (
          <li key={r.id}>
            <button
              type="button"
              data-action={r.id}
              aria-current={selected === r.id ? "true" : undefined}
              onClick={() => openAction(r.id)}
            >
              <strong>{r.direction_name}</strong>
              <span>
                {phases[r.phase] ?? r.phase} · 第 {r.round} 轮
              </span>
              <span>
                {r.environment_name} · {r.id.slice(0, 8)}
              </span>
              <span>{r.wait_reason || r.summary}</span>
              <span>
                预算已用 {duration(r.elapsed_seconds)} · 总历时{" "}
                {duration(r.wall_seconds ?? 0)}
              </span>
              {r.updated_at && (
                <time dateTime={r.updated_at}>
                  {new Date(r.updated_at).toLocaleString()}
                </time>
              )}
            </button>
          </li>
        ))}
      </ul>
    ) : (
      <p className="muted">暂无行动</p>
    );
  return (
    <div className="refactoring-page">
      <header className="page-heading refactoring-heading">
        <div>
          <h1>重构与维护</h1>
          {first && (
            <p>
              推进名额 {first.slots_used} / {first.slot_limit}
              {(!online || index.isError) && "（上次已知）"}
            </p>
          )}
        </div>
        <Button
          variant="default"
          ref={newButton}
          onClick={() => {
            setAccepted("");
            setOpened(true);
          }}
        >
          新建维护行动
        </Button>
      </header>
      {accepted && (
        <p className="notice" role="status">
          维护行动已受理{" "}
          <Link
            to={{ search: "?" + new URLSearchParams({ action: accepted }) }}
          >
            查看行动
          </Link>
        </p>
      )}
      <NewMaintenance
        open={opened}
        close={closeSheet}
        onSubmit={start}
        onAccepted={onAccepted}
        receipt={journalReceipt}
        full={!!first && first.slots_used >= first.slot_limit}
      />
      <div className="refactoring-layout" data-selected={!!selected}>
        <section
          ref={list}
          className="settings-panel refactoring-panel refactoring-sidebar"
          aria-label="行动列表"
        >
          {(!online || index.isError) && (
            <p className="notice amber" role="status">
              行动列表当前待核实{first ? "，显示上次已知结果" : ""}
            </p>
          )}
          {online && index.isPending && <p role="status">正在读取行动列表</p>}
          {first && (
            <>
              <h2>进行中 {progressing.length}</h2>
              {renderRows(progressing)}
              <h2>待答复 {waiting.length}</h2>
              {renderRows(waiting)}
              <h2>维护历史</h2>
              {renderRows(history)}
              {index.hasNextPage && (
                <Button
                  variant="ghost"
                  disabled={!online || index.isFetchingNextPage}
                  onClick={() => void index.fetchNextPage()}
                >
                  加载更多历史
                </Button>
              )}
            </>
          )}
        </section>
        <div className="refactoring-detail" ref={detailHeading} tabIndex={-1}>
          {selected && (
            <Button
              className="refactoring-back"
              variant="ghost"
              onClick={() => {
                initialSelection.current = true;
                setParams({});
                requestAnimationFrame(() =>
                  list.current
                    ?.querySelector<HTMLButtonElement>(
                      `[data-action="${selected}"]`,
                    )
                    ?.focus(),
                );
              }}
            >
              返回行动列表
            </Button>
          )}
          {selected &&
            (!online || detail.isError || !/^[a-f0-9]{32}$/.test(selected)) && (
              <p className="notice amber" role="status">
                {current
                  ? "维护详情当前待核实，显示上次已知结果"
                  : "无法读取此行动，请返回行动列表"}
              </p>
            )}
          {selected && online && !current && detail.isFetching && (
            <p role="status">正在读取维护详情</p>
          )}
          {!selected && <p className="muted">选择一条行动查看详情</p>}
          {current && (
            <MaintenanceDetail
              current={current}
              disabled={actionDisabled}
              command={command}
              answer={answers[questionKey] ?? ""}
              error={actionError}
              setAnswer={(value) =>
                setAnswers((items) => ({ ...items, [questionKey]: value }))
              }
            />
          )}
        </div>
      </div>
    </div>
  );
}

function duration(seconds: number) {
  return seconds < 60
    ? Math.floor(seconds) + " 秒"
    : Math.floor(seconds / 60) + " 分钟";
}

function MaintenanceDetail({
  current,
  disabled,
  command,
  answer,
  error,
  setAnswer,
}: {
  current: MaintenanceRecord;
  disabled: boolean;
  command: (
    action: string,
    id: string,
    revision: number,
    payload: Record<string, unknown>,
  ) => Promise<void>;
  answer: string;
  error?: string;
  setAnswer: (value: string) => void;
}) {
  const [choice, setChoice] = useState({ question: "", option: "" });
  const option = choice.question === current.question?.id ? choice.option : "";
  return (
    <section className="settings-panel refactoring-panel" aria-label="维护详情">
      <h2>{current.direction_name}</h2>
      <p>{phases[current.phase] ?? current.phase}</p>
      {current.wait_reason && <p role="status">{current.wait_reason}</p>}
      <p>
        {current.environment_name} · 第 {current.round} 轮 · 预算已用{" "}
        {duration(current.elapsed_seconds)}
      </p>
      <p>
        起始提交 {current.source_branch} <code>{current.source_commit}</code>
      </p>
      {current.direction_prompt && (
        <details>
          <summary>方向提示词</summary>
          <pre>{current.direction_prompt}</pre>
        </details>
      )}
      <p>
        策略：
        {
          {
            cautious: "谨慎决策",
            autonomous: "完全自主",
            aggressive: "激进决策",
          }[current.input.strategy]
        }{" "}
        · 执行配置：{current.profile_name || current.input.profile}
      </p>
      <p>
        预算：
        {current.input.budget.dollars
          ? "$" + current.input.budget.dollars
          : "美元未设置"}{" "}
        ·{" "}
        {current.input.budget.minutes
          ? current.input.budget.minutes + " 分钟"
          : "时间未设置"}{" "}
        ·{" "}
        {current.input.budget.rounds
          ? current.input.budget.rounds + " 轮"
          : "轮次未设置"}
      </p>
      <p>
        已覆盖参考费用：
        {current.amount == null ? "待采集" : "$" + current.amount}
        {!current.cost_complete && "（采集不完整）"}
      </p>
      {current.cost_breakdown && (
        <p>
          推进参考费用：
          {current.cost_breakdown.progress.amount == null
            ? "待采集"
            : "$" + current.cost_breakdown.progress.amount}{" "}
          · 收尾参考费用：
          {current.cost_breakdown.closing.amount == null
            ? "待采集"
            : "$" + current.cost_breakdown.closing.amount}
        </p>
      )}
      {current.wall_seconds !== undefined && (
        <p>总历时 {duration(current.wall_seconds)}（含等待与离线）</p>
      )}
      <p>
        费用包含主会话与已登记内部调用；不覆盖子
        Agent、工具内部模型和不可观测供应商调用。
      </p>
      {error && (
        <p className="notice amber" role="alert">
          {error}
        </p>
      )}
      {current.summary && <p>{current.summary}</p>}
      {current.diagnostic && (
        <p className="notice amber" role="alert">
          {current.diagnostic}
        </p>
      )}
      <div className="refactoring-actions">
        {current.can_finish && (
          <Button
            disabled={disabled}
            onClick={() =>
              void command(
                "refactoring_finish",
                current.id,
                current.revision,
                {},
              )
            }
          >
            结束并交付
          </Button>
        )}
        {current.can_stop && (
          <Button
            variant="destructive"
            disabled={disabled}
            onClick={() =>
              void command("refactoring_stop", current.id, current.revision, {})
            }
          >
            立即停止
          </Button>
        )}
        {current.can_conflict && (
          <Button
            disabled={disabled}
            onClick={() =>
              void command(
                "refactoring_conflict",
                current.id,
                current.revision,
                {},
              )
            }
          >
            处理冲突并重新评审
          </Button>
        )}
        {current.can_review && (
          <Button
            variant="outline"
            disabled={disabled}
            onClick={() =>
              void command(
                "refactoring_review",
                current.id,
                current.revision,
                {},
              )
            }
          >
            重新评审
          </Button>
        )}
        {current.pr && <SafeLink url={current.pr.url}>查看 PR</SafeLink>}
      </div>
      {current.question && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (disabled || !option) return;
            void command("refactoring_answer", current.id, current.revision, {
              question_id: current.question!.id,
              option_id: option,
              note: answer,
            });
          }}
        >
          <p className="refactoring-pre">{current.question.prompt}</p>
          <ul>
            {[...current.question.facts, ...current.question.attempts].map(
              (fact, index) => (
                <li key={index}>{fact}</li>
              ),
            )}
          </ul>
          <fieldset>
            <legend>选择处理方式</legend>
            {current.question.options.map((choice) => (
              <div key={choice.id}>
                <label>
                  <input
                    type="radio"
                    name="maintenance-option"
                    value={choice.id}
                    checked={option === choice.id}
                    onChange={() =>
                      setChoice({
                        question: current.question!.id,
                        option: choice.id,
                      })
                    }
                  />
                  {choice.label}
                </label>
                <p>{choice.impact}</p>
                {current.question!.recommended_option_id === choice.id && (
                  <p>推荐 · {current.question!.recommendation_reason}</p>
                )}
              </div>
            ))}
          </fieldset>
          <label>
            补充说明（可选）
            <textarea
              maxLength={32768}
              value={answer}
              onChange={(e) => setAnswer(e.target.value)}
            />
          </label>
          <Button variant="default" disabled={disabled || !option}>
            答复并继续
          </Button>
        </form>
      )}
      {current.reports?.map((r, i) => (
        <article key={i}>
          <h3>PR 合入评审 {i + 1}</h3>
          <p>
            head {r.head} · base {r.base}
          </p>
          <p>
            {
              {
                merge: "建议合入",
                revise: "建议修改后再审",
                insufficient_evidence: "证据不足",
              }[r.recommendation]
            }
          </p>
          {current.report_stale && i === current.reports!.length - 1 && (
            <p>此评审已过期或正在复核。</p>
          )}
          <div className="refactoring-pre">{r.body}</div>
        </article>
      ))}
      <h3>轮次与执行反馈</h3>
      {current.operations?.map((o) => (
        <details key={o.id}>
          <summary>
            第 {o.round} 轮 · {o.purpose} · {o.state}
          </summary>
          <p>Runner：{o.run_id || "尚未提交"}</p>
          <h4>提示词</h4>
          <pre>{o.prompt}</pre>
          <h4>反馈</h4>
          <pre>
            {o.feedback || (o.state === "complete" ? "未返回文字" : "等待反馈")}
          </pre>
        </details>
      ))}
    </section>
  );
}
