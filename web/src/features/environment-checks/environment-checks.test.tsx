import { afterEach, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { projectQueries, invalidateCommand } from "../../shared/api/queries";
import {
  identity,
  mountFeature,
  testClient,
  tasks,
} from "../../test/workspace";
import { EnvironmentChecks } from "./environment-checks";
import type { CheckCatalog, CheckRecord } from "./types";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});
function catalog(): CheckCatalog {
  return {
    repository: "/fixture/repo",
    unavailable_reason: "",
    profiles: [
      {
        id: "local",
        name: "本机预填",
        execution: {
          adapter: "codex-app-server",
          target: { kind: "local" },
          settings: { executable: "fixture-codex", model: "fixture-model" },
        },
      },
    ],
    capabilities: {
      contract: "steerrunner-environment-check/v1",
      targets: [
        {
          adapter: "codex-app-server",
          mechanism: "implemented",
          target: "local",
          access_modes: ["sandbox", "full_access"],
        },
      ],
    },
  };
}
function record(): CheckRecord {
  return {
    id: "a".repeat(32),
    check_id: "check",
    created_at: "2026-10-16T01:00:00Z",
    runner: {
      executable: "fixture",
      version: "fixture-only",
      sha256: "a".repeat(64),
    },
    resource_digest: "b".repeat(64),
    input: {
      repository: {
        url: "fixture.invalid",
        base_branch: "main",
        base_revision: "a".repeat(40),
      },
      execution: {
        ...catalog().profiles[0].execution,
        settings: {
          ...catalog().profiles[0].execution.settings,
          access_mode: "sandbox",
        },
      },
    },
    operations: [
      {
        request_id: "a".repeat(32),
        action: "start_environment_check",
        status: "applied",
        runner_request: { input: {} },
      },
    ],
    report: {
      check_id: "check",
      revision: 1,
      input_digest: "c".repeat(64),
      updated_at: "2026-10-16T01:00:00Z",
      status: "completed",
      result: "failed",
      actual: { worker: "fixture identity" },
      cleanup_status: "pending",
      allowed_actions: ["stop"],
      diagnostic: "测试 PR 待收尾",
      items: [
        {
          id: "git_push",
          status: "failed",
          stage: "缺少现场凭据 401",
          required: true,
          evidence: [{ source: "fixture", facts: { diagnostic: "sanitized fixture evidence" } }],
        },
      ],
      resources: {},
    },
  };
}
function fixture(
  rows: CheckRecord[] = [],
  options = catalog(),
  online = true,
  send = vi.fn().mockResolvedValue(true),
) {
  const api = new Api(identity, "csrf", "home");
  const client = testClient();
  const get = vi
    .spyOn(api, "get")
    .mockImplementation(
      (path) =>
        Promise.resolve(
          path === "/environment-checks/options"
            ? options
            : path === "/environment-checks"
              ? { checks: rows }
              : { check: rows[0] },
        ) as never,
    );
  const mounted = mountFeature(<EnvironmentChecks />, api, client, online, {
    send,
  });
  return { api, client, get, send, ...mounted };
}
it("previews the exact authorization, defaults to sandbox, and guards repeated clicks", async () => {
  let finish!: (v: boolean) => void;
  const send = vi.fn().mockImplementation(
    () =>
      new Promise<boolean>((resolve) => {
        finish = resolve;
      }),
  );
  const f = fixture([], catalog(), true, send);
  const start = screen.getByRole("button", { name: "开始实际检查" });
  await waitFor(() => expect(start).toBeEnabled());
  expect(screen.getByLabelText("权限模式")).toHaveValue("sandbox");
  expect(screen.getByLabelText("执行预览")).toHaveTextContent("创建测试分支");
  fireEvent.change(screen.getByLabelText("权限模式"), {
    target: { value: "full_access" },
  });
  expect(screen.getByLabelText("执行预览")).toHaveTextContent(
    "直接使用 Worker 沙箱外权限",
  );
  fireEvent.click(start);
  fireEvent.click(start);
  expect(send).toHaveBeenCalledTimes(1);
  expect(send).toHaveBeenCalledWith(
    "start_environment_check",
    identity.project_id,
    0,
    expect.objectContaining({
      access_mode: "full_access",
      execution: expect.objectContaining({
        settings: expect.objectContaining({
          access_mode: "full_access",
          model: "fixture-model",
        }),
      }),
    }),
  );
  await act(async () => finish(true));
  f.unmount();
  expect(send).toHaveBeenCalledTimes(1);
  f.client.clear();
});
it("distinguishes unsupported mechanism from a supported check's credential failure", async () => {
  const f = fixture([record()]);
  await screen.findByText("缺少现场凭据 401");
  expect(screen.getByText(/能力结论/)).toHaveTextContent("失败");
  expect(screen.getByText(/资源：/)).toHaveTextContent("资源待处理");
  fireEvent.click(screen.getByText("查看证据"));
  expect(screen.getByText(/sanitized fixture evidence/)).toBeVisible();
  fireEvent.change(screen.getByLabelText("执行目标"), {
    target: { value: "orca_environment" },
  });
  expect(screen.getByText(/未声明支持此目标/)).toBeVisible();
  expect(screen.getByRole("button", { name: "开始实际检查" })).toBeDisabled();
  expect(screen.getByText("结果不适用于当前选择")).toBeVisible();
  f.client.clear();
});
it("keeps unknown unbound checks query-only and reads cached history while execution is offline", async () => {
  const row = record();
  row.check_id = "";
  row.report = undefined;
  row.operations[0].status = "outcome_unknown";
  const f = fixture([row], catalog(), false);
  await screen.findByText(/原操作结果未知/);
  expect(f.get).toHaveBeenCalled();
  expect(screen.getByRole("button", { name: "开始实际检查" })).toBeDisabled();
  expect(
    screen.getByRole("button", { name: "停止 / 收尾原检查" }),
  ).toBeDisabled();
  expect(screen.getByText(/服务离线，显示历史缓存/)).toBeVisible();
  f.unmount();
  expect(f.send).not.toHaveBeenCalled();
  f.client.clear();
});
it("stops using the original check and revision, and blocks another unresolved stop", async () => {
  const row = record();
  const f = fixture([row]);
  const stop = await screen.findByRole("button", { name: "停止 / 收尾原检查" });
  await waitFor(() => expect(stop).toBeEnabled());
  fireEvent.click(stop);
  await waitFor(() =>
    expect(f.send).toHaveBeenCalledWith(
      "stop_environment_check",
      row.id,
      1,
      {},
    ),
  );
  f.unmount();
  f.client.clear();
  row.operations.push({
    request_id: "b".repeat(32),
    action: "stop_environment_check",
    status: "outcome_unknown",
    runner_request: { input: { expected_revision: 1 } },
  });
  const second = fixture([row]);
  await screen.findByText(/原操作结果未知/);
  expect(
    screen.getByRole("button", { name: "停止 / 收尾原检查" }),
  ).toBeDisabled();
  second.client.clear();
});
it("preserves source time, polls only the selected visible check, and never cancels on unmount", async () => {
  vi.useFakeTimers();
  const visible = vi
    .spyOn(document, "visibilityState", "get")
    .mockReturnValue("visible");
  const f = fixture([record()]);
  await act(() => vi.advanceTimersByTimeAsync(50));
  const time = screen.getByText(/源更新时间/).textContent;
  await act(() => vi.advanceTimersByTimeAsync(3100));
  expect(screen.getByText(/源更新时间/).textContent).toBe(time);
  f.get.mockRejectedValue(new Error("Runner offline"));
  await act(() => vi.advanceTimersByTimeAsync(3100));
  expect(screen.getByText(/源查询失败/)).toBeVisible();
  expect(screen.getByText(/源更新时间/).textContent).toBe(time);
  visible.mockReturnValue("hidden");
  fireEvent(document, new Event("visibilitychange"));
  const calls = f.get.mock.calls.length;
  await act(() => vi.advanceTimersByTimeAsync(6200));
  expect(f.get).toHaveBeenCalledTimes(calls);
  f.unmount();
  await act(() => vi.advanceTimersByTimeAsync(6200));
  expect(f.get).toHaveBeenCalledTimes(calls);
  expect(f.send).not.toHaveBeenCalled();
  f.client.clear();
});
it("invalidates check queries without touching Task or DAG caches", async () => {
  const api = new Api(identity, "csrf", "home");
  const client = testClient();
  const q = projectQueries(api);
  client.setQueryData(q.checks().queryKey, { checks: [] });
  client.setQueryData(q.tasks().queryKey, { ...tasks, tasks: [] });
  await invalidateCommand(
    client,
    api.home,
    api.envelope("start_environment_check", identity.project_id, 0, {}),
  );
  expect(client.getQueryState(q.checks().queryKey)?.isInvalidated).toBe(true);
  expect(client.getQueryState(q.tasks().queryKey)?.isInvalidated).toBe(false);
  client.clear();
});

