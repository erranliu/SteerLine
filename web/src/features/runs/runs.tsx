import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { ArrowUpRight } from "lucide-react";
import { useWorkspace } from "../../app/context";
import type { CandidateStart } from "../../app/use-candidate-starts";
import type {
  Candidate,
  Envelope,
  Receipt,
  Task,
} from "../../shared/api/types";
import { projectQueries } from "../../shared/api/queries";
import { Button } from "../../shared/ui/button";
import { Status, label } from "../../shared/ui/status";
import { TaskDetail } from "../tasks/task-detail";
import {
  candidateStartStatus,
  latestTaskStarts,
  TaskStatus,
} from "../tasks/task-status";
import { Answer, draftKey } from "./answer";
import {
  candidateStatuses,
  participating,
  candidateFacets,
  candidateFilters,
  matchesCandidate,
  optionalAnswer,
  startingExecution,
  type CandidateFilter,
} from "./state";
import { LastMessage } from "./last-message";
import { RunnerWorkflow } from "./runner-workflow";
import { DeliverySummary, deliveryLabel } from "./delivery-summary";
import { RecentActivity } from "./recent-activity";
import { FinalOutput } from "./final-output";
import { candidateNotices, commandIssues, currentStage } from "./presentation";
import {
  ExecutionWaitSummary,
  executionWaitTitle,
  preparationWait,
} from "./execution-wait";
import { PlanTests } from "./plan-tests";
import { CandidateDocuments } from "./candidate-documents";
import { nativeSessionReason } from "./native-session";
import { RunTask } from "./run-task";
import { CopyExecutionInfo } from "./copy-execution-info";
import { CopyExecutionInfo as CopyInfo } from "../activities/execution-info";
import { StopCandidate } from "./stop-candidate";
import { ResolvePRConflict } from "./resolve-pr-conflict";
import { RunUsage } from "./run-usage";
import { RejectedStart } from "./rejected-start";
import { ContinuationSuggestion } from "./continuation-suggestion";
import "./runs.css";

function candidateExecutionState(state: string) {
  if (state === "running") return state;
  if (startingExecution(state)) return "task_starting";
}

function matchesStart(start: CandidateStart, filter: CandidateFilter) {
  return (
    filter === "running" &&
    start.participating &&
    candidateStartStatus(start) === "task_starting"
  );
}

function CandidateStatuses({
  run,
  states,
}: {
  run: Candidate;
  states: string[];
}) {
  return (
    <span className="execution-statuses">
      {states.map((state) => (
        <Status
          key={state}
          value={
            state === "pending" || state === "preparing"
              ? "task_starting"
              : state
          }
          appearance={candidateExecutionState(state)}
        />
      ))}
      {states.includes("waiting_for_input") && optionalAnswer(run) && (
        <span className="muted">可稍后回答</span>
      )}
    </span>
  );
}

