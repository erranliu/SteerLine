import type { CandidateStart } from "../../app/use-candidate-starts";
import type { Task } from "../../shared/api/types";
import { TaskStatus, taskStartStatus } from "../tasks/task-status";

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
  return (
    <button
      onClick={onSelect}
      aria-current={selected ? "true" : undefined}
      data-execution-state={startStatus ?? task.status}
      className={"run-task" + (selected ? " active" : "")}
    >
      <span className="eyebrow">TASK #{task.task_id}</span>
      <strong>{task.title}</strong>
      <div>
        <TaskStatus status={task.status} startStatus={startStatus} />
        <span className="muted">{candidateCount} 个候选</span>
      </div>
      {summary && <span className="run-task-summary">{summary}</span>}
      {matchedCount !== undefined && (
        <span className="run-task-summary muted">
          {matchedCount} / {candidateCount} 个候选命中
        </span>
      )}
    </button>
  );
}
