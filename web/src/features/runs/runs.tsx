import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { ArrowUpRight, Layers } from "lucide-react";
import { useWorkspace } from "../../app/context";
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
import { Answer, draftKey } from "./answer";
import {
  candidateStatuses,
  participating,
  candidateFacets,
  candidateFilters,
  matchesCandidate,
  optionalAnswer,
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
import { RunTask, taskDependencyReason } from "./run-task";
import { CopyExecutionInfo } from "./copy-execution-info";
import { CopyExecutionInfo as CopyInfo } from "../activities/execution-info";
import { StopCandidate } from "./stop-candidate";
import { ResolvePRConflict } from "./resolve-pr-conflict";
import { RunUsage } from "./run-usage";
import { RejectedStart } from "./rejected-start";
import { ContinuationSuggestion } from "./continuation-suggestion";

function CandidateStatuses({ run, task }: { run: Candidate; task?: Task }) {
  const states = candidateStatuses(run, task);
  return (
    <span className="candidate-statuses">
      {states.map((state) => (
        <Status key={state} value={state} />
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
  const caps = snapshot?.connection.snapshot.page?.service.capabilities ?? [];
  const nativeUnavailable = nativeSessionReason(run, online, caps);
  const answerVisible = !!run.decision && (!stopApplied || !participating(run));
  return (
    <>
      <section className="run-section" aria-label="当前进度">
        <div className="candidate-heading">
          <div>
            <div className="eyebrow">
              候选 C{run.candidate_number || "—"} ·{" "}
              {run.execution_profile?.name || run.agent}
            </div>
            <h2>{run.title}</h2>
            <div className="actions">
              <CandidateStatuses run={run} task={task} />
              <span className="muted">{currentStage(run)}</span>
            </div>
          </div>
          <div className="actions candidate-actions">
            <CopyExecutionInfo run={run} />
            <ResolvePRConflict run={run} />
            {stopControl}
            <Button
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
          </div>
        </div>
        {nativeUnavailable && <p className="muted">{nativeUnavailable}</p>}
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
            stale={
              !online ||
              !!snapshot?.connection.snapshot.error ||
              !!snapshot?.connection.snapshot.page?.service.error
            }
            sampledAt={snapshot?.connection.snapshot.sampled_at}
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
        <DeliverySummary
          run={run}
          task={task}
          stale={
            !online ||
            !!snapshot?.connection.snapshot.error ||
            !!snapshot?.connection.snapshot.page?.service.error
          }
        />
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
  const runID = params.get("run") ?? "";
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
  const visibleRuns = runs.filter(
    (run) => showStopped || run.participation !== "abandoned",
  );
  const tasks = useQuery({
    ...projectQueries(api).tasks(),
    enabled: online,
    refetchInterval: 2500,
  });
  const selectedTask =
    taskID ||
    runs[0]?.domain_task_id ||
    runs[0]?.task_id ||
    tasks.data?.tasks[0]?.task_id ||
    "";
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
  const selectedDependencyReason = taskDependencyReason(selectedTaskRecord);
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
  const groups = (tasks.data?.tasks ?? []).map((task) => {
    const candidates = visibleRuns.filter(
      (run) => (run.domain_task_id || run.task_id) === task.task_id,
    );
    const pending = starts.filter((entry) => entry.taskID === task.task_id);
    const matching = (value: CandidateFilter) =>
      candidates
        .filter((run) => matchesCandidate(run, value, task))
        .sort(
          (a, b) =>
            a.candidate_number - b.candidate_number ||
            a.run_id.localeCompare(b.run_id),
        );
    const counts = new Map<string, number>();
    for (const run of candidates) {
      const facets = candidateFacets(run, task);
      const states = facets.length
        ? facets.map(label)
        : candidateStatuses(run, task).map(label);
      for (const state of states)
        counts.set(state, (counts.get(state) ?? 0) + 1);
    }
    for (const entry of pending)
      counts.set(entry.status, (counts.get(entry.status) ?? 0) + 1);
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
    setParams(next ? { task: id, run: next.run_id } : { task: id });
  };
  const applyFilter = (value: CandidateFilter) => {
    setFilter(value);
    if (value === "all") return;
    const matching = groups.filter((group) => group.matching(value).length > 0);
    const next =
      matching.find((group) => group.task.task_id === selectedTask) ??
      matching[0];
    if (
      next &&
      (!selected ||
        !matchesCandidate(selected, value, next.task) ||
        next.task.task_id !== selectedTask)
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
    <>
      <div className="page-heading">
        <div>
          <h1>执行与待处理</h1>
        </div>
        <Layers className="heading-icon" size={30} />
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
        <aside className="run-task-list">
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
            (!selected ||
              !matchesCandidate(selected, filter, selectedTaskRecord)) && (
              <p className="notice amber" role="status">
                当前候选已不符合筛选，保留当前查看内容。
              </p>
            )}
          <div className="candidate-strip">
            {candidates.map((r) => (
              <button
                key={r.run_id}
                onClick={() => chooseRun(r.run_id)}
                className={
                  "candidate-card " +
                  (selected?.run_id === r.run_id ? "active " : "") +
                  (filter !== "all" &&
                  matchesCandidate(r, filter, selectedTaskRecord)
                    ? "filter-match"
                    : "")
                }
                aria-pressed={selected?.run_id === r.run_id}
              >
                <span>候选 C{r.candidate_number || "—"}</span>
                <strong>{r.execution_profile?.name || r.agent}</strong>
                <CandidateStatuses run={r} task={selectedTaskRecord} />
                {filter !== "all" &&
                  matchesCandidate(r, filter, selectedTaskRecord) && (
                    <span className="candidate-match">符合筛选</span>
                  )}
                {preparationWait(r) && (
                  <span className="muted">
                    {executionWaitTitle(preparationWait(r)!)}
                  </span>
                )}
              </button>
            ))}
            {pendingCandidates.map((entry) => (
              <button
                key={entry.id}
                onClick={() => chooseRun(entry.id)}
                className={
                  "candidate-card " + (runID === entry.id ? "active" : "")
                }
              >
                <span>新候选</span>
                <strong>{entry.profile}</strong>
                <span className="muted">
                  {entry.status}
                  {entry.rejected ? "（历史）" : ""}
                </span>
              </button>
            ))}
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
                : selectedDependencyReason
                  ? `${selectedDependencyReason}，解除依赖阻塞后可启动。`
                  : "此任务尚无执行候选，请从任务详情手动启动。"}
            </div>
          )}
        </section>
        <aside className="detail-panel">
          {selectedTask && (
            <TaskDetail key={selectedTask} id={selectedTask} allowCandidate />
          )}
        </aside>
      </div>
    </>
  );
}
