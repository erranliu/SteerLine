import { LockKeyhole } from "lucide-react";
import type { Task } from "../../shared/api/types";
import { Status } from "../../shared/ui/status";

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
  onSelect,
}: {
  task: Task;
  selected: boolean;
  candidateCount: number;
  onSelect: () => void;
}) {
  const reason = taskDependencyReason(task);
  return (
    <button
      onClick={onSelect}
      aria-current={selected ? "true" : undefined}
      className={
        "run-task" +
        (reason ? " dependency-blocked" : "") +
        (selected ? " active" : "")
      }
    >
      <span className="eyebrow">TASK #{task.task_id}</span>
      <strong>{task.title}</strong>
      <div>
        <Status value={reason ? "dependency_blocked" : task.status} />
        <span className="muted">{candidateCount} 个候选</span>
      </div>
      {reason && (
        <span className="run-task-blocker">
          <LockKeyhole size={12} aria-hidden="true" />
          <span>{reason}</span>
        </span>
      )}
    </button>
  );
}
