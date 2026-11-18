import { useEffect, useRef, useState } from "react";
import {
  queryOptions,
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { requestByID } from "../../shared/api/journal";
import type { StoredRequest } from "../../shared/api/types";
import { Button } from "../../shared/ui/button";
import { useAbandonedActivities } from "../activities/abandonment-marks";
import {
  CopyInfoButton,
  formatExecutionInfo,
} from "../activities/execution-info";
import { useActivityFeedback } from "../activities/feedback/feedback";
import {
  ExecutionConversation,
  operationPurposeLabel,
  operationStateLabel,
} from "./execution-conversation";
import { NewMaintenance, type StartInput } from "./new-maintenance";
import { ManageDirections } from "./directions";
import {
  DeliveryResult,
  finishActions,
  type DeliveryStage,
  type FinishAction,
} from "./delivery-result";
import "./refactoring-maintenance.css";

type Input = Pick<StartInput, "direction" | "profile" | "strategy" | "budget"> &
  Partial<StartInput>;
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
  startup_failed?: boolean;
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
  pr?: {
    url: string;
    title?: string;
    number?: number;
    state?: string;
    merged?: boolean;
    head_revision?: string;
    base_revision?: string;
  };
  delivery_summary?: string;
  delivery_stage?: DeliveryStage;
  delivery_result?: "" | "pr" | "no_changes";
  finish_action?: FinishAction | "";
  can_finish: boolean;
  can_stop: boolean;
  can_conflict: boolean;
  can_review?: boolean;
  operations?: {
    id: string;
    purpose: string;
    round: number;
    state: string;
    workflow: "generate" | "execute";
    feedback: string;
    prompt: string;
    run_id?: string;
    run_status?: string;
    created_at?: string;
    completed_at?: string | null;
    result?: { summary?: string; question?: string } | null;
    decision_kind?: string;
    decision_summary?: string;
    report_index?: number;
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
  | "diagnostic"
  | "wait_reason"
  | "elapsed_seconds"
  | "wall_seconds"
  | "delivery_stage"
  | "delivery_result"
>;
type DebugRecord = Summary &
  Partial<Pick<MaintenanceRecord, "input" | "source_commit" | "operations">>;

type Index = {
  ongoing: Summary[];
  history: Summary[];
  history_next?: string;
  slots_used: number;
  slot_limit: number;
};

const phases: Record<string, string> = {
  start_pending: "提交待确认",
  start_accepted: "已受理，正在读取重构任务状态",
  checking_start: "正在核验启动条件",
  queued: "排队中",
  submission_unknown: "提交结果待核实",
  first: "首轮调查中",
  select: "准备中",
  report: "PR 合入评审中",
  execute: "Runner 执行中",
  audit: "独立审计中",
  finish: "正在验证并创建 PR",
  verifying: "核验交付",
  waiting_input: "待答复",
  stopping: "停止待结算",
  stopped: "已停止",
  blocked: "已阻断",
  delivered: "任务已完成",
};

function phaseLabel(
  record: Pick<
    MaintenanceRecord,
    "phase" | "delivery_stage" | "delivery_result"
  >,
) {
  if (record.phase === "delivered")
    return record.delivery_result === "pr" ? "已创建 PR" : "已完成 · 无需修改";
  if (record.phase === "blocked" && record.delivery_stage) return "交付失败";
  if (record.phase === "execute" && record.delivery_stage === "publish")
    return "正在验证并创建 PR";
  return phases[record.phase] ?? record.phase;
}

const strategyNames: Record<string, string> = {
  cautious: "谨慎决策",
  autonomous: "完全自主",
  aggressive: "激进决策",
};

const narrowScreen = "(max-width: 760px)";

type StartRequest = StoredRequest & {
  directionName?: string;
  environmentName?: string;
  unsent?: boolean;
};

const rejectedStart = (row: StartRequest) =>
  row.unsent || row.response?.status === "rejected";

export function RefactoringMaintenance() {
  const { api, identity, online, submit, requests, pendingRequests } =
    useWorkspace();
  const queryClient = useQueryClient();
  const feedback = useActivityFeedback();
  const [params, setParams] = useSearchParams();
  const selected = params.get("action") ?? "";
  const [opened, setOpened] = useState(false);
  const [managingDirections, setManagingDirections] = useState(false);
  const [form, setForm] = useState<{ key: number; input?: StartInput }>({
    key: 0,
  });
  const [starts, setStarts] = useState<Record<string, StartRequest>>({});
  const confirmedActions = useRef(new Set<string>());
  const sendingStart = useRef(false);
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
  const currentSelection = useRef("");
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
  const taskDetail = (id: string) =>
    queryOptions({
      queryKey: [...scope, id],
      queryFn: ({ signal }) =>
        api.get<MaintenanceRecord>("/refactoring/" + id, signal),
      retry: false,
    });
  const detail = useQuery({
    ...taskDetail(selected),
    enabled: online && /^[a-f0-9]{32}$/.test(selected),
    refetchInterval: 1500,
  });
  const first = index.data?.pages[0];
  const ownerOngoing = first?.ongoing ?? [];
  const ownerHistory = (index.data?.pages ?? []).flatMap(
    (page) => page.history,
  );
  const known = new Set([...ownerOngoing, ...ownerHistory].map(({ id }) => id));
  useEffect(() => {
    // Keep list projections only until the action is listed or known to have ended.
    for (const page of index.data?.pages ?? []) {
      for (const { id } of [...page.ongoing, ...page.history])
        confirmedActions.current.add(id);
    }
    if (detail.data && !detail.data.active)
      confirmedActions.current.add(detail.data.id);
  }, [index.data, detail.data]);
  const startRequests = new Map(Object.entries(starts));
  for (const row of requests ?? []) {
    const envelope = row.envelope;
    if (
      envelope.action !== "refactoring_start" ||
      envelope.project_id !== identity.project_id ||
      envelope.instance_id !== identity.instance_id
    )
      continue;
    const previous = startRequests.get(envelope.request_id);
    if (
      previous ||
      envelope.request_id === selected ||
      row.response?.status !== "applied"
    ) {
      const response = previous?.response;
      startRequests.set(envelope.request_id, {
        ...previous,
        ...row,
        unsent: false,
        response:
          response?.status === "rejected" || response?.status === "applied"
            ? response
            : (row.response ?? response),
      });
    }
  }
  const submitting = [...startRequests.values()].some(
    (row) => !rejectedStart(row) && row.response?.status !== "applied",
  );
  const startSummary = ([id, row]: [string, StartRequest]): Summary => ({
    id,
    direction_name: row.directionName ?? String(row.envelope.payload.direction),
    environment_name: row.environmentName,
    phase:
      row.response?.status === "applied" ? "start_accepted" : "start_pending",
    round: 0,
    created_at: row.created_at,
    enqueued_at: row.created_at,
    elapsed_seconds: 0,
  });
  const projectedStarts = [...startRequests.entries()].flatMap(([id, row]) => {
    if (known.has(id) || confirmedActions.current.has(id) || rejectedStart(row))
      return [];
    const saved = detail.data;
    if (saved?.id === id) return saved.active ? [saved] : [];
    return [startSummary([id, row])];
  });
  const selectedStart = startRequests.get(selected);
  const archived = useAbandonedActivities(
    "refactoring",
    [...ownerOngoing, ...ownerHistory].map(({ id }) => id),
  );
  const ongoing = [...ownerOngoing, ...projectedStarts].filter(
    ({ id }) => !archived.isAbandoned(id),
  );
  const archivedOngoing = ownerOngoing.filter(({ id }) =>
    archived.isAbandoned(id),
  );
  const archivedSlots = archivedOngoing.filter(({ slot }) => slot);
  const active = new Set(
    [...ownerOngoing, ...projectedStarts].map(({ id }) => id),
  );
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
    ...archivedOngoing,
    ...new Map(
      ownerHistory
        .filter((r) => !active.has(r.id))
        .map((r) => [r.id, r] as const),
    ).values(),
  ];
  const suggested =
    waiting[0]?.id ??
    progressing[0]?.id ??
    history.find(({ id }) => !archived.isAbandoned(id))?.id;
  const selectedArchived = archived.isAbandoned(selected);

  useEffect(() => {
    if (initialSelection.current) return;
    if (
      selected ||
      (typeof matchMedia === "function" && matchMedia(narrowScreen).matches)
    ) {
      initialSelection.current = true;
      return;
    }
    if (suggested && !archived.pending) {
      initialSelection.current = true;
      currentSelection.current = suggested;
      setParams({ action: suggested }, { replace: true });
    }
  }, [selected, suggested, archived.pending, setParams]);

  useEffect(() => {
    if (selectedArchived && currentSelection.current === selected) {
      currentSelection.current = "";
      setParams({}, { replace: true });
    }
  }, [selected, selectedArchived, setParams]);

  useEffect(() => {
    if (!selected && list.current)
      list.current.scrollTop = listPosition.current;
  }, [selected]);

  const openAction = (id: string, followCurrent = false) => {
    listPosition.current = list.current?.scrollTop ?? listPosition.current;
    initialSelection.current = true;
    currentSelection.current = followCurrent ? id : "";
    setParams({ action: id });
    if (typeof matchMedia === "function" && matchMedia(narrowScreen).matches) {
      requestAnimationFrame(() => detailHeading.current?.focus());
    }
  };
  const closeSheet = () => {
    setOpened(false);
    requestAnimationFrame(() => newButton.current?.focus());
  };
  const start = async (
    payload: StartInput,
    directionName: string,
    environmentName: string,
  ) => {
    if (sendingStart.current || submitting) return;
    const envelope = api.envelope(
      "refactoring_start",
      identity.repository_identity,
      1,
      payload,
    );
    sendingStart.current = true;
    const id = envelope.request_id;
    const initial: StartRequest = {
      envelope,
      created_at: new Date().toISOString(),
      directionName,
      environmentName,
    };
    setStarts((items) => ({ ...items, [id]: initial }));
    setOpened(false);
    setForm(({ key }) => ({ key: key + 1 }));
    openAction(id, true);
    requestAnimationFrame(() => detailHeading.current?.focus());
    feedback.notify("重构与维护");
    let response;
    let error: string | undefined;
    try {
      response = await submit(envelope);
    } catch (err) {
      error = String(err);
    }
    if (response) {
      setStarts((items) => ({ ...items, [id]: { ...initial, response } }));
    } else {
      try {
        const saved = await requestByID(id);
        setStarts((items) => ({
          ...items,
          [id]: saved
            ? { ...initial, ...saved }
            : {
                ...initial,
                unsent: true,
                error: error || "启动请求未发送，请重新发起",
              },
        }));
      } catch {
        // Leave the original request pending if the journal cannot be read.
      }
    }
    sendingStart.current = false;
    void index.refetch();
  };
  const retryStart = (input: StartInput) => {
    setForm(({ key }) => ({ key: key + 1, input }));
    setOpened(true);
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
  const rowDebugText = (row: Summary) => {
    const query = taskDetail(row.id);
    const project = identity.repository_identity;
    if (online && known.has(row.id))
      return async () =>
        formatDebugInfo(await queryClient.fetchQuery(query), project);
    return formatDebugInfo(
      queryClient.getQueryData(query.queryKey) ?? row,
      project,
    );
  };
  const renderRows = (rows: Summary[]) =>
    rows.length ? (
      <ul className="refactoring-list">
        {rows.map((r) => (
          <li key={r.id}>
            <button
              type="button"
              data-action={r.id}
              aria-current={selected === r.id ? "true" : undefined}
              onClick={() =>
                openAction(
                  r.id,
                  active.has(r.id) && !archived.isAbandoned(r.id),
                )
              }
            >
              <strong>{r.direction_name}</strong>
              <span>
                {phaseLabel(r)}
                {r.round > 0 && ` · 重构轮次 ${r.round}`}
              </span>
              <span>
                {r.environment_name && r.environment_name + " · "}
                {r.id.slice(0, 8)}
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
            <CopyInfoButton label="复制调试信息" text={rowDebugText(r)} />
          </li>
        ))}
      </ul>
    ) : (
      <p className="muted">暂无重构任务</p>
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
          {archivedSlots.map((action) => (
            <p key={action.id}>
              {action.direction_name}仍占用推进名额 ·{" "}
              <Link
                to={{
                  search: "?" + new URLSearchParams({ action: action.id }),
                }}
              >
                核实原重构任务
              </Link>
            </p>
          ))}
        </div>
        <div className="refactoring-direction-actions">
          <Button
            type="button"
            variant="outline"
            onClick={() => setManagingDirections(true)}
          >
            管理重构方向
          </Button>
          <Button
            variant="default"
            ref={newButton}
            onClick={() => setOpened(true)}
          >
            新建重构任务
          </Button>
        </div>
      </header>
      {managingDirections && (
        <ManageDirections close={() => setManagingDirections(false)} />
      )}
      <NewMaintenance
        key={form.key}
        open={opened}
        close={closeSheet}
        onSubmit={start}
        initial={form.input}
        submissionPending={submitting}
        full={!!first && first.slots_used >= first.slot_limit}
      />
      <div className="refactoring-layout" data-selected={!!selected}>
        <section
          ref={list}
          className="settings-panel refactoring-panel refactoring-sidebar"
          aria-label="重构任务列表"
        >
          {(!online || index.isError) && (
            <p className="notice amber" role="status">
              重构任务列表当前待核实{first ? "，显示上次已知结果" : ""}
            </p>
          )}
          {online && index.isPending && (
            <p role="status">正在读取重构任务列表</p>
          )}
          {(first || projectedStarts.length > 0) && (
            <>
              <h2>进行中 {progressing.length}</h2>
              {renderRows(progressing)}
            </>
          )}
          {first && (
            <>
              <h2>待答复 {waiting.length}</h2>
              {renderRows(waiting)}
              <h2>重构任务历史</h2>
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
                currentSelection.current = "";
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
              返回重构任务列表
            </Button>
          )}
          {selected &&
            (!selectedStart || current) &&
            (!online || detail.isError || !/^[a-f0-9]{32}$/.test(selected)) && (
              <p className="notice amber" role="status">
                {current
                  ? "重构任务详情当前待核实，显示上次已知结果"
                  : "无法读取此重构任务，请返回重构任务列表"}
              </p>
            )}
          {selected &&
            online &&
            !selectedStart &&
            !current &&
            detail.isFetching && <p role="status">正在读取重构任务详情</p>}
          {!selected && <p className="muted">选择一个重构任务查看详情</p>}
          {!current && selectedStart && (
            <section
              className="settings-panel refactoring-panel"
              aria-label="重构任务详情"
            >
              <h2>
                {selectedStart.directionName ??
                  String(selectedStart.envelope.payload.direction)}
              </h2>
              {rejectedStart(selectedStart) ? (
                <>
                  <p role="alert">
                    {selectedStart.response?.diagnostic ||
                      selectedStart.error ||
                      "启动请求被拒绝"}
                  </p>
                  <Button
                    onClick={() =>
                      retryStart(selectedStart.envelope.payload as StartInput)
                    }
                  >
                    带入新建
                  </Button>
                </>
              ) : (
                <p role="status">
                  {phases[startSummary([selected, selectedStart]).phase]}
                </p>
              )}
            </section>
          )}
          {current && (
            <MaintenanceDetail
              key={current.id}
              current={current}
              disabled={actionDisabled}
              command={command}
              answer={answers[questionKey] ?? ""}
              error={actionError}
              setAnswer={(value) =>
                setAnswers((items) => ({ ...items, [questionKey]: value }))
              }
              retryStart={() => retryStart(current.input as StartInput)}
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

function formatDebugInfo(current: DebugRecord, project: string) {
  const lines = [
    formatExecutionInfo({
      project,
      title: `重构任务 · ${current.direction_name}`,
      requestID: current.id,
      record: { id: current.id, label: "重构任务" },
    }),
    `状态：${phaseLabel(current)}${current.round > 0 ? ` · 第 ${current.round} 轮` : ""}`,
  ];
  if (current.environment_name)
    lines.push(`运行环境：${current.environment_name}`);
  if (current.source_commit) lines.push(`起始提交：${current.source_commit}`);
  else if (current.input?.base_commit)
    lines.push(`申请的起始提交：${current.input.base_commit}`);
  if (current.wait_reason) lines.push(`等待原因：${current.wait_reason}`);
  if (current.diagnostic) lines.push(`诊断：${current.diagnostic}`);
  if (!current.input) lines.push("运行详情未加载");
  for (const operation of current.operations ?? []) {
    lines.push(
      "",
      `第 ${operation.round} 轮 · ${operationPurposeLabel(operation.purpose)} · ${operationStateLabel(operation.state)}`,
      `操作 ID：${operation.id}`,
      `Runner Run ID：${operation.run_id || "尚未取得"}`,
    );
  }
  return lines.join("\n");
}

function MaintenanceDetail({
  current,
  disabled,
  command,
  answer,
  error,
  setAnswer,
  retryStart,
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
  retryStart: () => void;
}) {
  const { identity } = useWorkspace();
  const [choice, setChoice] = useState({ question: "", option: "" });
  const option = choice.question === current.question?.id ? choice.option : "";
  const executionCount =
    current.operations?.filter(
      (operation) => operation.workflow === "execute" && operation.run_id,
    ).length ?? 0;
  const strategy =
    strategyNames[current.input.strategy] ?? current.input.strategy;
  const finish = current.finish_action
    ? finishActions[current.finish_action]
    : undefined;
  return (
    <section
      className="settings-panel refactoring-panel"
      aria-label="重构任务详情"
    >
      <h2>{current.direction_name}</h2>
      <p>
        <span>{phaseLabel(current)}</span>
        {" · "}
        <span>重构轮次 {current.round}</span>
        {" · "}
        <span>Runner 执行 {executionCount} 次</span>
      </p>
      <DeliveryResult record={current} />
      {current.wait_reason && <p role="status">{current.wait_reason}</p>}
      {error && (
        <p className="notice amber" role="alert">
          {error}
        </p>
      )}
      {current.diagnostic && (
        <p className="notice amber" role="alert">
          {current.diagnostic}
        </p>
      )}
      {current.startup_failed && (
        <div className="refactoring-actions">
          <Link
            to={{
              pathname: `/projects/${identity.project_id}/dag`,
              search: "?maintenance=preparation",
            }}
          >
            打开项目维护
          </Link>
          <Link to={`/projects/${identity.project_id}/environment`}>
            打开运行环境
          </Link>
          <Button onClick={retryStart}>带入新建</Button>
        </div>
      )}
      <div className="refactoring-actions">
        <CopyInfoButton
          label="复制调试信息"
          text={formatDebugInfo(current, identity.repository_identity)}
        />
        {current.can_finish && finish && (
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
            {finish.label}
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
      </div>
      {finish && <p className="muted">{finish.explanation}</p>}
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
      <ExecutionConversation key={current.id} record={current} />
      <details>
        <summary>运行信息</summary>
        {current.summary && <p>{current.summary}</p>}
        <p>
          {current.environment_name && <>{current.environment_name} · </>}
          执行配置：{current.profile_name || current.input.profile} · 策略：
          {strategy}
        </p>
        {(current.source_commit || current.input.base_commit) && (
          <p>
            {current.source_commit ? "起始提交" : "申请的起始提交"}{" "}
            {current.source_branch}{" "}
            <code>{current.source_commit || current.input.base_commit}</code>
          </p>
        )}
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
            : "轮次未设置"}{" "}
          · 已用 {duration(current.elapsed_seconds)}
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
      </details>
    </section>
  );
}
