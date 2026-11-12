import { afterEach, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { Link } from "react-router-dom";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, testClient } from "../../test/workspace";
import {
  ProjectMaintenance,
  type MaintenanceView,
} from "./project-maintenance";
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  sessionStorage.clear();
});
function setup(
  overrides: Partial<MaintenanceView> = {},
  online = true,
  destination = "",
) {
  const sha = "a".repeat(40);
  const view: MaintenanceView = {
    readiness: {
      status: "not_ready",
      commit: sha,
      total: 1,
      ready: 1,
      blockers: [
        {
          category: "worktree",
          code: "local_behind_remote",
          diagnostic: "本地尚未与目标分支同步",
        },
      ],
    },
    observation: { commit: sha, last_success: "2026-10-21T01:00:00Z" },
    remote_name: "origin/main",
    remote: {
      sha,
      title: "Update Plan",
      author: "Developer",
      time: "2026-10-21T00:00:00Z",
      url: "https://github.com/example/repo/commit/" + sha,
      prs: [],
    },
    local: {
      commit: "b".repeat(40),
      branch: "main",
      status: "behind",
      ahead: 0,
      behind: 2,
      can_update: true,
    },
    tasks: [
      {
        task_id: "101",
        contract: { plan_ref: { path: "SteerlineFlow/plans/task.plan.md" } },
        status: "ready",
      },
    ],
    ...overrides,
  };
  const api = new Api(identity, "csrf");
  const get = vi.spyOn(api, "get").mockImplementation(async (route) => {
    if (route.startsWith("/maintenance/log"))
      return { commits: [view.remote], more: false } as never;
    if (route.startsWith("/maintenance/commits/"))
      return {
        ...view.remote,
        prs: ["https://github.com/example/repo/pull/9"],
      } as never;
    return { ...view } as never;
  });
  const submit = vi
    .spyOn(api, "submit")
    .mockResolvedValue({ status: "applied" } as never);
  mountFeature(
    <>
      {destination && <Link to={destination}>任务维护入口</Link>}
      <ProjectMaintenance />
    </>,
    api,
    testClient(),
    online,
  );
  return { api, get, submit, view };
}

it("opens project maintenance when arriving through the task link without preparing automatically", async () => {
  const { submit } = setup(
    {},
    true,
    "/projects/" + identity.project_id + "/status?task=101",
  );
  fireEvent.click(screen.getByRole("link", { name: "任务维护入口" }));
  expect(
    await screen.findByRole("heading", { name: "Git 任务材料与测试安排" }),
  ).toBeVisible();
  expect(await screen.findByText("SteerlineFlow/plans/task.tests.json")).toBeVisible();
  expect(document.getElementById("project-status-task-101")).toBeVisible();
  expect(submit).not.toHaveBeenCalled();
});

