import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, testClient } from "../../test/workspace";
import ManualVerification from "./manual-verification";

afterEach(cleanup);

it("lists current-project launches with no PRs and gates start on readiness", async () => {
  const head = "a".repeat(40);
  const attempt = "b".repeat(32);
  const api = new Api(identity, "csrf");
  const entries = [
    {
      id: "playmode",
      name: "进入 Play Mode",
      launch: { program: "node", args: ["play.js"] },
    },
    {
      id: "editor",
      name: "打开 Unity 编辑器",
      launch: { program: "node", args: ["editor.js"] },
    },
  ];
  let view: Record<string, unknown> = {
    path: "C:/project",
    branch: "main",
    head,
    ready: false,
    diagnostic: "项目未就绪",
    entries,
  };
  vi.spyOn(api, "get").mockImplementation(
    async (path) =>
      (path.endsWith("/project")
        ? view
        : path.endsWith("/pulls/run")
          ? null
          : []) as never,
  );
  const submit = vi
    .spyOn(api, "submit")
    .mockImplementation(async (envelope) => {
      if (envelope.action === "project_launch_start") {
        view = {
          ...view,
          run: { id: attempt, head, status: "running", name: "进入 Play Mode" },
        };
      }
      return { status: "applied" } as never;
    });
  const client = testClient();
  mountFeature(<ManualVerification />, api, client);
  await screen.findByText("暂无待核验 PR。");
  expect(screen.getByRole("button", { name: "从 PR 启动" })).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "启动当前项目" }));
  const play = await screen.findByRole("button", { name: "进入 Play Mode" });
  expect(play).toBeDisabled();
  expect(
    screen.getByRole("button", { name: "打开 Unity 编辑器" }),
  ).toBeDisabled();
  view = { ...view, ready: true, diagnostic: undefined };
  await client.invalidateQueries({ queryKey: ["project-launch"] });
  await waitFor(() => expect(play).toBeEnabled());
  fireEvent.click(play);
  const stop = await screen.findByRole("button", { name: "停止项目启动" });
  expect(submit.mock.calls[0][0]).toMatchObject({
    action: "project_launch_start",
    target_id: identity.project_id,
    payload: { head, entry_id: "playmode" },
  });
  expect(play).toBeDisabled();
  fireEvent.click(stop);
  await waitFor(() =>
    expect(submit.mock.calls[1][0]).toMatchObject({
      action: "project_launch_stop",
      payload: { attempt_id: attempt },
    }),
  );
});

