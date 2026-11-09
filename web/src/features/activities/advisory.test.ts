import { expect, it } from "vitest";
import { activityDestination, type Activity } from "./model";
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
