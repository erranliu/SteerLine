import type { Candidate } from "../../shared/api/types";
const actionableStates = new Set([
  "queued",
  "running",
  "waiting_input",
  "blocked",
  "reconciling",
  "waiting_external",
  "pr_ready",
]);
export function participating(run: Candidate) {
  return run.participation === "active";
}
export function businessOpen(run: Candidate) {
  return (
    participating(run) &&
    !run.stop_requested &&
    actionableStates.has(run.workflow_status) &&
    !["closed", "completed", "discarded"].includes(run.task_status)
  );
}
export function waitingForMerge(run: Candidate) {
  return (
    businessOpen(run) &&
    (run.workflow_status === "pr_ready" ||
      (run.workflow_stage === "await_merge" &&
        run.workflow_status === "waiting_external"))
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
  if (run.participation === "abandoned") return "abandoned";
  if (run.delivery_outcome === "winner") return "delivered";
  if (run.participation === "completed") return "completed";
  if (run.execution_status === "running") return "running";
  if (requiresAnswer(run)) return "waiting_for_input";
  if (waitingForMerge(run)) return "waiting_for_merge";
  return run.execution_status;
}
export function stopReason(run: Candidate, runs: Candidate[]) {
  if (!participating(run)) return "候选已结束，历史只读；资源保留";
  if (run.stop_requested) return "候选停止待核实";
  if (
    !run.run_id ||
    !run.control_revision ||
    !run.candidate_number ||
    !runs.some((other) => other.run_id === run.run_id)
  )
    return "等待服务器确认候选身份与版本";
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
