import { useRef } from "react";
import { Link } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import type { ProjectBlocker } from "../../shared/api/types";
import { useProjectReadiness } from "./use-readiness";

const reasonLabels: Record<string, string> = {
  remote_unchecked: "远端尚未检查",
  remote_check_failed: "远端检查失败",
  remote_observation_changed: "远端状态已变化",
  remote_history_rewritten: "远端历史已改写",
  worktree_unverified: "工作区待核实",
  worktree_dirty: "未提交修改",
  git_operation_in_progress: "Git 操作进行中",
  detached_head: "游离 HEAD",
  wrong_branch: "分支不匹配",
  local_behind_remote: "未同步远端",
  local_ahead_remote: "本地提交未发布",
  local_diverged: "本地与远端分叉",
  task_material_unprepared: "任务材料未准备",
  task_material_stale: "任务材料已过期",
  task_material_invalid: "任务材料失效",
  task_material_unverified: "任务材料待核实",
  test_plan_missing: "测试安排缺失",
  test_plan_invalid: "测试安排失效",
  test_catalogue_invalid: "测试清单失效",
};

const categoryLabels: Record<ProjectBlocker["category"], string> = {
  remote: "远端状态异常",
  worktree: "工作区状态异常",
  task_material: "任务材料未就绪",
  test_plan: "测试安排未就绪",
};

export function projectBlockerLabel(blocker: ProjectBlocker) {
  return (
    reasonLabels[blocker.code] ??
    categoryLabels[blocker.category] ??
    "项目状态异常"
  );
}

export function ProjectReadinessStatus() {
  const { identity } = useWorkspace();
  const project = useProjectReadiness();
  const details = useRef<HTMLDetailsElement>(null);
  const summary = useRef<HTMLElement>(null);
  const root = `/projects/${identity.project_id}`;
  const reasons = [
    ...new Map(
      project.blockers.map((blocker) => [blocker.code, blocker]),
    ).values(),
  ];
  const extra = reasons.length - 2;
  const label = project.verified ? project.text : "项目未就绪（待核实）";
  function close() {
    if (details.current) details.current.open = false;
  }
  return (
    <details
      className="project-readiness-status"
      ref={details}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) close();
      }}
      onKeyDown={(event) => {
        if (event.key === "Escape" && details.current?.open) {
          close();
          summary.current?.focus();
        }
      }}
    >
      <summary className={project.ready ? "ready" : ""} ref={summary}>
        <span aria-live="polite">
          {label}
          {reasons.length > 0 &&
            ` · ${reasons.slice(0, 2).map(projectBlockerLabel).join("、")}`}
          {extra > 0 && (
            <span aria-label={`另有 ${extra} 项原因`}> +{extra}</span>
          )}
        </span>
      </summary>
      <nav aria-label="项目就绪状态导航">
        <Link to={root + "/status"} onClick={close}>
          项目状态
        </Link>
        <Link to={root + "/tests"} onClick={close}>
          测试管理
        </Link>
      </nav>
    </details>
  );
}