it("selects a recent merged PR with no open PRs and still looks up older PRs by number", async () => {
  const head = "a".repeat(40);
  const api = new Api(identity, "csrf");
  const pull = {
    number: 7,
    title: "Older PR",
    url: "https://github.com/example/project/pull/7",
    branch: "feature",
    head,
    state: "closed",
  };
  const recent = Array.from({ length: 11 }, (_, index) => ({
    ...pull,
    number: index + 8,
    title: "Recent " + index,
    state: index === 0 ? "merged" : "closed",
  }));
  vi.spyOn(api, "get").mockImplementation(async (route) => {
    switch (route) {
      case "/manual-verification/project":
        return { ready: false, diagnostic: "项目未就绪", entries: [] } as never;
      case "/manual-verification/pulls":
        return recent as never;
      case "/manual-verification/pulls/7":
      case "/manual-verification/pulls/8":
        return {
          pr: route.endsWith("/8") ? recent[0] : pull,
          entries: [
            {
              id: "preview",
              name: "启动 PR 预览",
              launch: { program: "node", args: [] },
            },
          ],
        } as never;
      case "/manual-verification/pulls/run":
        return {
          latest: {
            id: "b".repeat(32),
            head,
            pr_number: 12,
            name: "旧预览",
            status: "exited",
            worktree: "D:/old-worktree",
            removed: false,
            cleanup_diagnostic: "有本地修改",
          },
        } as never;
      default:
        return [] as never;
    }
  });
  const submit = vi
    .spyOn(api, "submit")
    .mockResolvedValue({ status: "applied" } as never);
  mountFeature(<ManualVerification />, api, testClient());
  fireEvent.click(screen.getByRole("button", { name: "从 PR 启动" }));
  await screen.findByRole("option", { name: "#17 Recent 9 · 已关闭" });
  expect(
    screen.queryByRole("option", { name: "#18 Recent 10 · 已关闭" }),
  ).toBeNull();
  fireEvent.change(screen.getByLabelText("最近更新的 PR"), {
    target: { value: "8" },
  });
  expect(
    await screen.findByRole("link", { name: "#8 Recent 0" }),
  ).toBeVisible();
  expect(screen.getByRole("button", { name: "启动 PR 预览" })).toBeEnabled();
  fireEvent.change(screen.getByLabelText("PR 编号"), {
    target: { value: "7" },
  });
  fireEvent.click(screen.getByRole("button", { name: "查找 PR" }));
  const start = await screen.findByRole("button", { name: "启动 PR 预览" });
  expect(start).toBeEnabled();
  expect(screen.getByRole("button", { name: "重试清理" })).toBeVisible();
  fireEvent.click(start);
  await waitFor(() =>
    expect(submit.mock.calls[0][0]).toMatchObject({
      action: "project_launch_pr_start",
      payload: { pr_number: 7, head, entry_id: "preview" },
    }),
  );
});

it("starts the selected PR Head and records only its current attempt", async () => {
  const head = "a".repeat(40);
  const next = "b".repeat(40);
  const candidate = "c".repeat(32);
  const attempt = {
    id: "d".repeat(32),
    head,
    source: "pr",
    status: "ready",
    started: true,
    worktree: "C:/preview/worktree",
    url: "http://127.0.0.1:40100",
  };
  const original = {
    candidate_id: candidate,
    task_id: "7",
    pr_url: "https://github.com/example/project/pull/21",
    head,
    revision: 1,
    draft: true,
    archived: false,
    configuration: {
      definition: {
        launch: { program: "node", args: ["preview.cjs"] },
        checks: [
          {
            id: "M01",
            title: "Preview",
            steps: ["Open it"],
            expected: "Renders",
          },
        ],
      },
    },
    attempts: [] as (typeof attempt)[],
    results: [] as Record<string, unknown>[],
  };
  let current = original;
  const api = new Api(identity, "csrf");
  vi.spyOn(api, "get").mockImplementation(
    async (path) =>
      (path === "/manual-verification" ? [current] : null) as never,
  );
  const submit = vi
    .spyOn(api, "submit")
    .mockImplementation(async (envelope) => {
      if (envelope.action === "manual_verification_start") {
        current = { ...current, attempts: [attempt], revision: 2 };
      } else {
        current = {
          ...current,
          head: next,
          revision: 3,
          results: [
            {
              ...envelope.payload,
              source: "pr",
              actor: "operator",
              at: "2026-11-13T00:00:00Z",
            },
          ],
        };
      }
      return { status: "applied" } as never;
    });
  mountFeature(<ManualVerification />, api, testClient());
  fireEvent.click(await screen.findByRole("button", { name: /PR #21/ }));
  fireEvent.click(screen.getByRole("button", { name: "创建 worktree 并启动" }));
  expect(await screen.findByRole("link", { name: "打开预览" })).toHaveAttribute(
    "href",
    attempt.url,
  );
  expect(submit.mock.calls[0][0]).toMatchObject({
    target_id: candidate,
    expected_version: 1,
    payload: { head },
  });
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "记录通过" })).toBeEnabled(),
  );
  fireEvent.click(screen.getByRole("button", { name: "记录通过" }));
  await screen.findByRole("button", { name: "启动最新版本" });
  expect(submit.mock.calls[1][0]).toMatchObject({
    expected_version: 2,
    payload: {
      head,
      attempt_id: attempt.id,
      check_id: "M01",
      outcome: "passed",
    },
  });
  expect(
    screen.getByRole("heading", { name: /Preview · 待核验/ }),
  ).toBeVisible();
  expect(screen.getByRole("button", { name: "记录通过" })).toBeDisabled();
});

