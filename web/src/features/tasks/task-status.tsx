import type { CandidateStart } from "../../app/use-candidate-starts";
import { Status } from "../../shared/ui/status";

export function latestTaskStarts(items: CandidateStart[] = []) {
  const starts = new Map<string, CandidateStart>();
  for (const item of items) {
    if (!item.initialRequestedAt) continue;
    const previous = starts.get(item.taskID)?.initialRequestedAt;
    if (!previous || previous <= item.initialRequestedAt)
      starts.set(item.taskID, item);
  }
  return starts;
}

export function candidateStartStatus(start: CandidateStart) {
  switch (start.phase) {
    case "submitting":
    case "waiting":
    case "applied":
      return "task_starting";
    case "unknown":
      return "task_start_unknown";
    case "rejected":
    case "unsent":
      return "task_start_failed";
  }
}

export function taskStartStatus(status: string, start?: CandidateStart) {
  if (status === "open" && start?.initialRequestedAt)
    return candidateStartStatus(start);
}

export function TaskStatus({
  status,
  startStatus,
}: {
  status: string;
  startStatus?: ReturnType<typeof taskStartStatus>;
}) {
  return (
    <span className="execution-statuses">
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
