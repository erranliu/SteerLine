import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, testClient } from "../../test/workspace";
import { Navigate } from "react-router-dom";
import type { EnvironmentOptions } from "./use-readiness";
import { ProjectEnvironment } from "./project-environment";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const optionsKey = [
  "home",
  identity.project_id,
  identity.instance_id,
  "environment",
  "options",
];

it("binds each card's invalidation to its own key and revision and shares only the material document", async () => {
  const { client, send } = fixture();
  await screen.findByText("Orca · Codex · 验收阻断");
  const options = structuredClone(
    client.getQueryData<EnvironmentOptions>(optionsKey)!,
  );
  options.environments.push({
    ...options.environments[0],
    environment_key: "f".repeat(64),
    current_revision: 7,
    profiles: [{ ...options.profiles[0], id: "second", name: "另一权限方案" }],
  });
  client.setQueryData(optionsKey, options);
  const card = (await screen.findByText(/共用方案：另一权限方案/)).closest(
    "article",
  )!;
  fireEvent.click(within(card).getByText("标记失效", { selector: "summary" }));
  fireEvent.change(within(card).getByLabelText("环境失效原因"), {
    target: { value: "账号变化" },
  });
  fireEvent.click(
    within(card).getByRole("button", { name: "标记当前项目环境失效" }),
  );
  expect(send).toHaveBeenCalledWith(
    "environment_invalidate",
    identity.project_id,
    1,
    {
      environment_key: "f".repeat(64),
      expected_current_revision: 7,
      reason: "账号变化",
    },
  );
  expect(screen.getAllByText("阅读项目准备清单")).toHaveLength(1);
});

it("marks cached ready as unverified after query failure, blocks a new operation and preserves original stop", async () => {
  const { client, api, send, record } = fixture();
  await screen.findByText("Orca · Codex · 验收阻断");
  const options = structuredClone(
    client.getQueryData<EnvironmentOptions>(optionsKey)!,
  );
  options.environments[0].stage = "ready";
  options.environments[0].status = "ready";
  client.setQueryData(optionsKey, options);
  vi.mocked(api.get).mockImplementation(async (path) => {
    if (path === "/environment/options") throw new Error("options unavailable");
    return (
      path === "/environment"
        ? { records: [{ ...record, rounds: [] }] }
        : record
    ) as never;
  });
  await client.refetchQueries({ queryKey: optionsKey });
  expect(await screen.findByText(/上次已知状态，当前待核实/)).toBeVisible();
  fireEvent.click(screen.getByText("重新验收", { selector: "summary" }));
  expect(screen.getByRole("button", { name: "提交验收" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "停止原活动" }));
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "environment_stop",
      record.id,
      record.revision,
      {},
    ),
  );
  expect(send).toHaveBeenCalledTimes(1);
});

it("keeps a stale unsettled activity visible and never starts a replacement", async () => {
  const { client, send } = fixture();
  await screen.findByText("Orca · Codex · 验收阻断");
  const options = structuredClone(
    client.getQueryData<EnvironmentOptions>(optionsKey)!,
  );
  Object.assign(options.environments[0], {
    stage: "stale",
    activity_settled: false,
  });
  client.setQueryData(optionsKey, options);
  fireEvent.click(await screen.findByText("重新验收", { selector: "summary" }));
  expect(screen.getByRole("button", { name: "提交验收" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "停止原活动" })).toBeEnabled();
  expect(send).not.toHaveBeenCalled();
});

it("requires a choice for a shared environment without a default and rejects an obsolete linked profile", async () => {
  const { client, send } = fixture(
    true,
    vi.fn(),
    false,
    "/projects/project/environment?profile=deleted",
  );
  await screen.findByText(/原执行方案已不存在/);
  const options = structuredClone(
    client.getQueryData<EnvironmentOptions>(optionsKey)!,
  );
  options.default_profile_id = "";
  options.environments[0].environment_key = "f".repeat(64);
  options.environments[0].profiles.push({
    ...options.profiles[0],
    id: "another",
    name: "另一模型",
  });
  client.setQueryData(optionsKey, options);
  fireEvent.click(await screen.findByText("重新验收", { selector: "summary" }));
  expect(screen.getByLabelText("验收执行方案")).toHaveValue("");
  expect(screen.getByRole("button", { name: "提交验收" })).toBeDisabled();
  fireEvent.change(screen.getByLabelText("验收执行方案"), {
    target: { value: "another" },
  });
  expect(screen.getByRole("button", { name: "提交验收" })).toBeEnabled();
  expect(send).not.toHaveBeenCalled();
});