it("closes a merged PR's selected remote snapshot after recording its own result", async () => {
  const head = "a".repeat(40);
  const candidate = "c".repeat(32);
  const attempt = {
    id: "d".repeat(32),
    head,
    source: "remote_head",
    status: "ready",
    started: true,
    worktree: "C:/preview/remote",
  };
  let current = {
    candidate_id: candidate,
    task_id: "7",
    pr_url: "https://github.com/example/project/pull/21",
    head,
    revision: 1,
    draft: false,
    merged: true,
    archived: false,
    closure: undefined as Record<string, unknown> | undefined,
    configuration: {
      definition: {
        launch: { program: "node", args: ["preview.cjs"] },
        checks: [
          {
            id: "M01",
            title: "Preview",
            steps: ["Open it"],
            expected: "Renders",
          },
        ],
      },
    },
    attempts: [] as (typeof attempt)[],
    results: [
      { head, source: "pr", check_id: "M01", outcome: "passed" },
    ] as Record<string, unknown>[],
  };
  const api = new Api(identity, "csrf");
  vi.spyOn(api, "get").mockImplementation(
    async (path) =>
      (path === "/manual-verification" ? [current] : null) as never,
  );
  const submit = vi
    .spyOn(api, "submit")
    .mockImplementation(async (envelope) => {
      current = { ...current, revision: current.revision + 1 };
      switch (envelope.action) {
        case "manual_verification_start":
          current.attempts = [attempt];
          break;
        case "manual_verification_result":
          current.results = [
            ...current.results,
            { ...envelope.payload, source: "remote_head" },
          ];
          break;
        case "manual_verification_close":
          current.closure = {
            ...envelope.payload,
            actor: "operator",
            at: "2026-11-13T00:00:00Z",
          };
          break;
        case "manual_verification_reopen":
          current.closure = undefined;
      }
      return { status: "applied" } as never;
    });
  mountFeature(<ManualVerification />, api, testClient());
  fireEvent.click(await screen.findByRole("button", { name: /PR #21/ }));
  fireEvent.change(await screen.findByRole("combobox", { name: "核验来源" }), {
    target: { value: "remote_head" },
  });
  fireEvent.click(
    screen.getByRole("button", { name: /在远端 Head 创建 worktree/ }),
  );
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "记录通过" })).toBeEnabled(),
  );
  const close = () =>
    screen.getByRole("button", { name: "标记核验完成并关闭" });
  expect(close()).toBeDisabled();
  fireEvent.change(screen.getByRole("textbox", { name: "核验备注" }), {
    target: { value: "Observed a failure" },
  });
  fireEvent.click(screen.getByRole("button", { name: "记录失败" }));
  await waitFor(() => expect(close()).toBeEnabled());
  fireEvent.click(close());
  await screen.findByText("暂无待核验 PR。");
  expect(submit.mock.calls.map(([envelope]) => envelope.payload)).toMatchObject(
    [
      { head, source: "remote_head" },
      { head, attempt_id: attempt.id, check_id: "M01", outcome: "failed" },
      { head, source: "remote_head" },
    ],
  );
  fireEvent.click(screen.getByRole("checkbox", { name: /显示已关闭/ }));
  expect(screen.getByRole("button", { name: /PR #21/ })).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "重新打开核验待办" }));
  await waitFor(() => expect(close()).toBeEnabled());
  expect(submit.mock.lastCall?.[0]).toMatchObject({
    action: "manual_verification_reopen",
    payload: { head },
  });
});
