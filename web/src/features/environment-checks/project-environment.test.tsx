import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  act,
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
const remoteMachineID = "e68f4edf-8a83-42c2-a9da-6d404d3aa3d8";
const currentCard = () => screen.getByRole("region", { name: "当前运行环境" });
async function loaded() {
  await screen.findByRole("heading", { name: "检测阻断" });
}
async function updateOptions(
  client: ReturnType<typeof testClient>,
  update: (options: EnvironmentOptions) => void,
) {
  const options = structuredClone(
    client.getQueryData<EnvironmentOptions>(optionsKey)!,
  );
  update(options);
  await act(async () => {
    client.setQueryData(optionsKey, options);
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}
async function openHistory() {
  fireEvent.click(screen.getByText("检测历史", { selector: "summary" }));
  await screen.findByLabelText("历史范围");
}

it("shows one readable environment and status, with only the detection ID for debugging", async () => {
  const { api, record, send } = fixture();
  await loaded();
  expect(
    await screen.findByRole("option", { name: "远程工作站 · Orca → Codex" }),
  ).toBeInTheDocument();
  expect(screen.getAllByText("缺少工具")).toHaveLength(1);
  expect(screen.getAllByText(record.id)).toHaveLength(1);
  expect(screen.queryByLabelText("检测所用方案")).not.toBeInTheDocument();
  expect(screen.getByText("更多设置").closest("details")).not.toHaveAttribute(
    "open",
  );
  expect(screen.getByText("检测历史").closest("details")).not.toHaveAttribute(
    "open",
  );
  expect(
    screen.getByText("查看准备要求").closest("details"),
  ).not.toHaveAttribute("open");
  expect(api.get).not.toHaveBeenCalledWith(
    expect.stringContaining("/environment/materials"),
    expect.anything(),
  );
  expect(api.get).not.toHaveBeenCalledWith(
    expect.stringMatching(/^\/environment\?/),
    expect.anything(),
  );
  for (const text of [
    remoteMachineID,
    "e".repeat(64),
    "original-run",
    "SteerlineFlow/environment.md",
    "原执行文件与技术详情",
    "Agent 回复",
    "共用方案",
    "下一步：",
  ]) {
    expect(screen.queryByText(text, { exact: false })).not.toBeInTheDocument();
  }
  expect(send).not.toHaveBeenCalled();
});

it("does not expose the remote ID while the machine directory is pending", async () => {
  fixture();
  vi.mocked(fetch).mockImplementation(() => new Promise(() => {}));
  await loaded();
  expect(
    screen.getByRole("option", { name: "远端机器 · Orca → Codex" }),
  ).toBeInTheDocument();
  expect(
    screen.queryByText(remoteMachineID, { exact: false }),
  ).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "重新检测" })).toBeEnabled();
});

