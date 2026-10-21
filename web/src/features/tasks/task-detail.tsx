import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import { ArrowUpRight, Play, Plus } from "lucide-react";
import { useWorkspace } from "../../app/context";
import { projectQueries } from "../../shared/api/queries";
import { Button } from "../../shared/ui/button";
import { participating } from "../runs/state";
import { SafeLink, Status } from "../../shared/ui/status";

export function TaskDetail({
  id,
  allowCandidate = false,
}: {
  id: string;
  allowCandidate?: boolean;
}) {
  const { api, online, snapshot, candidateStarts } = useWorkspace();
  const navigate = useNavigate();
  const [starting, setStarting] = useState(false);
  const [profile, setProfile] = useState("");
  const detail = useQuery({
    ...projectQueries(api).task(id),
    enabled: online,
    refetchInterval: 2000,
  });
  const presentation = useQuery({
    ...projectQueries(api).presentation(),
    enabled: online,
  });
  const task = detail.data?.tasks[0];
  const page = snapshot?.connection.snapshot.page;
  const candidates = (page?.runs ?? []).filter(
    (run) => (run.domain_task_id || run.task_id) === id,
  );
  const pendingStarts =
    candidateStarts?.items.filter(
      (entry) =>
        entry.taskID === id &&
        !task?.workflow_runs?.some((run) => run.workflow_run_id === entry.id) &&
        !page?.runs?.some((run) => run.run_id === entry.id),
    ) ?? [];
  const hasCandidates =
    (task?.workflow_runs?.length ?? 0) > 0 ||
    candidates.length > 0 ||
    pendingStarts.some((entry) => entry.participating);
  const awaitingClaim =
    task?.status === "open" &&
    (candidates.length > 0 ||
      pendingStarts.some((entry) => entry.participating));
  const participatingIDs = new Set([
    ...candidates.filter(participating).map((run) => run.run_id),
    ...(task?.workflow_runs
      ?.filter((run) => run.participation === "active")
      .map((run) => run.workflow_run_id) ?? []),
    ...pendingStarts
      .filter((entry) => entry.participating)
      .map((entry) => entry.id),
  ]);
  const action = hasCandidates ? "start_candidate" : "start_task";
  const profiles = presentation.data?.presentation.profiles ?? [];
  const chosen =
    profile || presentation.data?.presentation.default_profile_id || "";
  const selected = profiles.find((p) => p.id === chosen);
  const supported = page?.service.capabilities?.includes(action);
  const dependenciesMissing = hasCandidates
    ? task?.predecessors?.some((p) => !p.delivered)
    : !task?.dependency_ready;
  const blockers = hasCandidates
    ? task?.startup_blockers?.filter((value) => value !== "task_not_open")
    : task?.startup_blockers;
  const startReason = !online
    ? "服务未连接"
    : detail.isError
      ? "任务查询失败"
      : !supported
        ? "服务未提供此动作"
        : page?.control.dispatch_paused
          ? "派发已暂停，请先恢复"
          : page?.control.dispatch_mode !== "manual"
            ? "手动启动要求手动派发模式"
            : task?.delivery
              ? "任务已交付"
              : task?.status === "discarded"
                ? "任务已放弃"
                : dependenciesMissing
                  ? "前置尚未交付"
                  : blockers?.length
                    ? blockers.join("；")
                    : hasCandidates &&
                        !awaitingClaim &&
                        task?.status !== "in_progress" &&
                        !candidates.some(
                          (run) => run.workflow_stage === "start.submit",
                        )
                      ? "新增候选需要进行中的任务"
                      : hasCandidates &&
                          candidates.some((run) => run.pr.state === "merged")
                        ? "已有候选 PR 合并"
                        : hasCandidates && participatingIDs.size >= 5
                          ? "已达到 5 个参与候选"
                          : "";
  const profileReason =
    !selected || selected.unavailable_reason
      ? selected?.unavailable_reason || "没有可用执行方案"
      : "";
  const reason = startReason || profileReason;
  if (!task)
    return (
      <div className="empty">
        {detail.error ? String(detail.error) : "正在读取任务详情…"}
      </div>
    );
  return (
    <section className="task-detail">
      {detail.error && <p className="notice amber">{String(detail.error)}</p>}
      {presentation.error && (
        <p className="notice amber">{String(presentation.error)}</p>
      )}
      <div className="eyebrow">正式任务 · #{task.task_id}</div>
      <h2>{task.title}</h2>
      <Status value={task.status} />
      <div className="detail-section">
        <h3>依赖与阻塞</h3>
        {task.predecessors?.length ? (
          task.predecessors.map((p) => (
            <div className="dependency" key={p.task_id}>
              <span>#{p.task_id}</span>
              <span>
                {p.delivered ? "前置已交付" : p.blocker || "等待前置交付"}
              </span>
            </div>
          ))
        ) : (
          <p className="muted">没有前置依赖</p>
        )}
        {blockers?.map((b) => (
          <p className="notice amber" key={b}>
            {b}
          </p>
        ))}
        {task.stop_pending?.map((b) => (
          <p className="notice amber" key={b}>
            停止待核实 · {b}
          </p>
        ))}
      </div>
      <div className="detail-section">
        <h3>固定规格</h3>
        <p className="path">{task.spec_path || "未关联规格"}</p>
        <p className="muted">
          Task v{task.version} · 图 v{detail.data?.graph_revision}
        </p>
      </div>
      <div className="detail-section">
        <h3>交付与执行</h3>
        {task.delivery ? (
          <>
            <SafeLink url={task.delivery.pr_url}>
              查看交付 PR <ArrowUpRight size={13} />
            </SafeLink>
            <p className="muted">{task.delivery.merged_at}</p>
          </>
        ) : (
          <p className="muted">尚未核实任务交付</p>
        )}
        {hasCandidates && (
          <Button asChild>
            <Link
              to={
                "/projects/" +
                api.identity.project_id +
                "/runs?task=" +
                encodeURIComponent(id)
              }
            >
              查看执行 <ArrowUpRight size={14} />
            </Link>
          </Button>
        )}
        {!task.delivery && (!hasCandidates || allowCandidate) && (
          <Button
            variant="default"
            disabled={!!startReason || !candidateStarts}
            title={startReason}
            onClick={() => setStarting(!starting)}
          >
            {hasCandidates ? <Plus size={14} /> : <Play size={14} />}{" "}
            {hasCandidates ? "新增并行候选" : "手动启动"}
          </Button>
        )}
        {startReason && !task.delivery && (
          <p className="muted">{startReason}</p>
        )}
      </div>
      {starting && (
        <div className="start-panel">
          <label htmlFor="execution-profile">本次执行方案</label>
          <select
            id="execution-profile"
            value={chosen}
            onChange={(e) => setProfile(e.target.value)}
          >
            {profiles.map((p) => (
              <option key={p.id} value={p.id} disabled={!!p.unavailable_reason}>
                {p.name} · {p.host}/{p.agent}
              </option>
            ))}
          </select>
          <p className="muted">
            使用服务已保存的冻结方案。本次选择不改变全局默认。
          </p>
          {selected?.details?.map((d, i) => (
            <p key={i} className="muted">
              {d}
            </p>
          ))}
          {reason && <p className="muted">{reason}</p>}
          <div className="actions">
            <Button onClick={() => setStarting(false)}>取消</Button>
            <Button
              variant="default"
              disabled={!!reason || !candidateStarts}
              onClick={() => {
                if (!selected || !candidateStarts || reason) return;
                const requestID = candidateStarts.start(
                  task,
                  selected,
                  detail.data!.graph_revision,
                  page?.control.revision ?? 0,
                );
                if (!requestID) return;
                setStarting(false);
                navigate(
                  `/projects/${api.identity.project_id}/runs?task=${encodeURIComponent(id)}&run=${requestID}`,
                );
              }}
            >
              确认启动
            </Button>
          </div>
        </div>
      )}
    </section>
  );
}
