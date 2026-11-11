import type { CandidateStart } from "../../app/use-candidate-starts";
import { Status } from "../../shared/ui/status";

export function latestTaskStarts(items: CandidateStart[] = []) {
  const starts = new Map<string, CandidateStart>();
  for (const item of items) {
    if (!item.initial) continue;
    const previous = starts.get(item.taskID)?.initial;
    if (!previous || previous.requestedAt <= item.initial.requestedAt)
      starts.set(item.taskID, item);
  }
  return starts;
}

export function taskStartStatus(status: string, start?: CandidateStart) {
  if (status !== "open") return undefined;
  switch (start?.initial?.phase) {
    case "submitting":
    case "applied":
      return "task_starting";
    case "unknown":
      return "task_start_unknown";
    case "rejected":
    case "unsent":
      return "task_start_failed";
  }
}

export function TaskStatus({
  status,
  startStatus,
}: {
  status: string;
  startStatus?: ReturnType<typeof taskStartStatus>;
}) {
  return (
    <span className="task-status">
      {startStatus === "task_start_failed" ? (
        <>
          <Status value={status} />
          <Status value={startStatus} />
        </>
      ) : (
        <Status value={startStatus ?? status} />
      )}
    </span>
  );
}