it("keeps history readable when Runner advertises no supported check targets", async () => {
  const options = catalog();
  options.capabilities!.targets = null;
  const f = fixture([record()], options);
  await screen.findByText("缺少现场凭据 401");
  expect(screen.getByText(/未声明支持此目标/)).toBeVisible();
  expect(screen.getByRole("button", { name: "开始实际检查" })).toBeDisabled();
  expect(f.send).not.toHaveBeenCalled();
  f.client.clear();
});

it("uses the latest selected detail when its cached history still says settled", async () => {
  const old = record();
  old.report!.cleanup_status = "completed";
  const f = fixture([old]);
  const current = record();
  current.report!.revision = 2;
  f.get.mockImplementation(
    (path) =>
      Promise.resolve(
        path === "/environment-checks/options"
          ? catalog()
          : path === "/environment-checks"
            ? { checks: [old] }
            : { check: current },
      ) as never,
  );
  await act(() =>
    f.client.refetchQueries({
      queryKey: projectQueries(f.api).checks().queryKey,
    }),
  );
  await waitFor(() =>
    expect(screen.getByText(/已有检查或资源未核实/)).toBeVisible(),
  );
  expect(screen.getByRole("button", { name: "开始实际检查" })).toBeDisabled();
  expect(f.send).not.toHaveBeenCalled();
  f.client.clear();
});
