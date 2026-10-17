import { it, expect } from "vitest";
import { candidateState, stopReason, businessOpen } from "./state";
import type { Candidate } from "../../shared/api/types";
const base = {
  task_id: "task",
  run_id: "a",
  candidate_number: 1,
  control_revision: 1,
  participation: "active",
  submission_status: "accepted",
  stop_requested: false,
  workflow_status: "waiting_external",
  workflow_stage: "await_merge",
  execution_status: "succeeded",
  pr: { state: "open" },
} as Candidate;
it("shows verified waiting-merge without turning optional feedback into a required answer", () => {
  expect(
    candidateState({
      ...base,
      decision: { status: "pending", request: { blocks_progress: false } },
    } as Candidate),
  ).toBe("waiting_for_merge");
  expect(candidateState({ ...base, execution_status: "running" })).toBe(
    "running",
  );
  expect(
    candidateState({
      ...base,
      participation: "active",
      submission_status: "accepted",
      stop_requested: false,
      workflow_status: "waiting_input",
      decision: { status: "pending", request: { blocks_progress: true } },
    } as Candidate),
  ).toBe("waiting_for_input");
});
it("stopped participation preserves the execution fact and cannot stop the last participant", () => {
  const stopped = {
    ...base,
    run_id: "stopped",
    participation: "abandoned",
    submission_status: "accepted",
    stop_requested: false,
    workflow_status: "running",
    execution_status: "running",
  };
  expect(candidateState(stopped)).toBe("abandoned");
  expect(stopped.execution_status).toBe("running");
  expect(
    stopReason(base, [
      base,
      stopped,
      { ...base, run_id: "other", task_id: "other-task" },
    ]),
  ).toContain("只剩一个");
  expect(
    stopReason(base, [
      base,
      { ...base, run_id: "second", workflow_status: "blocked" },
    ]),
  ).toBe("");
  expect(stopReason(stopped, [base, stopped])).toContain("历史只读");
  expect(
    stopReason({ ...base, control_revision: 0 }, [
      base,
      { ...base, run_id: "second" },
    ]),
  ).toContain("等待服务器确认");
});

it("keeps public Runner pr_ready candidates open for feedback and merge", () => {
  const run = {
    ...base,
    participation: "active",
    submission_status: "accepted",
    stop_requested: false,
    workflow_status: "pr_ready",
    execution_status: "pr_ready",
    task_status: "in_progress",
    decision: {
      status: "pending",
      request: { interaction_kind: "business_feedback", blocks_progress: true },
    },
  } as Candidate;
  expect(businessOpen(run)).toBe(true);
  expect(candidateState(run)).toBe("waiting_for_merge");
  expect(businessOpen({ ...run, task_status: "closed" })).toBe(false);
  expect(businessOpen({ ...run, participation: "abandoned" })).toBe(false);
});

it("keeps participation, delivery and raw observation independent", () => {
  const winner = {
    ...base,
    participation: "completed",
    delivery_outcome: "winner",
    workflow_status: "running",
    execution_status: "running",
  };
  expect(candidateState(winner)).toBe("delivered");
  expect(winner.execution_status).toBe("running");
  expect(businessOpen(winner)).toBe(false);
  expect(candidateState({ ...winner, delivery_outcome: "" })).toBe("completed");
  for (const status of ["pending", "unknown", "future_runner_status"]) {
    expect(businessOpen({ ...base, workflow_status: status })).toBe(false);
    expect(
      stopReason({ ...base, workflow_status: status }, [
        base,
        { ...base, run_id: "second" },
      ]),
    ).toBe("");
    expect(
      stopReason(base, [
        base,
        { ...base, run_id: "unknown", workflow_status: status },
      ]),
    ).toBe("");
  }
  expect(businessOpen({ ...base, stop_requested: true })).toBe(false);
  expect(stopReason({ ...base, stop_requested: true }, [base])).toContain(
    "停止待核实",
  );
  expect(businessOpen({ ...base, task_status: "discarded" })).toBe(false);
});
