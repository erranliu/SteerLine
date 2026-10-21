import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import {
  identity,
  mountFeature,
  testClient,
  tasks,
} from "../../test/workspace";
import { ProjectMaintenance } from "./project-maintenance";
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
const view = {
  local_commit: "old",
  observation: {
    commit: "latest",
    checked_at: "2026-10-21T01:00:00Z",
    rewritten: false,
  },
  assessment: {
    items: [
      {
        plan: "docs/task.plan.md",
        target: "docs/task.tests.json",
        status: "needs_review",
        tests: ["rules"],
        review_digest: "a".repeat(64),
        approval_revision: 2,
      },
    ],
  },
  generations: [
    { id: "b".repeat(32), plan: "docs/task.plan.md", state: "generated" },
  ],
};
function setup(online = true, settled = true, applied = false) {
  const api = new Api(identity, "csrf", "home");
  const get = vi.spyOn(api, "get").mockImplementation(async (route) =>
    route.startsWith("/maintenance")
      ? view
      : route === "/presentation"
        ? {
            presentation: {
              profiles: [{ id: "local", name: "Local" }],
              default_profile_id: "local",
            },
          }
        : route.startsWith("/tasks")
          ? tasks
          : {
              id: route.split("/").pop() || "",
              revision: 3,
              state: applied ? "applied" : "generated",
              applied,
              stop_status: settled ? "stopped" : "active",
              review: applied
                ? undefined
                : {
                    review_digest: (route.endsWith("b".repeat(32))
                      ? "c"
                      : "d"
                    ).repeat(64),
                    before: "old-file",
                    after: "proposed-file",
                  },
            },
  );
  const submit = vi.spyOn(api, "submit").mockImplementation(async (e) => ({
    ...e,
    original: e,
    status: "applied",
    sampled_at: "now",
  }));
  mountFeature(<ProjectMaintenance />, api, testClient(), online);
  return {
    api,
    get,
    submit,
    settle: () => {
      settled = true;
    },
  };
}
it("PM keeps generation and file application explicit and binds review conditions", async () => {
  const { submit } = setup();
  await screen.findByText(/项目维护/);
  fireEvent.click(screen.getByRole("button", { name: /项目维护/ }));
  expect(
    await screen.findByText(/本地与远端基线不同 · 本地 HEAD/),
  ).toBeVisible();
  fireEvent.click(await screen.findByRole("button", { name: "选择此 Plan" }));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "生成测试安排" })).toBeEnabled(),
  );
  expect(submit).not.toHaveBeenCalled();
  await screen.findByRole("option", { name: /bbbbbbbb/ });
  fireEvent.change(screen.getByRole("combobox", { name: "已有生成记录" }), {
    target: { value: "b".repeat(32) },
  });
  expect(await screen.findByText("proposed-file")).toBeVisible();
  expect(submit).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "应用已审阅内容" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "maintenance_apply",
        target_id: "b".repeat(32),
        expected_version: 3,
        payload: expect.objectContaining({
          review_digest: "c".repeat(64),
          expected_revision: 3,
        }),
      }),
    ),
  );
});
it("PM scopes read caches and issues no reads or commands offline", () => {
  const { get, submit } = setup(false);
  fireEvent.click(screen.getByRole("button", { name: /项目维护/ }));
  expect(get).not.toHaveBeenCalled();
  expect(submit).not.toHaveBeenCalled();
  expect(screen.getByRole("button", { name: "检查远端" })).toBeDisabled();
});

it("PM separates verified output application from resource settlement", async () => {
  const { submit, settle } = setup(true, false);
  fireEvent.click(screen.getByRole("button", { name: /项目维护/ }));
  await screen.findByRole("option", { name: /bbbbbbbb/ });
  fireEvent.change(screen.getByRole("combobox", { name: "已有生成记录" }), {
    target: { value: "b".repeat(32) },
  });
  await screen.findByText("proposed-file");
  expect(screen.getByRole("button", { name: "应用已审阅内容" })).toBeEnabled();
  expect(submit).not.toHaveBeenCalled();
  settle();
  fireEvent.click(screen.getByRole("button", { name: "重新读取生成预览" }));
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "应用已审阅内容" }),
    ).toBeEnabled(),
  );
  expect(submit).not.toHaveBeenCalled();
});

it("PM empty arrangement confirmation belongs to the reviewed generation", async () => {
  const { submit } = setup();
  fireEvent.click(screen.getByRole("button", { name: /项目维护/ }));
  await screen.findByRole("option", { name: /bbbbbbbb/ });
  fireEvent.change(screen.getByRole("combobox", { name: "已有生成记录" }), {
    target: { value: "b".repeat(32) },
  });
  const confirmation = await screen.findByRole("checkbox", {
    name: /已确认空安排/,
  });
  fireEvent.click(confirmation);
  expect(confirmation).toBeChecked();
  fireEvent.change(screen.getByRole("textbox", { name: "原生成请求" }), {
    target: { value: "a".repeat(32) },
  });
  await waitFor(() =>
    expect(
      screen.getByRole("checkbox", { name: /已确认空安排/ }),
    ).not.toBeChecked(),
  );
  fireEvent.click(screen.getByRole("button", { name: "应用已审阅内容" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        target_id: "a".repeat(32),
        payload: expect.objectContaining({ allow_empty: false }),
      }),
    ),
  );
});

it("PM applied output keeps unsettled resource visible and stoppable", async () => {
  const { submit } = setup(true, false, true);
  fireEvent.click(screen.getByRole("button", { name: /项目维护/ }));
  await screen.findByRole("option", { name: /bbbbbbbb/ });
  fireEvent.change(screen.getByRole("combobox", { name: "已有生成记录" }), {
    target: { value: "b".repeat(32) },
  });
  expect(await screen.findByText(/原生成资源尚未核实停止/)).toBeVisible();
  const stop = screen.getByRole("button", { name: "取消生成/禁止后续应用" });
  expect(stop).toBeEnabled();
  fireEvent.click(stop);
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "maintenance_stop_generation",
        target_id: "b".repeat(32),
      }),
    ),
  );
});