it("PMS shows remote/local meaning and only the update workflow", async () => {
  const { get, submit } = setup();
  expect(await screen.findByText(/远端基线 · origin\/main/)).toBeVisible();
  expect(
    screen.getByRole("heading", { name: /本地基线.*落后 2/ }),
  ).toBeVisible();
  expect(screen.getByText(/Update Plan/)).toBeVisible();
  expect(screen.queryByLabelText("维护范围")).toBeNull();
  expect(screen.queryByRole("button", { name: "应用已审阅内容" })).toBeNull();
  expect(submit).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "更新本地" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "maintenance_update",
        target_id: identity.project_id,
      }),
    ),
  );
  expect(get).toHaveBeenCalledWith("/maintenance");
});
it("PMS displays why update needs manual handling and disables unsafe update", async () => {
  setup({
    readiness: {
      status: "not_ready",
      commit: "base",
      total: 1,
      ready: 1,
      blockers: [
        {
          category: "worktree",
          code: "wrong_branch",
          diagnostic: "当前分支与目标不同，请人工处理",
        },
        {
          category: "test_plan",
          code: "test_plan_missing",
          task_id: "101",
          path: "missing.tests.json",
          diagnostic: "缺少已提交的测试安排",
        },
        {
          category: "task_material",
          code: "task_material_unprepared",
          task_id: "102",
          diagnostic: "任务 102 材料未准备",
        },
      ],
    },
    local: {
      commit: "b".repeat(40),
      branch: "feature",
      status: "diverged",
      ahead: 3,
      behind: 2,
      can_update: false,
      reason: "本地存在独有提交，请人工处理",
    },
  });
  expect(
    await screen.findByRole("heading", { name: /本地基线.*已分叉 · 领先 3/ }),
  ).toBeVisible();
  expect(screen.getByText(/请人工处理/)).toBeVisible();
  expect(screen.getByText("缺少已提交的测试安排")).toBeVisible();
  expect(screen.getByText("任务 102 材料未准备")).toBeVisible();
  expect(
    screen.getByRole("link", { name: "查看任务 #101 材料" }),
  ).toHaveAttribute("href", "#project-status-task-101");
  expect(screen.getByRole("button", { name: "更新本地" })).toBeDisabled();
});
it("PMS lets users browse log and real associated PR without mutation", async () => {
  const { submit } = setup();
  fireEvent.click(await screen.findByRole("button", { name: "查看提交日志" }));
  await screen.findByText(/Update Plan/);
  await screen.findByRole("region", { name: "提交日志" });
  fireEvent.click(
    screen.getAllByRole("button", { name: "提交详情 / 关联 PR" })[0],
  );
  expect(
    await screen.findByRole("link", {
      name: "https://github.com/example/repo/pull/9",
    }),
  ).toBeVisible();
  expect(submit).not.toHaveBeenCalled();
});
it("PMS offline view retains facts and does not submit or check automatically", async () => {
  const { get, submit } = setup({}, false);
  expect(screen.getByRole("button", { name: "检查更新" })).toBeDisabled();
  expect(screen.getByText(/离线/)).toBeVisible();
  expect(get).not.toHaveBeenCalled();
  expect(submit).not.toHaveBeenCalled();
});

it("keeps a binary project status and follows the server verdict instead of individual rows", async () => {
  const { view, api } = setup({
    readiness: {
      status: "ready",
      commit: "base",
      total: 2,
      ready: 1,
      waiting: 1,
      blockers: [],
    },
  });
  expect(
    await screen.findByText(/测试安排 1\/2 就绪.*1 项等待前置测试/),
  ).toBeVisible();
  expect(screen.queryByRole("heading", { name: "未就绪原因" })).toBeNull();
  expect(api.submit).not.toHaveBeenCalled();
  view.readiness = {
    ...view.readiness,
    status: "not_ready",
    blockers: [
      {
        category: "task_material",
        code: "task_material_unprepared",
        diagnostic: "任务 #99 的材料与测试安排未就绪",
      },
    ],
  };
  fireEvent.click(screen.getByRole("button", { name: "检查更新" }));
  expect(
    await screen.findByRole("heading", { name: "未就绪原因" }),
  ).toBeVisible();
  expect(screen.getByText("任务 #99 的材料与测试安排未就绪")).toBeVisible();
});

it.each(["unknown test ID", "pinned path missing"])(
  "shows committed arrangement errors (%s) without generation controls",
  async (diagnostic) => {
    const { submit } = setup({
      tasks: [
        {
          task_id: "101",
          contract: { plan_ref: { path: "SteerlineFlow/plans/task.plan.md" } },
          status: "invalid",
          diagnostic: "task.tests.json: " + diagnostic,
        },
      ],
    });
    expect(
      await screen.findByText("SteerlineFlow/plans/task.tests.json"),
    ).toBeVisible();
    expect(screen.getByRole("alert")).toHaveTextContent(diagnostic);
    expect(
      screen.queryByRole("button", { name: "准备材料与测试安排" }),
    ).toBeNull();
    expect(screen.queryByRole("button", { name: "重试生成" })).toBeNull();
    expect(screen.queryByLabelText("测试生成方案")).toBeNull();
    expect(submit).not.toHaveBeenCalled();
  },
);

it("retains trusted source check time when the latest remote check failed", async () => {
  const { submit } = setup({
    observation: {
      commit: "fixed",
      last_success: "2026-11-10T01:02:03Z",
      diagnostic: "network unavailable",
    },
  });
  expect(await screen.findByText(/最近成功检查/)).toHaveAttribute(
    "datetime",
    "2026-11-10T01:02:03Z",
  );
  expect(submit).not.toHaveBeenCalled();
});
it("does not invent a check time when the source has none", async () => {
  setup({
    observation: { commit: "fixed", diagnostic: "network unavailable" },
  });
  expect(await screen.findByText("上次成功检查时间未知")).toBeVisible();
});
