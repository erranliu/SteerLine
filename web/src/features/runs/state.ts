import type { Candidate, Task } from "../../shared/api/types";

export const candidateFilters = [
  ["all", "全部"],
  ["running", "运行中"],
  ["waiting_for_input", "待回答"],
  ["waiting_for_merge", "待合并"],
  ["needs_attention", "需处理"],
] as const;
export type CandidateFilter = (typeof candidateFilters)[number][0];

// Execution, questions and recovery are independent facts. A running candidate
// can still have an unanswered asynchronous question.
function currentCandidate(run: Candidate, task?: Task) {
  return (
    participating(run) &&
    !run.stop_requested &&
    run.delivery_outcome !== "winner" &&
    !task?.delivery &&
    ![run.task_status, task?.status].some((status) =>
      ["closed", "completed", "discarded"].includes(status ?? ""),
    )
  );
}

export function candidateFacets(
  run: Candidate,
  task?: Task,
): CandidateFilter[] {
  if (!currentCandidate(run, task)) return [];
  const facets: CandidateFilter[] = [];
  if (run.execution_status === "running") facets.push("running");
  if (
    requiresAnswer(run) ||
    (businessOpen(run) &&
      run.decision?.status === "pending" &&
      run.decision.request.interaction_kind === "async")
  )
    facets.push("waiting_for_input");
  if (waitingForMerge(run)) facets.push("waiting_for_merge");
  if (
    [run.workflow_status, run.execution_status].some((status) =>
      ["blocked", "failed"].includes(status),
    ) ||
    run.recovery_required ||
    run.runner_delivery?.resume_required ||
    run.environment_blocker
  )
    facets.push("needs_attention");
  return facets;
}

export function matchesCandidate(
  run: Candidate,
  filter: CandidateFilter,
  task?: Task,
) {
  return filter === "all" || candidateFacets(run, task).includes(filter);
}

export function optionalAnswer(run: Candidate) {
  return (
    run.decision?.status === "pending" &&
    run.decision.request.interaction_kind === "async" &&
    !run.decision.request.blocks_progress &&
    !requiresAnswer(run)
  );
}
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

export function candidateStatuses(run: Candidate, task?: Task) {
  const primary = candidateState(run);
  if (
    !currentCandidate(run, task) &&
    !["abandoned", "delivered", "completed"].includes(primary)
  ) {
    return [run.stop_requested ? "stop_pending" : "historical"];
  }
  return [...new Set([primary, ...candidateFacets(run, task)])];
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