it("distinguishes configurations on the same machine and link using readable scheme names", async () => {
  const { client } = fixture();
  await loaded();
  await updateOptions(client, (options) =>
    options.environments.push({
      ...options.environments[0],
      environment_key: "f".repeat(64),
      profiles: [
        { ...options.profiles[0], id: "other-account", name: "测试账号" },
      ],
    }),
  );
  expect(
    screen.getByRole("option", { name: "远程工作站 · Orca → Codex · 远端一" }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("option", {
      name: "远程工作站 · Orca → Codex · 测试账号",
    }),
  ).toBeInTheDocument();
  expect(
    screen.queryByText("f".repeat(64), { exact: false }),
  ).not.toBeInTheDocument();
});

it("copies the full detection ID and supports manual copying when clipboard access fails", async () => {
  const { record } = fixture();
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  await loaded();
  fireEvent.click(within(currentCard()).getByRole("button", { name: "复制" }));
  expect(await screen.findByText("已复制")).toBeVisible();
  expect(writeText).toHaveBeenCalledWith(record.id);
  writeText.mockRejectedValue(new Error("denied"));
  fireEvent.click(within(currentCard()).getByRole("button", { name: "复制" }));
  expect(await screen.findByText("复制失败，请手动复制检测 ID")).toBeVisible();
  expect(screen.getByText(record.id)).toBeVisible();
});

it("loads preparation requirements only when opened and excludes protocol metadata", async () => {
  const { api } = fixture();
  await loaded();
  const original = vi.mocked(api.get).getMockImplementation()!;
  vi.mocked(api.get).mockImplementation(async (path, signal) =>
    path.startsWith("/environment/materials")
      ? ({
          document:
            '准备 Git 和认证。\n```steerline-environment\n{"environment_key":"hidden-key"}\n```\n完成后开始检测。',
          document_path: "hidden-path",
          source_commit: "hidden-sha",
        } as never)
      : original(path, signal),
  );
  fireEvent.click(screen.getByText("查看准备要求", { selector: "summary" }));
  expect(await screen.findByText(/准备 Git 和认证/)).toBeVisible();
  for (const text of ["hidden-key", "hidden-path", "hidden-sha"])
    expect(screen.queryByText(text, { exact: false })).not.toBeInTheDocument();
});

it("preserves the selected timeout and profile and prevents duplicate detection submissions", async () => {
  let finish!: (value: boolean) => void;
  const send = vi.fn(
    () =>
      new Promise<boolean>((resolve) => {
        finish = resolve;
      }),
  );
  fixture(true, send);
  await loaded();
  fireEvent.click(screen.getByText("更多设置", { selector: "summary" }));
  fireEvent.change(screen.getByLabelText("本次检测总超时（秒）"), {
    target: { value: "120" },
  });
  const start = screen.getByRole("button", { name: "重新检测" });
  fireEvent.click(start);
  fireEvent.click(start);
  expect(send).toHaveBeenCalledTimes(1);
  expect(send).toHaveBeenCalledWith(
    "environment_start",
    identity.project_id,
    1,
    {
      environment_key: "e".repeat(64),
      profile_id: "remote",
      round_timeout_seconds: 120,
      expected_current_revision: 2,
    },
  );
  finish(true);
});

it("requires a scheme selection only for shared environments without a default", async () => {
  const { client, send } = fixture();
  await loaded();
  await updateOptions(client, (options) => {
    options.default_profile_id = "";
    options.environments[0].environment_key = "f".repeat(64);
    options.environments[0].profiles.push({
      ...options.profiles[0],
      id: "another",
      name: "另一方案",
    });
  });
  fireEvent.change(screen.getByLabelText("运行环境"), {
    target: { value: "f".repeat(64) },
  });
  expect(await screen.findByLabelText("检测所用方案")).toHaveValue("");
  expect(screen.getByRole("button", { name: "重新检测" })).toBeDisabled();
  fireEvent.change(screen.getByLabelText("检测所用方案"), {
    target: { value: "another" },
  });
  expect(screen.getByRole("button", { name: "重新检测" })).toBeEnabled();
  expect(send).not.toHaveBeenCalled();
});

it("stops the exact current activity even outside the history window and never starts a replacement", async () => {
  const { client, record, send } = fixture(
    true,
    vi.fn().mockResolvedValue(true),
    true,
  );
  record.settled = false;
  await loaded();
  await updateOptions(client, (options) => {
    options.environments[0].activity_settled = false;
    options.environments[0].stage = "stale";
  });
  const stop = await screen.findByRole("button", { name: "停止检测" });
  await waitFor(() => expect(stop).toBeEnabled());
  expect(
    screen.queryByRole("button", { name: "重新检测" }),
  ).not.toBeInTheDocument();
  fireEvent.click(stop);
  expect(send).toHaveBeenCalledWith(
    "environment_stop",
    record.id,
    record.revision,
    {},
  );
  expect(send).toHaveBeenCalledTimes(1);
});

it("keeps stop available after qualification observation fails while disabling new operations", async () => {
  const { api, client, record, send } = fixture();
  record.settled = false;
  await loaded();
  await updateOptions(client, (options) => {
    options.environments[0].activity_settled = false;
  });
  const read = vi.mocked(api.get).getMockImplementation()!;
  vi.mocked(api.get).mockImplementation(async (path, signal) => {
    if (path === "/environment/options")
      throw new Error("raw ID and diagnostics");
    return read(path, signal);
  });
  await client.refetchQueries({ queryKey: optionsKey });
  expect(await screen.findByText(/上次已知状态，当前待核实/)).toBeVisible();
  expect(screen.queryByText(/raw ID and diagnostics/)).not.toBeInTheDocument();
  fireEvent.click(screen.getByText("更多设置", { selector: "summary" }));
  fireEvent.change(screen.getByLabelText("环境失效原因"), {
    target: { value: "工具变化" },
  });
  expect(screen.getByRole("button", { name: "标记失效" })).toBeDisabled();
  fireEvent.click(await screen.findByRole("button", { name: "停止检测" }));
  expect(send).toHaveBeenCalledWith(
    "environment_stop",
    record.id,
    record.revision,
    {},
  );
});

it("binds invalidation to the selected environment revision", async () => {
  const { client, send } = fixture();
  await loaded();
  await updateOptions(client, (options) =>
    options.environments.push({
      ...options.environments[0],
      environment_key: "f".repeat(64),
      current_revision: 7,
    }),
  );
  fireEvent.change(screen.getByLabelText("运行环境"), {
    target: { value: "f".repeat(64) },
  });
  fireEvent.click(screen.getByText("更多设置", { selector: "summary" }));
  fireEvent.change(screen.getByLabelText("环境失效原因"), {
    target: { value: "账号变化" },
  });
  fireEvent.click(screen.getByRole("button", { name: "标记失效" }));
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
});

it("keeps collapsed history concise and filters by environment or project", async () => {
  const { api, record, send } = fixture();
  await loaded();
  await openHistory();
  await waitFor(() => expect(screen.getAllByText(record.id)).toHaveLength(2));
  expect(screen.getByText("未通过")).toBeVisible();
  expect(
    screen.queryByText("original-run", { exact: false }),
  ).not.toBeInTheDocument();
  expect(api.get).toHaveBeenCalledWith(
    "/environment?environment_key=" + "e".repeat(64),
    expect.anything(),
  );
  fireEvent.change(screen.getByLabelText("历史范围"), {
    target: { value: "project" },
  });
  await waitFor(() =>
    expect(api.get).toHaveBeenCalledWith("/environment", expect.anything()),
  );
  expect(
    await screen.findByText(/远程工作站 · Orca → Codex/, { selector: "p" }),
  ).toBeVisible();
  expect(send).not.toHaveBeenCalled();
});

it("allows reading project history offline and disables stopping", async () => {
  const { record } = fixture(false);
  record.settled = false;
  await openHistory();
  fireEvent.change(screen.getByLabelText("历史范围"), {
    target: { value: "project" },
  });
  expect(await screen.findByText(record.id)).toBeVisible();
  expect(screen.getByRole("button", { name: "停止检测" })).toBeDisabled();
});

it("locates the linked scheme and returns to the task without executing anything", async () => {
  const { client, send } = fixture(
    true,
    vi.fn(),
    false,
    "/projects/project/environment?profile=remote&task=38",
  );
  await loaded();
  expect(screen.getByRole("link", { name: "返回任务" })).toHaveAttribute(
    "href",
    "/projects/project/runs?task=38&start_profile=remote",
  );
  await updateOptions(client, (options) => {
    options.environments[0].profiles.push({
      ...options.profiles[0],
      id: "other",
      name: "另一方案",
    });
  });
  expect(await screen.findByLabelText("检测所用方案")).toHaveValue("remote");
  await waitFor(() =>
    expect(document.activeElement?.id).toBe(`environment-${"e".repeat(64)}`),
  );
  expect(send).not.toHaveBeenCalled();
});

it("does not turn a project ready report into platform qualification", async () => {
  const { record } = fixture();
  Object.assign(record.rounds[0], {
    report: {
      completed: true,
      ready: true,
      message: "project ready",
      details: { checks: "opaque data" },
    },
  });
  await loaded();
  expect(screen.getByRole("heading", { name: "检测阻断" })).toBeVisible();
  expect(screen.queryByText("project ready")).not.toBeInTheDocument();
  expect(screen.queryByText(/opaque data/)).not.toBeInTheDocument();
});

it("shows configured choices while qualification is pending but disables detection", async () => {
  const { send } = fixture(true, vi.fn(), false, "", true);
  expect(
    await screen.findByRole("option", { name: "远程工作站 · Orca → Codex" }),
  ).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "重新检测" })).toBeDisabled();
  expect(screen.getByText("项目运行环境状态待核实")).toBeVisible();
  expect(send).not.toHaveBeenCalled();
});

