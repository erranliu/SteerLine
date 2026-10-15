import { it, expect } from "vitest";
import { candidateState, stopReason, businessOpen } from "./state";
import type { Candidate } from "../../shared/api/types";
const base = {
  task_id: "task",
  run_id: "a",
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
      workflow_status: "waiting_input",
      decision: { status: "pending", request: { blocks_progress: true } },
    } as Candidate),
  ).toBe("waiting_for_input");
});
it("stopped participation preserves the execution fact and cannot stop the last participant", () => {
  const stopped = {
    ...base,
    run_id: "stopped",
    workflow_status: "abandoned",
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
});

it("keeps public Runner pr_ready candidates open for feedback and merge", () => {
  const run = {
    ...base,
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
  expect(businessOpen({ ...run, workflow_status: "abandoned" })).toBe(false);
});
