import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { ArrowUpRight, Activity as ActivityIcon, Layers } from "lucide-react";
import { useWorkspace } from "../../app/context";
import type { Candidate, Envelope, Receipt } from "../../shared/api/types";
import { projectQueries } from "../../shared/api/queries";
import { Button } from "../../shared/ui/button";
import { SafeLink, Status, short } from "../../shared/ui/status";
import { TaskDetail } from "../tasks/task-detail";
import { Answer, draftKey } from "./answer";
import { candidateState, participating } from "./state";
import { LastMessage } from "./last-message";
import { RunnerWorkflow } from "./runner-workflow";
import { nativeSessionReason } from "./native-session";
import { RunTask, taskDependencyReason } from "./run-task";
import { CopyExecutionInfo } from "./copy-execution-info";
import { StopCandidate } from "./stop-candidate";

function Output({ run }: { run: Candidate }) {
  const { api, online } = useWorkspace();
  const [tab, setTab] = useState<"activity" | "final">("activity");
  const activity = useQuery({
    ...projectQueries(api).activity(run),
    enabled:
      online && tab === "activity" && !!run.execution_id && !!run.attempt_id,
    refetchInterval: 2500,
  });
  const final = useQuery({
    ...projectQueries(api).final(run),
    enabled: online && tab === "final" && !!run.attempt_id,
    refetchInterval: 5000,
  });
  const updatedAt =
    tab === "activity" ? activity.dataUpdatedAt : final.dataUpdatedAt;
  return (
    <section className="output-panel">
      <div className="toolbar">
        <div className="segmented">
          <Button
            variant={tab === "activity" ? "default" : "ghost"}
            onClick={() => setTab("activity")}
          >
            <ActivityIcon size={14} />
            近期活动
          </Button>
          <Button
            variant={tab === "final" ? "default" : "ghost"}
            onClick={() => setTab("final")}
          >
            最终回复
          </Button>
        </div>
        <span className="muted">
          {online ? "查询刷新于" : "最后查询于"} ·{" "}
          {updatedAt ? new Date(updatedAt).toLocaleTimeString("zh-CN") : "—"}
        </span>
      </div>
      {tab === "activity" ? (
        <div className="activity-list">
          {activity.error && (
            <p className="notice amber">{String(activity.error)}</p>
          )}
          {activity.data?.page.diagnostics?.map((d, i) => (
            <p className="notice amber" key={i}>
              {d}
            </p>
          ))}
          {activity.data?.page.activity
            ?.filter((event) => event.kind !== "runner_event")
            .map((event) => (
              <div className="activity-item" key={event.seq}>
                <span className="activity-dot" />
                <time>{new Date(event.ts).toLocaleTimeString("zh-CN")}</time>
                <div>
                  <strong>{event.title || event.kind}</strong>
                  {event.detail && <pre>{event.detail}</pre>}
                </div>
              </div>
            ))}
          {!activity.data?.page.activity?.length && (
            <div className="empty">
              {activity.isPending
                ? "正在读取近期活动…"
                : "当前候选没有可用活动"}
            </div>
          )}
          {!!activity.data?.page.activity?.some(
            (event) => event.kind === "runner_event",
          ) && (
            <details>
              <summary>内部执行事件</summary>
              {activity.data.page.activity
                .filter((event) => event.kind === "runner_event")
                .map((event) => (
                  <p key={event.seq} className="muted">
                    {new Date(event.ts).toLocaleTimeString("zh-CN")} ·{" "}
                    {event.title}
                  </p>
                ))}
            </details>
          )}
          {!!activity.data?.page.activity?.length &&
            !activity.data.page.activity.some(
              (event) => event.kind !== "runner_event",
            ) && (
              <p className="muted">
                暂无已记录的流程或状态变化；已有内部事件可在详情查看。
              </p>
            )}
          <p className="output-hint">
            近期有界活动 · 不代表完整工具过程或最终回复
          </p>
        </div>
      ) : (
        <div className="final-body">
          {final.data?.page.events
            .filter((event) => event.type === "gap" || event.diagnostic)
            .map((event, index) => (
              <p key={index} className="notice amber">
                会话采集缺口：{event.diagnostic || "来源未完整确认"}
              </p>
            ))}
          {final.error && <p className="notice amber">{String(final.error)}</p>}
          {final.data?.bodies.map((body) => (
            <article key={body.item_key + "-" + body.revision}>
              <p className="eyebrow">
                最终回复 ·{" "}
                {body.complete
                  ? "正文完整"
                  : "正文不完整：" + body.completeness}
              </p>
              <pre>{body.text}</pre>
            </article>
          ))}
          {final.data?.page.has_more && (
            <p className="notice amber">最终事件页未完整，请继续读取原会话。</p>
          )}
          {!final.data?.bodies.length && (
            <div className="empty">
              {final.isPending
                ? "正在读取最终回复…"
                : "当前轮次尚无完整最终回复"}
            </div>
          )}
        </div>
      )}
    </section>
  );
}
function CandidateDetail({
  run,
  stopControl,
}: {
  run: Candidate;
  stopControl: React.ReactNode;
}) {
  const { api, online, send, busy, snapshot } = useWorkspace();
  const caps = snapshot?.connection.snapshot.page?.service.capabilities ?? [];
  const nativeUnavailable = nativeSessionReason(run, online, caps);
  return (
    <>
      <div className="candidate-heading">
        <div>
          <div className="eyebrow">
            候选 C{run.candidate_number || "—"} ·{" "}
            {run.execution_profile?.name || run.agent}
          </div>
          <h2>{run.title}</h2>
          <div className="actions">
            <Status value={candidateState(run)} />
            <span className="muted">
              {run.workflow_stage} · {run.workflow_status}
            </span>
          </div>
        </div>
        <div className="actions candidate-actions">
          <CopyExecutionInfo run={run} />
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
      <div className="run-meta">
        <span>Run {short(run.run_id)}</span>
        <span>Execution {short(run.execution_id)}</span>
        <span>Attempt {short(run.attempt_id)}</span>
        <span>Turn {short(run.current_turn_id)}</span>
        <span>执行事实：{run.execution_status}</span>
      </div>
      {run.participation === "abandoned" && (
        <p className="notice amber">
          已停止参与 · 历史只读 · 资源保留。执行事实为最后采样，不代表 worker
          已停止。
          {run.candidate_stop_reason === "manual" && (
            <>
              {" "}
              人工停止：{run.candidate_stopped_by} ·{" "}
              {run.candidate_ended_at &&
                new Date(run.candidate_ended_at).toLocaleString("zh-CN")}
            </>
          )}
        </p>
      )}
      <div className="workspace-path">
        <span>工作区</span>
        <code>{run.worktree_path || "准备中"}</code>
        <span className="branch-name">{run.worktree_branch}</span>
      </div>
      <RunnerWorkflow run={run} />
      {run.recovery_required && (
        <p className="notice amber">
          原执行需要恢复核验。{run.stage_diagnostic || run.error}
        </p>
      )}
      {run.environment_blocker && (
        <p className="notice amber">
          {run.environment_blocker.fact.Reason} ·{" "}
          {run.environment_blocker.fact.Detail}
        </p>
      )}
      {run.error && <p className="notice amber">{run.error}</p>}
      {run.decision && (
        <Answer
          key={draftKey(api.identity.project_id, api.identity.instance_id, run)}
          run={run}
        />
      )}
      <LastMessage
        key={"message/" + run.execution_id + "/" + run.attempt_id}
        run={run}
      />
      <Output key={run.execution_id + "/" + run.attempt_id} run={run} />
      <div className="pr-panel">
        <div>
          <h3>PR 与交付事实</h3>
          <SafeLink url={run.pr.url}>
            {run.pr.url ? "查看 PR" : "尚未观察到 PR"}{" "}
            <ArrowUpRight size={13} />
          </SafeLink>
          <span className="muted"> · {run.pr.state || "未发布"}</span>
          {run.pr.query_error && (
            <p className="notice amber">PR 查询失败：{run.pr.query_error}</p>
          )}
          {run.pr.merge_commit && (
            <p className="muted">合并提交 {run.pr.merge_commit}</p>
          )}
        </div>
      </div>
    </>
  );
}
export function Runs() {
  const { api, online, snapshot, requests } = useWorkspace();
  const [pending, setPending] = useState<{
    envelope: Envelope;
    epoch: number;
    receipt?: Receipt;
  }>();
  const selectionEpoch = useRef(0);
  const [params, setParams] = useSearchParams();
  const taskID = params.get("task") ?? "";
  const runID = params.get("run") ?? "";
  const [filter, setFilter] = useState("all");
  const page = snapshot?.connection.snapshot.page;
  const runs = page?.runs ?? [];
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
  const candidates = runs.filter(
    (r) => (r.domain_task_id || r.task_id) === selectedTask,
  );
  const selectedDependencyReason = taskDependencyReason(
    tasks.data?.tasks.find((task) => task.task_id === selectedTask),
  );
  const selected = runID
    ? candidates.find((r) => r.run_id === runID)
    : candidates[0];
  const visible =
    tasks.data?.tasks.filter(
      (t) =>
        filter === "all" ||
        runs.some(
          (r) =>
            (r.domain_task_id || r.task_id) === t.task_id &&
            candidateState(r) === filter,
        ),
    ) ?? [];
  const chooseTask = (id: string) => {
    selectionEpoch.current++;
    setParams({ task: id });
  };
  const chooseRun = (id: string) => {
    selectionEpoch.current++;
    setParams({ task: selectedTask, run: id });
  };
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
      selected?.run_id !== stopped.run_id
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
  }, [pending, requests, runs, selected?.run_id, setParams]);
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
          <div className="eyebrow">PROJECT / EXECUTION</div>
          <h1>执行与待处理</h1>
          <p>每个候选保留独立输出、问题和交付记录。</p>
        </div>
        <Layers className="heading-icon" size={30} />
      </div>
      <div className="toolbar">
        <div className="segmented">
          {[
            ["all", "全部"],
            ["running", "运行中"],
            ["waiting_for_input", "待回答"],
            ["waiting_for_merge", "待合并"],
          ].map(([value, title]) => (
            <Button
              key={value}
              size="sm"
              variant={filter === value ? "default" : "ghost"}
              onClick={() => setFilter(value)}
            >
              {title}
            </Button>
          ))}
        </div>
        <span className="muted">{runs.length} 个候选</span>
      </div>
      <div className="runs-layout">
        <aside className="run-task-list">
          {tasks.isPending && <div className="empty">正在读取正式任务…</div>}
          {tasks.data?.tasks.length === 0 && (
            <div className="empty">还没有正式任务</div>
          )}
          {visible.map((t) => (
            <RunTask
              key={t.task_id}
              task={t}
              selected={t.task_id === selectedTask}
              onSelect={() => chooseTask(t.task_id)}
              candidateCount={
                runs.filter(
                  (r) => (r.domain_task_id || r.task_id) === t.task_id,
                ).length
              }
            />
          ))}
          {tasks.data?.has_more && (
            <p className="muted">
              显示前 100 个任务，可从 DAG 详情定位后续任务。
            </p>
          )}
          {tasks.error && <p className="notice amber">{String(tasks.error)}</p>}
        </aside>
        <section className="execution-panel">
          <div className="candidate-strip">
            {candidates.map((r) => (
              <button
                key={r.run_id}
                onClick={() => chooseRun(r.run_id)}
                className={
                  "candidate-card " +
                  (selected?.run_id === r.run_id ? "active" : "")
                }
              >
                <span>候选 C{r.candidate_number || "—"}</span>
                <strong>{r.execution_profile?.name || r.agent}</strong>
                <Status value={candidateState(r)} />
              </button>
            ))}
          </div>
          {selected ? (
            <CandidateDetail
              key={selected.run_id}
              run={selected}
              stopControl={
                <StopCandidate
                  run={selected}
                  pending={stopPending(selected)}
                  onRequest={(envelope) =>
                    setPending({ envelope, epoch: selectionEpoch.current })
                  }
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
              {selectedDependencyReason
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
