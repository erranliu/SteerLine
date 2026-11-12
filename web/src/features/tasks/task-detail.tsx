import { useRef, useState } from "react";
import { useActivityFeedback } from "../activities/feedback/feedback";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { ArrowUpRight, Play, Plus } from "lucide-react";
import {
  environmentLink,
  environmentStage,
  useEnvironmentReadiness,
} from "../environment-checks/use-readiness";
import { useWorkspace } from "../../app/context";
import type { CandidateStart } from "../../app/use-candidate-starts";
import { RoadmapTaskSource } from "../roadmaps/roadmaps";
import { TaskMaterials } from "./task-materials";
import { projectQueries } from "../../shared/api/queries";
import { Button } from "../../shared/ui/button";
import { SafeLink } from "../../shared/ui/status";
import { TaskStatus, taskStartStatus } from "./task-status";

export function TaskDetail({
  id,
  allowCandidate = false,
  startInPlace = false,
  initialStart,
}: {
  id: string;
  allowCandidate?: boolean;
  startInPlace?: boolean;
  initialStart?: CandidateStart;
}) {
  const { api, online, snapshot, candidateStarts } = useWorkspace();
  const feedback = useActivityFeedback();
  const confirmed = useRef(false);
  const navigate = useNavigate();
  const environment = useEnvironmentReadiness();
  const [params] = useSearchParams();
  const restored =
    params.get("task") === id ? params.get("start_profile") || "" : "";
  const [starting, setStarting] = useState(!!restored);
  if (!starting) confirmed.current = false;
  const [profile, setProfile] = useState(restored);
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
  const eligibility = task?.start_eligibility;
  const candidate =
    eligibility?.action === "start_candidate" ||
    pendingStarts.some((entry) => entry.participating);
  const profiles = presentation.data?.presentation.profiles ?? [];
  const chosen = profile || presentation.data?.presentation.default_profile_id || "";
  const selected = profiles.find((item) => item.id === chosen);
  const controlRevision = detail.data?.authority_version;
  const blockers = eligibility?.blockers ?? [];
  const entryReason = !online
    ? "服务未连接"
    : detail.isError
      ? "任务资格查询失败"
      : !eligibility || !controlRevision
        ? "任务启动资格待核实"
        : blockers.map((blocker) => blocker.diagnostic).join("；");
  const profileBlocker = eligibility?.profile_blockers[chosen];
  const profileReason = !selected
    ? "请选择可用执行方案"
    : profileBlocker === undefined
      ? "执行方案资格待核实"
      : profileBlocker?.diagnostic;
  const reason = entryReason || profileReason;
  const projectBlocked = blockers.some(
    ({ code }) => code === "project_not_ready" || code === "project_readiness_unverified",
  );
  const preparation = environment.data?.profiles?.find((item) => item.id === chosen);
  if (!task)
    return (
      <div className="empty">
        {detail.error ? String(detail.error) : "正在读取任务详情…"}
      </div>
    );
  const startStatus = taskStartStatus(task.status, initialStart);
  return (
    <section className="task-detail">
      {detail.error && <p className="notice amber">{String(detail.error)}</p>}
      {presentation.error && (
        <p className="notice amber">{String(presentation.error)}</p>
      )}
      <div className="eyebrow">正式任务 · #{task.task_id}</div>
      <h2>{task.title}</h2>
      <div className="task-detail-status" role="status">
        <TaskStatus status={task.status} startStatus={startStatus} />
        {startStatus && initialStart && (
          <p className="muted">
            {initialStart.status}
            {initialStart.diagnostic && ` · ${initialStart.diagnostic}`}
          </p>
        )}
      </div>
      {(!!task.predecessors?.length ||
        !!task.stop_pending?.length) && (
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
          {task.stop_pending?.map((b) => (
            <p className="notice amber" key={b}>
              停止待核实 · {b}
            </p>
          ))}
        </div>
      )}
      <div className="detail-section">
        <h3>任务文档</h3>
        <p className="path">{task.spec_path || "未关联规格"}</p>
        <RoadmapTaskSource task={id} />
        <TaskMaterials id={id} />
      </div>
      <div className="detail-section">
        <h3>交付与执行</h3>
        {pendingStarts.length > 0 && (
          <button type="button" onClick={feedback.open}>
            查看活动
          </button>
        )}
        {projectBlocked && (
          <Link to={`/projects/${api.identity.project_id}/status?task=${encodeURIComponent(id)}`}>
            前往项目状态 <ArrowUpRight size={13} />
          </Link>
        )}
        {task.delivery ? (
          <>
            <SafeLink url={task.delivery.pr_url}>
              查看交付 PR <ArrowUpRight size={13} />
            </SafeLink>
            <p className="muted">{task.delivery.merged_at}</p>
          </>
        ) : null}
        {hasCandidates && (
          <Button asChild>
            <Link
              to={
                "/projects/" +
                api.identity.project_id +
                "/runs?task=" +
                encodeURIComponent(id) +
                (startStatus && initialStart?.participating
                  ? "&run=" + encodeURIComponent(initialStart.id)
                  : "")
              }
            >
              查看执行 <ArrowUpRight size={14} />
            </Link>
          </Button>
        )}
        {!task.delivery && (!candidate || allowCandidate) && (
          <Button
            variant="default"
            disabled={!!entryReason || !candidateStarts}
            title={entryReason}
            onClick={() => setStarting(!starting)}
          >
            {candidate ? <Plus size={14} /> : <Play size={14} />}{" "}
            {candidate ? "新增并行候选" : "手动启动"}
          </Button>
        )}
        {!task.delivery && entryReason && <p className="muted">{entryReason}</p>}
      </div>
      {starting && (
        <div className="start-panel">
          <label htmlFor="execution-profile">本次执行方案</label>
          <select
            id="execution-profile"
            value={chosen}
            onChange={(e) => setProfile(e.target.value)}
          >
            {chosen && !selected && (
              <option value={chosen}>
                {chosen} · 原方案已不存在，请重新选择
              </option>
            )}
            {profiles.map((p) => (
              <option key={p.id} value={p.id} disabled={!!eligibility?.profile_blockers[p.id] && eligibility.profile_blockers[p.id]?.code !== "environment_not_ready"}>
                {p.name} · {p.host}/{p.agent}
              </option>
            ))}
          </select>
          {selected?.details?.map((d, i) => (
            <p key={i} className="muted">
              {d}
            </p>
          ))}
          <p>
            对应项目运行环境：
            {environment.verified
              ? environmentStage[preparation?.stage || ""] ||
                (preparation?.status === "ready" ? "已就绪" : "待核实")
              : "当前待核实"}
          </p>
          {reason && <p className="muted">{reason}</p>}
          {eligibility?.environment_readiness_required === false && (
            <p className="notice amber">环境就绪阻塞已关闭（开发者配置）</p>
          )}
          {profileBlocker?.code === "environment_not_ready" && (
            <Link to={environmentLink(api.identity.project_id, chosen, id)}>
              前往环境准备
            </Link>
          )}
          <div className="actions">
            <Button onClick={() => setStarting(false)}>取消</Button>
            <Button
              variant="default"
              disabled={!!reason || !candidateStarts}
              onClick={(event) => {
                if (
                  !selected ||
                  !controlRevision ||
                  !candidateStarts ||
                  reason ||
                  confirmed.current
                )
                  return;
                confirmed.current = true;
                const requestID = candidateStarts.start(
                  task,
                  selected,
                  detail.data!.graph_revision,
                  controlRevision,
                );
                if (!requestID) {
                  confirmed.current = false;
                  return;
                }
                feedback.notify("启动任务候选", event.currentTarget);
                setStarting(false);
                if (!startInPlace)
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
