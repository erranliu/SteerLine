import { LockKeyhole } from "lucide-react";
import type { CandidateStart } from "../../app/use-candidate-starts";
import type { Task } from "../../shared/api/types";
import { TaskStatus, taskStartStatus } from "../tasks/task-status";

export function taskDependencyReason(task?: Task): string {
  if (!task || task.status !== "open" || task.dependency_ready) return "";
  const waiting = task.predecessors?.filter((p) => !p.delivered) ?? [];
  return waiting.length
    ? `等待 ${waiting.map((p) => "#" + p.task_id).join("、")} 交付`
    : "等待前置任务交付";
}

export function RunTask({
  task,
  selected,
  candidateCount,
  summary,
  matchedCount,
  initialStart,
  onSelect,
}: {
  task: Task;
  selected: boolean;
  candidateCount: number;
  summary?: string;
  matchedCount?: number;
  initialStart?: CandidateStart;
  onSelect: () => void;
}) {
  const startStatus = taskStartStatus(task.status, initialStart);
  const reason =
    !startStatus || startStatus === "task_start_failed"
      ? taskDependencyReason(task)
      : "";
  return (
    <button
      onClick={onSelect}
      aria-current={selected ? "true" : undefined}
      data-execution-state={startStatus ?? task.status}
      className={
        "run-task" +
        (reason ? " dependency-blocked" : "") +
        (selected ? " active" : "")
      }
    >
      <span className="eyebrow">TASK #{task.task_id}</span>
      <strong>{task.title}</strong>
      <div>
        <TaskStatus
          status={reason ? "dependency_blocked" : task.status}
          startStatus={startStatus}
        />
        <span className="muted">{candidateCount} 个候选</span>
      </div>
      {summary && <span className="run-task-summary">{summary}</span>}
      {matchedCount !== undefined && (
        <span className="run-task-summary muted">
          {matchedCount} / {candidateCount} 个候选命中
        </span>
      )}
      {reason && (
        <span className="run-task-blocker">
          <LockKeyhole size={12} aria-hidden="true" />
          <span>{reason}</span>
        </span>
      )}
    </button>
  );
}
