import { expect, it } from "vitest";
import { activityDestination, steerAgentLabel, type Activity } from "./model";
it("navigates to the exact advisory without creating a new request", () => {
  const a = {
    details_ref: {
      kind: "advisory",
      target: "original-request",
      task_id: "task",
      candidate_id: "candidate",
    },
  } as Activity;
  expect(activityDestination("project", a)).toBe(
    "/projects/project/runs?task=task&run=candidate&advisory=original-request",
  );
});
it("keeps unrelated and unknown activity classifications unlabeled", () => {
  expect(steerAgentLabel({} as Activity)).toBeUndefined();
  for (const type of ["future", "constructor", "__proto__", "toString"]) {
    expect(
      steerAgentLabel({
        steer_agent: { type, operation: "future" },
      } as Activity),
    ).toBeUndefined();
  }
});
