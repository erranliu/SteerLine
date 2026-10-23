import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, testClient } from "../../test/workspace";
import { Tests } from "./tests";
import type { RecordPage, RecordRow } from "./types";

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
  level: "L1",
  environment: "isolated",
  status: "failed",
  note: "原始断言失败",
  at: "2026-10-23T00:00:00Z",
  commit: "1".repeat(40),
  path: "SteerlineFlow/testing/records/" + "1".repeat(40) + ".json",
};
const passed: RecordRow = {
  ...failed,
  id: "b".repeat(32),
  at: "2026-10-23T01:00:00Z",
  status: "passed",
  note: undefined,
};
const page: RecordPage = {
  records: [passed, failed],
  cursor: "",
  commit: "2".repeat(40),
  path: "SteerlineFlow/testing/records",
  observed_at: failed.at,
  summary: { total: 2, failed: 1, latest: passed, last_passed: passed },
};
function setup(online = true, bad = false) {
  const api = new Api(identity, "csrf", "home");
  const get = vi.spyOn(api, "get").mockImplementation(async (path) => {
    if (path === "/tests/catalogue")
      return {
        tests: [definition],
        commit: page.commit,
        path: "test-list.json",
        observed_at: failed.at,
      };
    if (path.startsWith("/tests/records?")) {
      if (bad) throw new Error("invalid Git JSON");
      return page;
    }
    return failed;
  });
  const post = vi.spyOn(api, "submit");
  mountFeature(<Tests />, api, testClient(), online);
  return { get, post };
}
it("TR displays original Git failure and subsequent pass without retest or evidence writes", async () => {
  const { get, post } = setup();
  await screen.findByRole("button", { name: /失败.*原始断言失败/ });
  expect(screen.getByRole("button", { name: /通过/ })).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: /失败.*原始断言失败/ }));
  expect(await screen.findByText("受测 SHA：" + failed.commit)).toBeVisible();
  expect(screen.getByText("Git 文件：" + failed.path)).toBeVisible();
  expect(screen.queryByText(/复验通过/)).toBeNull();
  expect(
    screen.queryByRole("button", { name: /执行测试|重跑|查看证据/ }),
  ).toBeNull();
  expect(get.mock.calls.some(([path]) => path.endsWith("/evidence"))).toBe(
    false,
  );
  expect(post).not.toHaveBeenCalled();
});
it("TR filters Git history and shows the current catalogue definition", async () => {
  const { get, post } = setup();
  await screen.findByRole("button", { name: /失败.*原始断言失败/ });
  fireEvent.change(screen.getByLabelText("原始结果"), {
    target: { value: "failed" },
  });
  fireEvent.change(screen.getByLabelText("受测 SHA"), {
    target: { value: "abc" },
  });
  await waitFor(() =>
    expect(
      get.mock.calls.some(
        ([path]) =>
          path.includes("outcome=failed") && path.includes("source=abc"),
      ),
    ).toBe(true),
  );
  fireEvent.click(screen.getByRole("button", { name: "测试定义" }));
  expect(screen.getByText("test rules")).toBeVisible();
  expect(post).not.toHaveBeenCalled();
});
it("TR keeps bad Git JSON distinct from an empty history", async () => {
  setup(true, true);
  expect(
    await screen.findByText(/记录查询失败.*invalid Git JSON/),
  ).toBeVisible();
  expect(screen.queryByText("尚无执行记录")).toBeNull();
});
it("TR stops queries offline and when hidden", () => {
  const offline = setup(false);
  expect(screen.getByText(/离线 · 以下为最后观察/)).toBeVisible();
  expect(offline.get).not.toHaveBeenCalled();
  cleanup();
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
  expect(setup().get).not.toHaveBeenCalled();
});
