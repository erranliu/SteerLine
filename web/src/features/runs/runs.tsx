import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import {
  ArrowUpRight,
  Activity as ActivityIcon,
  Layers,
  Square,
} from "lucide-react";
import { useWorkspace } from "../../app/context";
import type { Candidate } from "../../shared/api/types";
import { projectQueries } from "../../shared/api/queries";
import { Button } from "../../shared/ui/button";
import { SafeLink, Status, short } from "../../shared/ui/status";
import { TaskDetail } from "../tasks/task-detail";
import { Answer, draftKey } from "./answer";
import { candidateState, stopReason } from "./state";

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
          {online ? "定时刷新" : "最后观察"} ·{" "}
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
          {activity.data?.page.activity?.map((event) => (
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
function CandidateDetail({ run }: { run: Candidate }) {
  const { api, online, send, busy, snapshot } = useWorkspace();
  const [stop, setStop] = useState(false);
  const caps = snapshot?.connection.snapshot.page?.service.capabilities ?? [];
  const revision = snapshot?.connection.snapshot.page?.control.revision ?? 0;
  const stopUnavailable = stopReason(
    run,
    snapshot?.connection.snapshot.page?.runs ?? [],
  );
  const canOpen =
    online && !!run.session_id && !!run.session_generation && !!run.attempt_id;
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
        <Button
          disabled={!canOpen || busy}
          title={!canOpen ? "原会话绑定缺失或服务离线" : ""}
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
      <div className="run-meta">
        <span>Run {short(run.run_id)}</span>
        <span>Execution {short(run.execution_id)}</span>
        <span>Attempt {short(run.attempt_id)}</span>
        <span>Turn {short(run.current_turn_id)}</span>
        <span>执行事实：{run.execution_status}</span>
      </div>
      <div className="workspace-path">
        <span>工作区</span>
        <code>{run.worktree_path || "准备中"}</code>
        <span className="branch-name">{run.worktree_branch}</span>
      </div>
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
        <Button
          variant="destructive"
          disabled={
            !online ||
            busy ||
            !caps.includes("stop_candidate") ||
            !!stopUnavailable
          }
          title={stopUnavailable}
          onClick={() => setStop(true)}
        >
          <Square size={13} />
          停止参与
        </Button>
      </div>
      {stop && (
        <div className="notice amber">
          <strong>停止候选 C{run.candidate_number}</strong>
          <p>
            取消本候选的后续业务推进与交付资格；保留 worker、会话、PR
            和工作区，原 worker 可能继续已有工作。
          </p>
          <div className="actions">
            <Button onClick={() => setStop(false)}>取消</Button>
            <Button
              variant="destructive"
              disabled={!online || busy}
              onClick={async () => {
                const accepted = await send(
                  "stop_candidate",
                  run.run_id,
                  run.control_revision,
                  { task_id: run.domain_task_id || run.task_id },
                  { expected_control_revision: revision },
                );
                if (accepted) setStop(false);
              }}
            >
              确认停止
            </Button>
          </div>
        </div>
      )}
    </>
  );
}
export function Runs() {
  const { api, online, snapshot } = useWorkspace();
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
  const chooseTask = (id: string) => setParams({ task: id });
  const chooseRun = (id: string) => setParams({ task: selectedTask, run: id });
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
            <button
              key={t.task_id}
              onClick={() => chooseTask(t.task_id)}
              className={
                "run-task " + (t.task_id === selectedTask ? "active" : "")
              }
            >
              <span className="eyebrow">TASK #{t.task_id}</span>
              <strong>{t.title}</strong>
              <div>
                <Status value={t.status} />
                <span className="muted">
                  {
                    runs.filter(
                      (r) => (r.domain_task_id || r.task_id) === t.task_id,
                    ).length
                  }{" "}
                  个候选
                </span>
              </div>
            </button>
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
            <CandidateDetail key={selected.run_id} run={selected} />
          ) : (
            <div className="empty">
              此任务尚无执行候选，请从任务详情手动启动。
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