function CandidateDetail({
  run,
  task,
  stopControl,
  stopApplied = false,
}: {
  run: Candidate;
  task?: Task;
  stopControl: React.ReactNode;
  stopApplied?: boolean;
}) {
  const { api, online, send, busy, snapshot } = useWorkspace();
  const observation = snapshot?.connection.snapshot;
  const caps = observation?.page?.service.capabilities ?? [];
  const nativeUnavailable = nativeSessionReason(run, online, caps);
  const answerVisible = !!run.decision && (!stopApplied || !participating(run));
  const stale =
    !online || !!observation?.error || !!observation?.page?.service.error;
  const sampledAt = observation?.sampled_at;
  const observedAt = sampledAt ? new Date(sampledAt) : undefined;
  return (
    <>
      <section className="run-section" aria-label="当前进度">
        <div className="candidate-heading run-overview">
          <div className="run-title">
            <p className="run-candidate-meta">
              候选 C{run.candidate_number || "—"} ·{" "}
              {run.execution_profile?.name || run.agent}
            </p>
            <h2>{run.title}</h2>
          </div>
          <div className="run-stage">
            <div>
              <p className="run-stage-label">当前阶段</p>
              <p className="run-stage-name">{currentStage(run)}</p>
            </div>
            <div className="run-stage-status">
              <CandidateStatuses
                run={run}
                states={candidateStatuses(run, task)}
              />
              {observedAt && Number.isFinite(observedAt.getTime()) && (
                <p className="run-sampled-at">
                  {stale ? "上次已知" : "状态观察于"}{" "}
                  <time
                    dateTime={sampledAt}
                    title={observedAt.toLocaleString("zh-CN", {
                      hour12: false,
                    })}
                  >
                    {stale
                      ? observedAt.toLocaleString("zh-CN", { hour12: false })
                      : observedAt.toLocaleTimeString("zh-CN", { hour12: false })}
                  </time>
                </p>
              )}
            </div>
          </div>
          <div className="actions candidate-actions">
            <Button
              variant={
                !answerVisible && !nativeUnavailable ? "default" : "outline"
              }
              disabled={!!nativeUnavailable || busy}
              title={nativeUnavailable || (busy ? "正在处理上一项操作" : "")}
              onClick={() =>
                void send("open_native_session", run.run_id, 0, {
                  run_id: run.run_id,
                  execution_id: run.execution_id,
                  attempt_id: run.attempt_id,
                  session_id: run.session_id,
                  session_generation: run.session_generation,
                })
              }
            >
              在 {run.host === "orca" ? "Orca" : "Codex"} 中查看{" "}
              <ArrowUpRight size={14} />
            </Button>
            <CopyExecutionInfo run={run} />
            <ResolvePRConflict run={run} />
            {stopControl}
          </div>
          {nativeUnavailable && <p className="muted">{nativeUnavailable}</p>}
        </div>
        {answerVisible && (
          <Answer
            key={draftKey(
              api.identity.project_id,
              api.identity.instance_id,
              run,
            )}
            run={run}
          />
        )}
        {run.participation === "abandoned" && (
          <p className="notice amber">
            已停止参与 · 历史只读；底层执行可能仍在运行。
          </p>
        )}
        {preparationWait(run) && (
          <ExecutionWaitSummary
            wait={preparationWait(run)!}
            stale={stale}
            sampledAt={sampledAt}
          />
        )}
        {run.runner_delivery &&
          (!run.runner_delivery.available ||
            !["accepted", "pending"].includes(
              run.runner_delivery.submission,
            )) && (
            <p className="notice amber">
              请求受理 · {deliveryLabel(run.runner_delivery.submission)}
            </p>
          )}
        {run.runner_delivery?.available &&
          ["waiting_input", "blocked", "failed", "reconciling"].includes(
            run.runner_delivery.status === "waiting_input" && run.decision
              ? ""
              : (run.runner_delivery.status ?? ""),
          ) && (
            <p className="notice amber">
              执行观察 · {deliveryLabel(run.runner_delivery.status)}
            </p>
          )}
        {candidateNotices(run, answerVisible).map((text) => (
          <p key={text} className="notice amber">
            {text}
          </p>
        ))}
        {run.runner_delivery &&
          !run.runner_delivery.available &&
          !run.runner_delivery.diagnostic && (
            <p className="notice amber">
              执行观察暂不可核实，保留上次已知状态。
            </p>
          )}
        {commandIssues(run).map((issue) => (
          <p key={issue} className="notice amber">
            {issue}
          </p>
        ))}
        {run.runner_delivery?.resume_required &&
          !(answerVisible &&
            run.decision?.request.interaction_kind === "runner_resume") && (
            <p className="notice amber">需处理 · 核验原执行后显式恢复。</p>
          )}
        {stopApplied && run.participation === "active" && (
          <p className="notice amber">
            停止已生效，列表刷新中；可使用页面刷新重取。
          </p>
        )}
        {!run.decision && <ContinuationSuggestion key={run.run_id} run={run} />}
      </section>
      <section className="run-section" aria-label="最近动态">
        <h3>最近动态</h3>
        <LastMessage
          key={"message/" + run.execution_id + "/" + run.attempt_id}
          run={run}
          answerVisible={answerVisible}
        />
        <RecentActivity
          key={"activity/" + run.execution_id + "/" + run.attempt_id}
          run={run}
        />
      </section>
      <section className="run-section" aria-label="测试与审查">
        <h3>测试与审查</h3>
        <PlanTests key={run.execution_id + "/" + run.attempt_id} run={run} />
        <RunnerWorkflow run={run} />
        <CandidateDocuments
          key={run.execution_id + "/" + run.attempt_id}
          run={run}
          showNotice={false}
        />
      </section>
      <section className="run-section" aria-label="交付结果">
        <h3>交付结果</h3>
        <DeliverySummary run={run} task={task} stale={stale} />
        <FinalOutput key={run.execution_id + "/" + run.attempt_id} run={run} />
        <RunUsage key={run.run_id + "/" + run.execution_id} run={run} />
      </section>
    </>
  );
}
export function Runs() {
  const { api, online, snapshot, requests, candidateStarts } = useWorkspace();
  const [pending, setPending] = useState<{
    envelope: Envelope;
    epoch: number;
    receipt?: Receipt;
  }>();
  const selectionEpoch = useRef(0);
  const [params, setParams] = useSearchParams();
  const taskID = params.get("task") ?? "";
  const [filter, setFilter] = useState<CandidateFilter>("all");
  const [showStopped, setShowStopped] = useState(false);
  const page = snapshot?.connection.snapshot.page;
  const runs = page?.runs ?? [];
  const stopApplied = (id: string) =>
    (pending?.envelope.target_id === id &&
      pending.receipt?.status === "applied") ||
    !!requests?.some(
      (row) =>
        row.envelope.project_id === api.identity.project_id &&
        row.envelope.action === "stop_candidate" &&
        row.envelope.target_id === id &&
        row.response?.status === "applied",
    );
  const starts =
    candidateStarts?.items.filter(
      (entry) => !runs.some((run) => run.run_id === entry.id),
    ) ?? [];
  const initialStarts = latestTaskStarts(candidateStarts?.items);
  const visibleRuns = runs.filter(
    (run) => showStopped || run.participation !== "abandoned",
  );
  const tasks = useQuery({
    ...projectQueries(api).tasks(),
    enabled: online,
    refetchInterval: 2500,
  });
  const blockedTasks = new Set(
    tasks.data?.tasks
      .filter((task) => task.status === "open" && !task.dependency_ready)
      .map((task) => task.task_id),
  );
  const selectableTasks = (tasks.data?.tasks ?? []).filter(
    (task) => !blockedTasks.has(task.task_id),
  );
  const firstRun = runs.find(
    (run) => !blockedTasks.has(run.domain_task_id || run.task_id),
  );
  const selectedTask =
    (blockedTasks.has(taskID) ? "" : taskID) ||
    firstRun?.domain_task_id ||
    firstRun?.task_id ||
    selectableTasks[0]?.task_id ||
    "";
  const runID = blockedTasks.has(taskID) ? "" : params.get("run") ?? "";
  const taskCandidates = runs.filter(
    (r) => (r.domain_task_id || r.task_id) === selectedTask,
  );
  const candidates = visibleRuns.filter(
    (r) => (r.domain_task_id || r.task_id) === selectedTask,
  );
  const pendingCandidates = starts.filter(
    (entry) => entry.taskID === selectedTask,
  );
  const selectedStart = pendingCandidates.find((entry) => entry.id === runID);
  const selectedTaskRecord = tasks.data?.tasks.find(
    (task) => task.task_id === selectedTask,
  );
  const requested = runID
    ? taskCandidates.find((r) => r.run_id === runID)
    : taskCandidates[0];
  // Keep the current stop target until its original receipt is reconciled.
  const awaitingStop =
    pending?.envelope.target_id === requested?.run_id &&
    pending?.epoch === selectionEpoch.current;
  const selected =
    !showStopped && requested?.participation === "abandoned" && !awaitingStop
      ? candidates[0]
      : requested;
  const groups = selectableTasks.map((task) => {
    const candidates = visibleRuns.filter(
      (run) => (run.domain_task_id || run.task_id) === task.task_id,
    );
    const pending = starts.filter((entry) => entry.taskID === task.task_id);
    const matching = (value: CandidateFilter) => [
      ...candidates
        .filter((run) => matchesCandidate(run, value, task))
        .sort(
          (a, b) =>
            a.candidate_number - b.candidate_number ||
            a.run_id.localeCompare(b.run_id),
        )
        .map((run) => run.run_id),
      ...pending
        .filter((entry) => matchesStart(entry, value))
        .map((entry) => entry.id),
    ];
    const counts = new Map<string, number>();
    for (const run of candidates) {
      const facets = candidateFacets(run, task);
      const states = facets.length
        ? facets.map(label)
        : candidateStatuses(run, task).map(label);
      for (const state of states)
        counts.set(state, (counts.get(state) ?? 0) + 1);
    }
    for (const entry of pending) {
      const state = matchesStart(entry, "running")
        ? label("running")
        : entry.status;
      counts.set(state, (counts.get(state) ?? 0) + 1);
    }
    return {
      task,
      matching,
      count: candidates.length + pending.length,
      summary: [...counts]
        .sort(([a], [b]) => {
          const order: string[] = candidateFilters.map(([, title]) => title);
          const rank = (state: string) =>
            order.includes(state) ? order.indexOf(state) : order.length;
          return rank(a) - rank(b) || a.localeCompare(b);
        })
        .map(([state, count]) => `${state} ${count}`)
        .join(" · "),
    };
  });
  const visible = groups.filter(
    (group) => filter === "all" || group.matching(filter).length > 0,
  );
  const filterCounts = (value: CandidateFilter) => {
    const matching = groups.filter(
      (group) => value === "all" || group.matching(value).length > 0,
    );
    return `${matching.length} 个任务 / ${matching.reduce((count, group) => count + (value === "all" ? group.count : group.matching(value).length), 0)} 个候选`;
  };
  const chooseTask = (id: string, value = filter) => {
    selectionEpoch.current++;
    const next = groups
      .find((group) => group.task.task_id === id)
      ?.matching(value)[0];
    setParams(next ? { task: id, run: next } : { task: id });
  };
  const selectedMatches = (value: CandidateFilter) =>
    selectedStart
      ? matchesStart(selectedStart, value)
      : !!selected && matchesCandidate(selected, value, selectedTaskRecord);
  const applyFilter = (value: CandidateFilter) => {
    setFilter(value);
    if (value === "all") return;
    const matching = groups.filter((group) => group.matching(value).length > 0);
    const next =
      matching.find((group) => group.task.task_id === selectedTask) ??
      matching[0];
    if (
      next &&
      (next.task.task_id !== selectedTask || !selectedMatches(value))
    ) {
      chooseTask(next.task.task_id, value);
    }
  };
  const chooseRun = (id: string) => {
    selectionEpoch.current++;
    setParams({ task: selectedTask, run: id });
  };
  // Fix the initial candidate identity so snapshot reordering cannot switch the
  // output or answer editor. Later filter changes navigate only on user action.
  useEffect(() => {
    if (!runID && selected) {
      setParams(
        (current) => {
          const next = new URLSearchParams(current);
          next.set("task", selectedTask);
          next.set("run", selected.run_id);
          return next;
        },
        { replace: true },
      );
    }
  }, [runID, selected?.run_id, selectedTask, setParams]);
  useEffect(() => {
    if (!pending) return;
    const receipt =
      requests?.find(
        (row) => row.envelope.request_id === pending.envelope.request_id,
      )?.response || pending.receipt;
    if (receipt?.status === "rejected") {
      setPending(undefined);
      return;
    }
    const stopped = runs.find(
      (candidate) => candidate.run_id === pending.envelope.target_id,
    );
    if (receipt?.status !== "applied" || stopped?.participation !== "abandoned")
      return;
    if (
      selectionEpoch.current !== pending.epoch ||
      requested?.run_id !== stopped.run_id
    ) {
      setPending(undefined);
      return;
    }
    const next = runs
      .filter(
        (candidate) =>
          (candidate.domain_task_id || candidate.task_id) ===
            (stopped.domain_task_id || stopped.task_id) &&
          participating(candidate),
      )
      .sort((a, b) => b.candidate_number - a.candidate_number)[0];
    if (next) {
      setParams({
        task: stopped.domain_task_id || stopped.task_id,
        run: next.run_id,
      });
      setPending(undefined);
    }
  }, [pending, requests, runs, requested?.run_id, setParams]);
  const stopPending = (candidate: Candidate) =>
    !!(
      pending?.envelope.target_id === candidate.run_id ||
      requests?.some(
        (row) =>
          row.envelope.project_id === api.identity.project_id &&
          row.envelope.instance_id === api.identity.instance_id &&
          row.envelope.action === "stop_candidate" &&
          row.envelope.target_id === candidate.run_id &&
          row.response?.status !== "rejected" &&
          (row.response?.status !== "applied" || participating(candidate)),
      )
    );
  return (
    <div className="runs-page">
      <div className="page-heading">
        <h1>任务执行</h1>
      </div>
      <div className="toolbar">
        <div className="segmented execution-filters" aria-label="候选状态筛选">
          {candidateFilters.map(([value, title]) => (
            <Button
              key={value}
              size="sm"
              aria-label={title}
              aria-pressed={filter === value}
              title={`${title}：${filterCounts(value)}`}
              variant={filter === value ? "default" : "ghost"}
              onClick={() => applyFilter(value)}
            >
              {title}
            </Button>
          ))}
        </div>
        <div className="actions">
          <label className="candidate-visibility">
            <input
              type="checkbox"
              checked={showStopped}
              onChange={(event) => {
                setShowStopped(event.target.checked);
                if (
                  !event.target.checked &&
                  selected?.participation === "abandoned"
                ) {
                  const next = taskCandidates.find(
                    (run) => run.participation !== "abandoned",
                  );
                  if (next) chooseRun(next.run_id);
                  else chooseTask(selectedTask);
                }
              }}
            />
            显示已停止参与候选
          </label>
          <span className="muted" aria-live="polite">
            {candidateFilters.find(([value]) => value === filter)?.[1]}：
            {filterCounts(filter)}
          </span>
        </div>
      </div>
      <div className="runs-layout">
        <aside className="run-task-list" aria-label="任务列表">
          {tasks.isPending && <div className="empty">正在读取正式任务…</div>}
          {tasks.data?.tasks.length === 0 && (
            <div className="empty">还没有正式任务</div>
          )}
          {visible.map((group) => (
            <RunTask
              key={group.task.task_id}
              task={group.task}
              selected={group.task.task_id === selectedTask}
              onSelect={() => chooseTask(group.task.task_id)}
              candidateCount={group.count}
              summary={group.summary}
              initialStart={initialStarts.get(group.task.task_id)}
              matchedCount={
                filter === "all" ? undefined : group.matching(filter).length
              }
            />
          ))}
          {!tasks.isPending &&
            !tasks.error &&
            tasks.data?.tasks.length !== 0 &&
            visible.length === 0 && (
              <p className="empty">没有符合筛选条件的任务</p>
            )}
          {tasks.data?.has_more && (
            <p className="muted">
              仅筛选并统计前 100 个任务，可从 DAG 详情定位后续任务。
            </p>
          )}
          {tasks.error && <p className="notice amber">{String(tasks.error)}</p>}
        </aside>
        <section className="execution-panel">
          {filter !== "all" &&
            (selected || selectedStart) &&
            !selectedMatches(filter) && (
              <p className="notice amber" role="status">
                当前候选已不符合筛选，保留当前查看内容。
              </p>
            )}
          <div className="candidate-strip">
            {candidates.map((r) => {
              const states = candidateStatuses(r, selectedTaskRecord);
              const matches =
                filter !== "all" &&
                matchesCandidate(r, filter, selectedTaskRecord);
              const wait = preparationWait(r);
              return (
                <button
                  key={r.run_id}
                  onClick={() => chooseRun(r.run_id)}
                  className={
                    "candidate-card " +
                    (selected?.run_id === r.run_id ? "active " : "") +
                    (matches ? "filter-match" : "")
                  }
                  aria-pressed={selected?.run_id === r.run_id}
                  data-execution-state={candidateExecutionState(states[0])}
                >
                  <span>候选 C{r.candidate_number || "—"}</span>
                  <strong>{r.execution_profile?.name || r.agent}</strong>
                  <CandidateStatuses run={r} states={states} />
                  {matches && (
                    <span className="candidate-match">符合筛选</span>
                  )}
                  {wait && (
                    <span className="muted">{executionWaitTitle(wait)}</span>
                  )}
                </button>
              );
            })}
            {pendingCandidates.map((entry) => {
              const matches = filter !== "all" && matchesStart(entry, filter);
              return (
                <button
                  key={entry.id}
                  onClick={() => chooseRun(entry.id)}
                  className={
                    "candidate-card " +
                    (runID === entry.id ? "active " : "") +
                    (matches ? "filter-match" : "")
                  }
                  aria-pressed={runID === entry.id}
                  data-execution-state={candidateStartStatus(entry)}
                >
                  <span>新候选</span>
                  <strong>{entry.profile}</strong>
                  <TaskStatus status={candidateStartStatus(entry)} />
                  {matches && <span className="candidate-match">符合筛选</span>}
                  <span className="muted candidate-start-detail">
                    {entry.status}
                    {entry.rejected ? "（历史）" : ""}
                  </span>
                </button>
              );
            })}
          </div>
          {selectedStart ? (
            <div
              key={selectedStart.id}
              className="detail-section"
              role="status"
            >
              <div className="eyebrow">新候选 · {selectedStart.profile}</div>
              <h2>
                {selectedStart.title ||
                  tasks.data?.tasks.find(
                    (task) => task.task_id === selectedTask,
                  )?.title ||
                  selectedTask}
              </h2>
              <TaskStatus status={candidateStartStatus(selectedStart)} />
              <CopyInfo
                info={{
                  title: "任务候选启动",
                  project: api.identity.repository_identity,
                  requestID: selectedStart.id,
                  taskID: selectedStart.taskID,
                  runner: {},
                  runnerUnverified: !selectedStart.rejected,
                }}
              />
              {selectedStart.rejected ? (
                <RejectedStart start={selectedStart} />
              ) : (
                <>
                  <p>{selectedStart.status}</p>
                  {selectedStart.diagnostic && (
                    <p className="notice amber">{selectedStart.diagnostic}</p>
                  )}
                  <p className="muted">
                    {selectedStart.participating
                      ? "执行信息将在服务器确认后显示。"
                      : "请核对原因后重新选择方案。"}
                  </p>
                </>
              )}
            </div>
          ) : selected ? (
            <CandidateDetail
              key={[
                api.identity.project_id,
                api.identity.instance_id,
                selected.run_id,
                selected.execution_id,
                selected.attempt_id,
              ].join("/")}
              run={selected}
              task={selectedTaskRecord}
              stopApplied={stopApplied(selected.run_id)}
              stopControl={
                <StopCandidate
                  run={selected}
                  pending={stopPending(selected)}
                  applied={
                    stopApplied(selected.run_id) && participating(selected)
                  }
                  onRequest={(envelope) => {
                    setParams({ task: selectedTask, run: envelope.target_id });
                    setPending({ envelope, epoch: selectionEpoch.current });
                  }}
                  onResult={(envelope, receipt, unsent) =>
                    setPending((current) =>
                      current?.envelope.request_id !== envelope.request_id
                        ? current
                        : unsent || receipt?.status === "rejected"
                          ? undefined
                          : { ...current, receipt },
                    )
                  }
                />
              }
            />
          ) : (
            <div className="empty">
              {!showStopped &&
              taskCandidates.length > 0 &&
              candidates.length === 0
                ? "此任务的候选均已停止参与，勾选“显示已停止参与候选”可查看历史。"
                : selectedTask
                  ? "此任务尚无执行候选，请从任务详情手动启动。"
                  : "暂无可显示的任务。"}
            </div>
          )}
        </section>
        <aside className="detail-panel">
          {selectedTask && (
            <TaskDetail
              key={selectedTask}
              id={selectedTask}
              allowCandidate
              initialStart={initialStarts.get(selectedTask)}
            />
          )}
        </aside>
      </div>
    </div>
  );
}
