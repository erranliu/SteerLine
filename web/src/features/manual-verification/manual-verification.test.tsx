import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, testClient } from "../../test/workspace";
import ManualVerification from "./manual-verification";

afterEach(cleanup);

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
  vi.spyOn(api, "get").mockImplementation(async () => [current] as never);
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
  vi.spyOn(api, "get").mockImplementation(async () => [current] as never);
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
