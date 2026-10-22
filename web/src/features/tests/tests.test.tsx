import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, testClient } from "../../test/workspace";
import { Tests } from "./tests";
import type { RecordDetail, RecordPage, RecordRow } from "./types";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
const definition = {
  id: "rules",
  name: "规则测试",
  level: "L1",
  environment: "isolated",
  command: "test rules",
  check: "实际断言",
  current: true,
};
const failed: RecordRow = {
  id: "a".repeat(32),
  test_id: "rules",
  name: "规则测试",
  level: "L1",
  environment: "isolated",
  status: "failed",
  message: "原始断言失败",
  retest: "passed",
  retest_source_fixed: false,
  started_at: "2026-10-21T00:00:00Z",
  finished_at: "2026-10-21T00:00:01Z",
  branch: "main",
  commit: "1".repeat(40),
  source_fixed: true,
  partial: false,
  evidence_status: "available",
  source: { kind: "script" },
};
const page: RecordPage = {
  records: [failed],
  cursor: "",
  upper: 1,
  observed_at: failed.started_at,
  summary: { total: 1, pending: 0, latest: failed },
};
const detail: RecordDetail = {
  ...failed,
  chain: [failed],
  report: {
    definition,
    command: "actual rules",
    repository: "repo",
    before_commit: "1".repeat(40),
    after_commit: "1".repeat(40),
    before_clean: true,
    after_clean: true,
    definitions: { rules: "digest" },
    results: [],
  },
};
function setup(online = true, source = detail.source) {
  const api = new Api(identity, "csrf", "home");
  const get = vi.spyOn(api, "get").mockImplementation(async (path) => {
    if (path === "/tests/catalogue")
      return {
        tests: [definition],
        commit: "commit",
        path: "test-list.json",
        observed_at: failed.started_at,
      };
    if (path.includes("/evidence"))
      return {
        id: failed.id,
        status: "unavailable",
        diagnostic: "文件缺失",
        path: "summary.json",
        sha256: "sha",
        text: "",
      };
    if (path.startsWith("/tests/records?")) return page;
    return { ...detail, source };
  });
  const post = vi.spyOn(api, "submit");
  mountFeature(<Tests />, api, testClient(), online);
  return { api, get, post };
}
it("TR preserves the original failure, marks unfixed retest source, and reads evidence on demand", async () => {
  const { get, post } = setup();
  await screen.findByRole("button", { name: /失败.*复验通过/ });
  expect(screen.getByText(/复验源码未固定/)).toBeVisible();
  expect(get.mock.calls.some(([path]) => path.endsWith("/evidence"))).toBe(
    false,
  );
  fireEvent.click(screen.getByRole("button", { name: /失败.*复验通过/ }));
  await screen.findByText("原始断言失败");
  fireEvent.click(screen.getByRole("button", { name: "查看证据摘要" }));
  expect(await screen.findByText(/证据不可用：文件缺失/)).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "刷新记录" }));
  await waitFor(() =>
    expect(
      get.mock.calls.filter(([path]) => path.endsWith("/evidence")).length,
    ).toBe(2),
  );
  expect(screen.queryByRole("link", { name: "查看来源候选" })).toBeNull();
  expect(
    screen.queryByRole("button", { name: /执行测试|运行测试|重跑/ }),
  ).toBeNull();
  expect(post).not.toHaveBeenCalled();
});
it("TR filters through the API and switches to the frozen definition without execution", async () => {
  const { get, post } = setup();
  await screen.findByRole("button", { name: /失败.*复验通过/ });
  fireEvent.change(screen.getByLabelText("原始结果"), {
    target: { value: "failed" },
  });
  fireEvent.change(screen.getByLabelText("分支 / SHA"), {
    target: { value: "main" },
  });
  await waitFor(() =>
    expect(
      get.mock.calls.some(
        ([path]) =>
          path.includes("outcome=failed") && path.includes("source=main"),
      ),
    ).toBe(true),
  );
  fireEvent.click(screen.getByRole("button", { name: "测试定义" }));
  expect(screen.getByText("test rules")).toBeVisible();
  expect(post).not.toHaveBeenCalled();
});
it("TR does not query while offline or initially hidden", async () => {
  const offline = setup(false);
  expect(screen.getByText(/离线 · 以下为最后观察/)).toBeVisible();
  expect(offline.get).not.toHaveBeenCalled();
  cleanup();
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
  expect(setup().get).not.toHaveBeenCalled();
});

it("TR links only verified Runner origin to the product candidate", async () => {
  setup(true, {
    kind: "runner",
    task_id: "task",
    candidate_id: "candidate",
    run_id: "runner-run",
  });
  fireEvent.click(
    await screen.findByRole("button", { name: /失败.*复验通过/ }),
  );
  expect(
    await screen.findByRole("link", { name: "查看来源候选" }),
  ).toHaveAttribute("href", "/projects/project/runs?task=task&run=candidate");
});