it("retries configuration on refresh while qualification remains pending", async () => {
  const { client, send, recoverConfiguration } = fixture(
    true,
    vi.fn(),
    false,
    "",
    true,
    true,
  );
  await waitFor(() =>
    expect(
      client.getQueryState([...optionsKey.slice(0, -1), "configuration"])
        ?.status,
    ).toBe("error"),
  );
  recoverConfiguration();
  fireEvent.click(screen.getByRole("button", { name: "刷新环境" }));
  expect(
    await screen.findByRole("option", { name: "远程工作站 · Orca → Codex" }),
  ).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "重新检测" })).toBeDisabled();
  expect(send).not.toHaveBeenCalled();
});

function fixture(
  online = true,
  send = vi.fn().mockResolvedValue(true),
  emptyHistory = false,
  path = "",
  pendingReadiness = false,
  configurationFailure = false,
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      Response.json({
        environments: [{ id: remoteMachineID, name: "远程工作站" }],
      }),
    ),
  );
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
    environment_key: "e".repeat(64),
    created_at: "2026-11-02T06:30:00Z",
    state: "failed",
    status: "not_ready",
    settled: true,
    diagnostic: "",
    rounds: [round],
  };
  vi.spyOn(api, "get").mockImplementation(async (path) => {
    if (
      path === "/environment/options?configuration=true" &&
      configurationFailure
    )
      throw new Error("configuration temporarily unavailable");
    if (path === "/environment/options" && pendingReadiness)
      return new Promise(() => {});
    return (
      path.startsWith("/environment/options")
        ? {
            default_profile_id: "remote",
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
                target: {
                  kind: "orca_environment",
                  environment: remoteMachineID,
                },
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
          }
        : path.startsWith("/environment/materials")
          ? {
              document_path:
                "SteerlineFlow/environment.md",
              document: "Shared project requirements",
            }
          : path.split("?")[0] === "/environment"
            ? { records: emptyHistory ? [] : [{ ...record, rounds: [] }] }
            : record
    ) as never;
  });
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
  return {
    send,
    record,
    api,
    client,
    recoverConfiguration: () => {
      configurationFailure = false;
    },
  };
}