it("locates the linked profile and returns to the original task without executing anything", async () => {
  const { send } = fixture(
    true,
    vi.fn(),
    false,
    "/projects/project/environment?profile=remote&task=38",
  );
  expect(await screen.findByLabelText("验收执行方案")).toHaveValue("remote");
  expect(screen.getByRole("link", { name: "返回任务" })).toHaveAttribute(
    "href",
    "/projects/project/runs?task=38&start_profile=remote",
  );
  await waitFor(() =>
    expect(document.activeElement?.id).toBe(`environment-${"e".repeat(64)}`),
  );
  expect(send).not.toHaveBeenCalled();
});
function fixture(
  online = true,
  send = vi.fn().mockResolvedValue(true),
  emptyHistory = false,
  path = "",
) {
  const api = new Api(identity, "csrf", "home");
  const round = {
    number: 1,
    deadline: "2026-10-26T10:00:00Z",
    run_id: "original-run",
    settled: false,
    timed_out: false,
    diagnostic: "等待原执行收尾",
    run: {
      summary: "缺少必需工具",
      diagnostic: "",
      agent_reply: "请补充远端凭据引用",
      agent_reply_verified: true,
    },
    progress: {
      version: 9,
      collection_status: "observing",
      content_base64: btoa('{"pr":true}'),
    },
    result: {
      version: 0,
      collection_status: "pending",
    },
  };
  const record = {
    id: "a".repeat(32),
    revision: 5,
    kind: "adaptation",
    profile_id: "remote",
    state: "running",
    status: "not_ready",
    settled: false,
    diagnostic: "",
    rounds: [round],
  };
  vi.spyOn(api, "get").mockImplementation(
    async (path) =>
      (path === "/environment/options"
        ? {
            profiles: [
              {
                id: "remote",
                name: "远端一",
                status: "not_ready",
                stage: "blocked",
                environment_key: "e".repeat(64),
                current_revision: 2,
                reason: "缺少工具",
              },
            ],
            environments: [
              {
                environment_key: "e".repeat(64),
                name: "远端一",
                target: { kind: "orca_environment", environment: "remote" },
                adapter: "orca-terminal",
                agent: "Codex",
                activity_settled: true,
                status: "not_ready",
                stage: "blocked",
                reason: "缺少工具",
                current_revision: 2,
                document_path: "SteerlineFlow/environment.md",
                activity_id: record.id,
                profiles: [{ id: "remote", name: "远端一" }],
              },
            ],
            generators: [{ id: "local", name: "本地生成" }],
          }
        : path === "/environment"
          ? { records: emptyHistory ? [] : [{ ...record, rounds: [] }] }
          : record) as never,
  );
  const client = testClient();
  mountFeature(
    <>
      {path && <Navigate to={path} />}
      <ProjectEnvironment />
    </>,
    api,
    client,
    online,
    { send },
  );
  return { send, record, api, client };
}
it("keeps progress separate from binary admission and stops the exact original activity", async () => {
  const { send, record } = fixture();
  expect(await screen.findByText("Orca · Codex · 验收阻断")).toBeVisible();
  fireEvent.click(await screen.findByRole("button", { name: "查看活动详情" }));
  expect(await screen.findByText(/Run original-run/)).toBeVisible();
  fireEvent.click(screen.getByText("原执行文件与技术详情"));
  expect(screen.getByText(/进度文件（不作为验收结论）/)).toBeVisible();
  expect(screen.getByText(/验收结果文件/)).toBeVisible();
  expect(screen.getByText(/停止后封存一次/)).toBeVisible();
  expect(screen.getByText("原执行说明：缺少必需工具")).toBeVisible();
  expect(
    screen.getByText(/Agent 回复（原生来源已核实，仅供排障）/),
  ).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "停止原活动" }));
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "environment_stop",
      record.id,
      record.revision,
      {},
    ),
  );
});
it("shows a final collection failure without promoting binary admission", async () => {
  const { record } = fixture();
  Object.assign(record.rounds[0].result, {
    version: 0,
    collection_status: "closed",
    diagnostic: "file_missing",
  });
  fireEvent.click(await screen.findByRole("button", { name: "查看活动详情" }));
  fireEvent.click(await screen.findByText("原执行文件与技术详情"));
  fireEvent.click(await screen.findByText(/验收结果文件/));
  expect(await screen.findByText("结果采集诊断：file_missing")).toBeVisible();
  expect(screen.getByText("Orca · Codex · 验收阻断")).toBeVisible();
});
it("freezes the selected profile and timeout and prevents duplicate clicks", async () => {
  let finish!: (v: boolean) => void;
  const send = vi.fn().mockImplementation(
    () =>
      new Promise<boolean>((resolve) => {
        finish = resolve;
      }),
  );
  fixture(true, send);
  await screen.findByText("Orca · Codex · 验收阻断");
  fireEvent.click(screen.getByText("重新验收", { selector: "summary" }));
  fireEvent.change(screen.getByLabelText("验收执行方案"), {
    target: { value: "remote" },
  });
  fireEvent.change(screen.getByLabelText("每轮总超时（秒）"), {
    target: { value: "120" },
  });
  const start = screen.getByRole("button", { name: "提交验收" });
  fireEvent.click(start);
  fireEvent.click(start);
  expect(send).toHaveBeenCalledTimes(1);
  expect(send).toHaveBeenCalledWith(
    "environment_start",
    identity.project_id,
    1,
    {
      profile_id: "remote",
      round_timeout_seconds: 120,
      expected_current_revision: 2,
    },
  );
  finish(true);
});
it("keeps history readable while offline and disables mutations", async () => {
  fixture(false);
  fireEvent.click(await screen.findByRole("button", { name: "查看活动详情" }));
  await screen.findByText(/Run original-run/);
  expect(
    screen.queryByRole("button", { name: "提交验收" }),
  ).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "生成文档和脚本" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "停止原活动" })).toBeDisabled();
});

