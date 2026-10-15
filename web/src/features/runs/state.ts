import type { Candidate } from "../../shared/api/types";
const participatingStates = new Set([
  "queued",
  "running",
  "waiting_input",
  "blocked",
  "reconciling",
  "waiting_external",
]);
export function participating(run: Candidate) {
  return participatingStates.has(run.workflow_status);
}
export function businessOpen(run: Candidate) {
  return (
    participating(run) &&
    !["closed", "completed", "discarded"].includes(run.task_status)
  );
}
export function waitingForMerge(run: Candidate) {
  return (
    businessOpen(run) &&
    run.workflow_stage === "await_merge" &&
    run.workflow_status === "waiting_external"
  );
}
export function requiresAnswer(run: Candidate) {
  return (
    businessOpen(run) &&
    (run.workflow_status === "waiting_input" ||
      (run.decision?.status === "pending" &&
        !waitingForMerge(run) &&
        (run.decision.request.interaction_kind !== "async" ||
          run.decision.request.blocks_progress)))
  );
}
export function candidateState(run: Candidate) {
  if (run.workflow_status === "abandoned") return "abandoned";
  if (run.execution_status === "running") return "running";
  if (requiresAnswer(run)) return "waiting_for_input";
  if (waitingForMerge(run)) return "waiting_for_merge";
  if (run.delivery_outcome === "winner") return "delivered";
  return run.execution_status;
}
export function stopReason(run: Candidate, runs: Candidate[]) {
  if (!participating(run)) return "候选已结束，历史只读；资源保留";
  const task = run.domain_task_id || run.task_id;
  if (
    runs.filter(
      (other) =>
        (other.domain_task_id || other.task_id) === task &&
        participating(other),
    ).length <= 1
  )
    return "只剩一个参与候选，不能停止";
  return "";
}
