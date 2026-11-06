import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import type { Candidate } from "../../shared/api/types";
import {
  identity,
  mountFeature,
  run,
  snapshot,
  testClient,
} from "../../test/workspace";
import { ResolvePRConflict } from "./resolve-pr-conflict";

afterEach(cleanup);
const candidate = {
  ...run,
  control_revision: 7,
  workflow_status: "pr_ready",
  pr: { state: "open", url: "https://example.test/pr/1" },
  runner_delivery: {
    available: true,
    execution_wait: null,
    revision: 8,
    submission: "accepted",
    final_verified: true,
    closed: false,
    resume_required: false,
    new_inputs_allowed: true,
    controls: {
      can_message: true,
      can_reply: false,
      can_resume: false,
      can_stop: true,
      can_interrupt: false,
      can_close: false,
      reason: "",
    },
    commands: [],
  },
} as Candidate;
function mount(value = candidate, overrides = {}) {
  const api = new Api(identity, "csrf", "home");
  const state = snapshot(api, [value]);
  state.connection.snapshot.page!.service.capabilities.push(
    "resolve_pr_conflict",
  );
  const send = vi.fn(async () => true);
  mountFeature(<ResolvePRConflict run={value} />, api, testClient(), true, {
    snapshot: state,
    send,
    ...overrides,
  });
  return send;
}
it("sends one original candidate command without a prompt editor or confirmation", async () => {
  const send = mount();
  const button = screen.getByRole("button", { name: "解决合并冲突" });
  fireEvent.click(button);
  fireEvent.click(button);
  await waitFor(() => expect(send).toHaveBeenCalledTimes(1));
  expect(send).toHaveBeenCalledWith(
    "resolve_pr_conflict",
    "run",
    7,
    { task_id: "1" },
    { expected_control_revision: 1 },
  );
  expect(button).toBeDisabled();
});
it("does not enable an unsupported or busy original run", () => {
  mount({
    ...candidate,
    runner_delivery: {
      ...candidate.runner_delivery!,
      controls: { ...candidate.runner_delivery!.controls, can_message: false },
    },
  });
  expect(screen.getByRole("button", { name: "解决合并冲突" })).toBeDisabled();
});
it.each(["abandoned", "closed", "merged"])(
  "does not offer a terminal target (%s)",
  (state) => {
    mount(
      state === "abandoned"
        ? { ...candidate, participation: state }
        : { ...candidate, pr: { ...candidate.pr, state } },
    );
    expect(screen.queryByRole("button", { name: "解决合并冲突" })).toBeNull();
  },
);