it("shows the original intranet PR link without promoting an unfinished activity", async () => {
  const { record } = fixture();
  Object.assign(record.rounds[0], {
    report: {
      completed: true,
      checks: [
        { id: "git", status: "passed", message: "Git available" },
        {
          id: "unity.editor",
          status: "failed",
          message: "UNITY_EDITOR_PATH is not set",
        },
      ],
      message: "等待收尾",
      l2: [{ test_id: "one", status: "passed", message: "" }],
      pr: {
        url: "http://gitea.local/pulls/1",
        head_revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        cleanup_completed: true,
        outcome_unknown: false,
      },
    },
  });
  fireEvent.click(await screen.findByRole("button", { name: "查看活动详情" }));
  expect(
    await screen.findByRole("link", { name: "http://gitea.local/pulls/1" }),
  ).toHaveAttribute("href", "http://gitea.local/pulls/1");
  expect(screen.getByText("Orca · Codex · 验收阻断")).toBeVisible();
  expect(screen.getByText("git · 通过 · Git available")).toBeVisible();
  expect(
    screen.getByText("unity.editor · 不通过 · UNITY_EDITOR_PATH is not set"),
  ).toBeVisible();
});

it("exports the script under its declared path even when prose mentions another interpreter", async () => {
  const OriginalURL = URL;
  vi.stubGlobal(
    "URL",
    class extends OriginalURL {
      static createObjectURL() {
        return "blob:fixture";
      }
      static revokeObjectURL() {}
    },
  );
  const names: string[] = [];
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
    this: HTMLAnchorElement,
  ) {
    names.push(this.download);
  });
  const { record } = fixture();
  Object.assign(record, {
    kind: "generation",
    state: "generated",
    settled: true,
    profile_id: "local",
    generated: {
      document:
        'PowerShell verify-environment.ps1 is not selected.\n```steerline-environment\n{"version":1,"script":"scripts/verify-environment.sh","interpreter":"bash","validation_base":"validation","dependencies":[]}\n```',
      script: "#!/usr/bin/env bash\n",
      gaps: ["fixture only"],
      script_path: "scripts/verify-environment.sh",
    },
  });
  fireEvent.click(await screen.findByRole("button", { name: "查看活动详情" }));
  fireEvent.click(await screen.findByRole("button", { name: "导出验收脚本" }));
  expect(names).toEqual(["verify-environment.sh"]);
});

it("shows the selected current report even outside the recent history window", async () => {
  const { send, record } = fixture(true, vi.fn().mockResolvedValue(true), true);
  fireEvent.click(await screen.findByRole("button", { name: "查看当前验收" }));
  expect(await screen.findByText(/Run original-run/)).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "停止原活动" }));
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "environment_stop",
      record.id,
      record.revision,
      {},
    ),
  );
});

it("invalidates only the selected project context with its displayed current revision", async () => {
  const { send } = fixture();
  await screen.findByText("Orca · Codex · 验收阻断");
  fireEvent.click(screen.getByText("标记失效", { selector: "summary" }));
  fireEvent.change(screen.getByLabelText("环境失效原因"), {
    target: { value: "机器已重建" },
  });
  fireEvent.click(screen.getByRole("button", { name: "标记当前项目环境失效" }));
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "environment_invalidate",
      identity.project_id,
      1,
      {
        environment_key: "e".repeat(64),
        expected_current_revision: 2,
        reason: "机器已重建",
      },
    ),
  );
});
