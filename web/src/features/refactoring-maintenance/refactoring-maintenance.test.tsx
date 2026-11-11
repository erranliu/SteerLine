import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, testClient } from "../../test/workspace";
import {
  RefactoringMaintenance,
  type MaintenanceRecord,
} from "./refactoring-maintenance";
import { activityDestination, requestDestination } from "../activities/model";
import type { Activity } from "../activities/model";
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
const id = "a".repeat(32);
const record: MaintenanceRecord = {
  id,
  revision: 7,
  phase: "waiting_input",
  input: {
    direction: "simplify",
    requirements: "",
    profile: "local",
    strategy: "cautious",
    budget: { rounds: 1 },
  },
  round: 1,
  active: true,
  source_commit: "b".repeat(40),
  elapsed_seconds: 65,
  cost_complete: false,
  question: { id: "question", text: "保留原公共接口还是允许调整？建议保留。" },
  can_finish: true,
  can_stop: true,
  can_conflict: false,
  operations: [],
  reports: [],
};
function setup(records: MaintenanceRecord[] = []) {
  const api = new Api(identity, "token", "home");
  vi.spyOn(api, "get").mockImplementation(async (path: string) =>
    path === "/refactoring"
      ? {
          records,
          profiles: [{ id: "local", name: "本机 Codex" }],
          default_profile: "local",
        }
      : path.startsWith("/refactoring/")
        ? records[0]
        : { status: "ready", blockers: [] },
  );
  const submit = vi.fn(async (envelope) => ({
    ...envelope,
    status: "applied",
    refactoring: { id, revision: 8 },
    sampled_at: "now",
  }));
  mountFeature(<RefactoringMaintenance />, api, testClient(), true, { submit });
  return { submit, api };
}
it("defaults to cautious and one round, submits the original project identity", async () => {
  const { submit } = setup();
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "开始维护" })).not.toBeDisabled(),
  );
  expect(screen.getByLabelText("决策策略")).toHaveValue("cautious");
  expect(screen.getByLabelText("重构轮次")).toHaveValue(1);
  fireEvent.click(screen.getByRole("button", { name: "开始维护" }));
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0]).toMatchObject({
    action: "refactoring_start",
    target_id: identity.repository_identity,
    expected_version: 1,
    payload: { strategy: "cautious", profile: "local", budget: { rounds: 1 } },
  });
});
it("shows coverage exclusions and answers the original question without another chat", async () => {
  const { submit } = setup([record]);
  await screen.findByText(record.question!.text);
  expect(screen.getByText(/不覆盖子 Agent/)).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "开始维护" }),
  ).not.toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("你的决定"), {
    target: { value: "保留公共接口" },
  });
  fireEvent.click(screen.getByRole("button", { name: "答复并继续" }));
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0]).toMatchObject({
    action: "refactoring_answer",
    target_id: id,
    expected_version: 7,
    payload: { question_id: "question", answer: "保留公共接口" },
  });
});
it("links maintenance activity and receipts to the same action", () => {
  expect(
    activityDestination("p", {
      details_ref: { kind: "refactoring", target: id },
    } as Activity),
  ).toBe(`/projects/p/refactoring?action=${id}`);
  const api = new Api(identity, "token", "home");
  const envelope = api.envelope(
    "refactoring_start",
    identity.repository_identity,
    1,
    {},
  );
  expect(requestDestination(envelope)).toBe(
    `/projects/project/refactoring?action=${envelope.request_id}`,
  );
});
