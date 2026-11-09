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
import { Dispatch } from "../tasks/dispatch";
import { useOriginalRequests } from "../../app/use-original-requests";
import { WorkspaceContext, useWorkspace } from "../../app/context";

afterEach(() => {
  cleanup();
  sessionStorage.clear();
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
const overview = () => screen.getByRole("region", { name: "全部运行环境" });
const environmentRow = (name: string) =>
  within(overview()).getByRole("row", { name: new RegExp(name) });
function chooseEnvironment(name: string) {
  fireEvent.click(within(environmentRow(name)).getByRole("button"));
}
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

it("shows environment readiness directly with lazy details and only the detection ID for debugging", async () => {
  const { api, record, send } = fixture();
  await loaded();
  expect(
    await within(overview()).findByText("远程工作站 · Orca → Codex"),
  ).toBeInTheDocument();
  expect(within(overview()).getByText("缺少工具")).toBeVisible();
  expect(within(currentCard()).getByText("缺少工具")).toBeVisible();
  expect(screen.queryByLabelText("运行环境")).not.toBeInTheDocument();
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
    within(overview()).getByText("远端机器 · Orca → Codex"),
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
    within(overview()).getByText("远程工作站 · Orca → Codex · 远端一"),
  ).toBeInTheDocument();
  expect(
    within(overview()).getByText("远程工作站 · Orca → Codex · 测试账号"),
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
  const { api, client, record } = fixture(true, send);
  await loaded();
  fireEvent.click(screen.getByText("更多设置", { selector: "summary" }));
  fireEvent.change(screen.getByLabelText("本次检测总超时（秒）"), {
    target: { value: "120" },
  });
  const start = screen.getByRole("button", { name: "重新检测" });
  fireEvent.click(start);
  await screen.findByRole("heading", { name: "正在确认启动" });
  expect(start).toBeDisabled();
  fireEvent.click(start);
  await waitFor(() => expect(send).toHaveBeenCalledTimes(1));
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
  record.settled = false;
  record.state = "running";
  await updateOptions(client, ({ environments }) => {
    environments[0].stage = "verifying";
    environments[0].activity_settled = false;
  });
  expect(
    screen.queryByRole("heading", { name: "检测中" }),
  ).not.toBeInTheDocument();
  const get = vi.mocked(api.get).getMockImplementation()!;
  const running = client.getQueryData<EnvironmentOptions>(optionsKey);
  vi.mocked(api.get).mockImplementation(async (path, signal) =>
    path === "/environment/options" ? (running as never) : get(path, signal),
  );
  vi.mocked(fetch).mockImplementationOnce(() => new Promise(() => {}));
  finish(true);
  expect(await screen.findByRole("heading", { name: "检测中" })).toBeVisible();
  expect(screen.queryByText("检测超时")).not.toBeInTheDocument();
});

it("shows a recorded detection timeout on the card and in history", async () => {
  const { client, record } = fixture();
  record.timed_out = true;
  await loaded();
  await updateOptions(client, ({ environments }) => {
    environments[0].stage = "timed_out";
  });
  expect(screen.getByRole("heading", { name: "检测超时" })).toBeVisible();
  await openHistory();
  expect(
    await screen.findByText("检测超时", { selector: "strong" }),
  ).toBeVisible();
});

it("shows an unsettled blocked Run on the card and in history without restarting or stopping it", async () => {
  const { client, record, send } = fixture();
  record.state = "running";
  record.settled = false;
  record.blocked = true;
  record.diagnostic = "远端 Orca 仓库查询超时，无法连接远端运行时";
  await loaded();
  await updateOptions(client, ({ environments }) => {
    environments[0].stage = "blocked";
    environments[0].reason = record.diagnostic;
    environments[0].activity_settled = false;
  });
  expect(within(currentCard()).getByText(record.diagnostic)).toBeVisible();
  expect(
    within(currentCard()).getByText("本次检测尚未结束，可停止检测后重新发起。"),
  ).toBeVisible();
  expect(
    within(currentCard()).queryByRole("button", { name: "重新检测" }),
  ).not.toBeInTheDocument();
  await openHistory();
  const history = screen
    .getByText("检测历史", { selector: "summary" })
    .closest("details")!;
  expect(
    await within(history).findByText("检测阻断", { selector: "strong" }),
  ).toBeVisible();
  expect(within(history).getByText(record.diagnostic)).toBeVisible();
  expect(send).not.toHaveBeenCalled();
  fireEvent.click(
    within(currentCard()).getByRole("button", { name: "停止检测" }),
  );
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "environment_stop",
      record.id,
      record.revision,
      {},
    ),
  );
});

it("polls the requested detection ID and shows a later blocked observation", async () => {
  const { api, record, send } = fixture(
    true,
    undefined,
    false,
    `/projects/project/environment?activity=${"a".repeat(32)}`,
  );
  record.state = "running";
  record.settled = false;
  const get = vi.mocked(api.get).getMockImplementation()!;
  vi.mocked(api.get).mockImplementation(async (path, signal) =>
    path === `/environment/${record.id}`
      ? (structuredClone(record) as never)
      : get(path, signal),
  );
  const detail = await screen.findByRole("region", { name: "指定检测详情" });
  expect(await within(detail).findByText(/检测中 · remote/)).toBeVisible();
  record.blocked = true;
  record.diagnostic = "远端 Orca 连接超时";
  await waitFor(
    () => {
      expect(within(detail).getByText(/检测阻断 · remote/)).toBeVisible();
      expect(within(detail).getByText(record.diagnostic)).toBeVisible();
    },
    { timeout: 4500 },
  );
  expect(send).not.toHaveBeenCalled();
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
  chooseEnvironment("远程工作站 · Orca → Codex");
  expect(await screen.findByLabelText("检测所用方案")).toHaveValue("");
  expect(screen.getByRole("button", { name: "重新检测" })).toBeDisabled();
  fireEvent.change(screen.getByLabelText("检测所用方案"), {
    target: { value: "another" },
  });
  expect(screen.getByRole("button", { name: "重新检测" })).toBeEnabled();
  expect(send).not.toHaveBeenCalled();
});

it("restores the selected environment and scheme after switching pages", async () => {
  const { api, client, send } = fixture();
  await loaded();
  const key = "f".repeat(64);
  await updateOptions(client, (options) => {
    options.environments.push({
      ...options.environments[0],
      environment_key: key,
      target: { kind: "local" },
      profiles: [
        { ...options.profiles[0], id: "local-one", environment_key: key },
        { ...options.profiles[0], id: "local-two", environment_key: key },
      ],
    });
  });
  const options = client.getQueryData<EnvironmentOptions>(optionsKey)!;
  const read = vi.mocked(api.get).getMockImplementation()!;
  vi.mocked(api.get).mockImplementation(async (path, signal) =>
    path.startsWith("/environment/options")
      ? (options as never)
      : read(path, signal),
  );
  chooseEnvironment("本机 · Orca → Codex");
  fireEvent.change(screen.getByLabelText("检测所用方案"), {
    target: { value: "local-two" },
  });
  cleanup();
  const otherPage = mountFeature(<Dispatch />, api, client);
  otherPage.unmount();
  mountFeature(<ProjectEnvironment />, api, client, true, { send });
  await screen.findByRole("region", { name: "全部运行环境" });
  expect(
    within(environmentRow("本机 · Orca → Codex")).getByRole("button"),
  ).toHaveAttribute("aria-expanded", "true");
  expect(screen.getByLabelText("检测所用方案")).toHaveValue("local-two");
  fireEvent.click(screen.getByRole("button", { name: "重新检测" }));
  expect(send).toHaveBeenCalledWith(
    "environment_start",
    identity.project_id,
    1,
    expect.objectContaining({ environment_key: key, profile_id: "local-two" }),
  );
});

it("shows the running default detection on the dispatch page and labels read failures unverified", async () => {
  const { api, client } = fixture();
  await loaded();
  const options = structuredClone(
    client.getQueryData<EnvironmentOptions>(optionsKey)!,
  );
  options.profiles[0].stage = "verifying";
  const read = vi.mocked(api.get).getMockImplementation()!;
  vi.mocked(api.get).mockImplementation(async (path, signal) =>
    path === "/environment/options" ? (options as never) : read(path, signal),
  );
  cleanup();
  mountFeature(<Dispatch />, api, client);
  expect(await screen.findByText(/默认执行环境检测中/)).toBeVisible();
  vi.mocked(api.get).mockImplementation(async (path, signal) => {
    if (path === "/environment/options") throw new Error("read failed");
    return read(path, signal);
  });
  await act(async () => {
    await client.refetchQueries({ queryKey: optionsKey });
  });
  expect(await screen.findByText(/默认执行环境待核实/)).toBeVisible();
  expect(screen.queryByText(/默认执行环境阻断/)).not.toBeInTheDocument();
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
    options.environments[0].reason = "环境要求 go：1.26.0 → 1.27.0；须重新检测";
  });
  expect(
    await within(currentCard()).findByText(
      "环境要求 go：1.26.0 → 1.27.0；须重新检测",
    ),
  ).toBeVisible();
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
  expect(screen.getByRole("heading", { name: "待核实" })).toBeVisible();
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
      profiles: [{ ...options.profiles[0], id: "other", name: "其他环境" }],
    }),
  );
  chooseEnvironment("远程工作站 · Orca → Codex · 其他环境");
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
    await screen.findByRole("region", { name: "全部运行环境" }),
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
    await screen.findByRole("region", { name: "全部运行环境" }),
  ).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "重新检测" })).toBeDisabled();
  expect(send).not.toHaveBeenCalled();
});

function multipleEnvironments(options: EnvironmentOptions) {
  const base = options.environments[0];
  options.environments.unshift({
    ...base,
    environment_key: "direct",
    target: { kind: "local" },
    adapter: "codex-app-server",
    status: "ready",
    stage: "ready",
    reason: "",
    activity_id: undefined,
    profiles: [
      { ...options.profiles[0], id: "daily", name: "日常开发" },
      { ...options.profiles[0], id: "review", name: "深度审查" },
    ],
  });
  options.environments.push(
    {
      ...base,
      environment_key: "dsh",
      target: { kind: "local" },
      agent: "DSH",
      stage: "verifying",
      activity_id: undefined,
      activity_settled: false,
      profiles: [{ ...options.profiles[0], id: "dsh", name: "DSH 开发" }],
    },
    {
      ...base,
      environment_key: "pi",
      target: { kind: "local" },
      agent: "Pi",
      stage: "stale",
      reason: "准备材料已更新",
      activity_id: undefined,
      profiles: [{ ...options.profiles[0], id: "pi", name: "Pi 开发" }],
    },
  );
}

const overviewRows = () => within(overview()).getAllByRole("row").slice(1);

it("shows every environment and shared scheme without switching, with initial attention-first ordering", async () => {
  const { send } = fixture(
    true,
    undefined,
    false,
    "",
    false,
    false,
    multipleEnvironments,
  );
  await loaded();
  expect(
    overviewRows().map((row) => row.querySelector("strong")?.textContent),
  ).toEqual([
    "远程工作站 · Orca → Codex",
    "本机 · Orca → Pi",
    "本机 · Orca → DSH",
    "本机 · Codex 直连",
  ]);
  expect(
    within(environmentRow("本机 · Codex 直连")).getByText(
      "关联方案：日常开发、深度审查",
    ),
  ).toBeVisible();
  expect(within(overview()).getByRole("status")).toHaveTextContent(
    "1/4 已就绪检测中 1需要处理 2",
  );
  expect(
    within(environmentRow("本机 · Orca → Pi")).getByText("准备材料已更新"),
  ).toBeVisible();
  expect(send).not.toHaveBeenCalled();
});

it("updates all readiness rows in place and keeps the overview visible when details are collapsed or changed", async () => {
  const { client, send } = fixture(
    true,
    undefined,
    false,
    "",
    false,
    false,
    multipleEnvironments,
  );
  await loaded();
  await within(overview()).findByText("远程工作站 · Orca → Codex");
  const before = overviewRows().map(
    (row) => row.querySelector("strong")?.textContent,
  );
  await updateOptions(client, ({ environments }) => {
    environments[0].status = "not_ready";
    environments[0].stage = "blocked";
    environments[0].reason = "认证已失效";
    environments[1].status = "ready";
    environments[1].stage = "ready";
  });
  expect(
    overviewRows().map((row) => row.querySelector("strong")?.textContent),
  ).toEqual(before);
  expect(
    within(environmentRow("本机 · Codex 直连")).getByText("认证已失效"),
  ).toBeVisible();
  chooseEnvironment("远程工作站 · Orca → Codex");
  expect(
    screen.queryByRole("region", { name: "当前运行环境" }),
  ).not.toBeInTheDocument();
  expect(overviewRows()).toHaveLength(4);
  chooseEnvironment("本机 · Orca → Pi");
  expect(
    within(currentCard()).getByRole("heading", { name: "本机 · Orca → Pi" }),
  ).toBeVisible();
  expect(overviewRows()).toHaveLength(4);
  expect(send).not.toHaveBeenCalled();
});

it("marks every cached row unverified after a read failure instead of retaining current ready totals", async () => {
  const { api, client } = fixture(
    true,
    undefined,
    false,
    "",
    false,
    false,
    multipleEnvironments,
  );
  await loaded();
  const read = vi.mocked(api.get).getMockImplementation()!;
  vi.mocked(api.get).mockImplementation(async (path, signal) => {
    if (path === "/environment/options") throw new Error("offline");
    return read(path, signal);
  });
  await act(async () => {
    await client.refetchQueries({ queryKey: optionsKey });
  });
  await waitFor(() =>
    expect(within(overview()).getByRole("status")).toHaveTextContent(
      "共 4 个环境 · 当前状态待核实",
    ),
  );
  expect(
    within(overview()).getAllByText("待核实", { exact: true }),
  ).toHaveLength(4);
  expect(
    within(environmentRow("本机 · Codex 直连")).getByText("上次状态：已就绪"),
  ).toBeVisible();
});

it("shows disabled readiness enforcement separately from actual qualification", async () => {
  fixture(true, undefined, false, "", false, false, (options) => {
    multipleEnvironments(options);
    options.environment_readiness_required = false;
  });
  await loaded();
  await within(overview()).findByText("远程工作站 · Orca → Codex");
  expect(
    screen.getByText("环境就绪阻塞已关闭；检测结果仍按实际状态展示。"),
  ).toBeVisible();
  expect(within(overview()).getByRole("status")).toHaveTextContent(
    "1/4 已就绪",
  );
  expect(
    within(environmentRow("远程工作站 · Orca → Codex")).getByText("检测阻断"),
  ).toBeVisible();
});

it("keeps a pending original submission visible in its row after opening another environment", async () => {
  let finish!: (accepted: boolean) => void;
  const send = vi.fn(
    () =>
      new Promise<boolean>((resolve) => {
        finish = resolve;
      }),
  );
  fixture(true, send, false, "", false, false, multipleEnvironments);
  await loaded();
  fireEvent.click(screen.getByRole("button", { name: "重新检测" }));
  await screen.findByRole("heading", { name: "正在确认启动" });
  chooseEnvironment("本机 · Orca → Pi");
  expect(
    within(environmentRow("远程工作站 · Orca → Codex")).getByText(
      "正在确认启动",
    ),
  ).toBeVisible();
  chooseEnvironment("远程工作站 · Orca → Codex");
  expect(screen.getByRole("button", { name: "正在提交检测…" })).toBeDisabled();
  expect(send).toHaveBeenCalledTimes(1);
  await act(async () => {
    finish(true);
  });
});

it("does not steal focus back to a linked scheme after choosing another environment", async () => {
  fixture(
    true,
    undefined,
    false,
    "/projects/project/environment?profile=remote&task=38",
    false,
    false,
    multipleEnvironments,
  );
  await loaded();
  const button = within(environmentRow("本机 · Orca → Pi")).getByRole("button");
  button.focus();
  fireEvent.click(button);
  expect(document.activeElement).toBe(button);
  expect(
    within(currentCard()).getByRole("heading", { name: "本机 · Orca → Pi" }),
  ).toBeVisible();
});

function fixture(
  online = true,
  send = vi.fn().mockResolvedValue(true),
  emptyHistory = false,
  path = "",
  pendingReadiness = false,
  configurationFailure = false,
  configureOptions?: (options: EnvironmentOptions) => void,
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
    blocked: false,
    timed_out: false,
    diagnostic: "",
    rounds: [round],
  };
  vi.spyOn(api, "get").mockImplementation(async (path) => {
    if (path === "/maintenance/readiness")
      return {
        status: "ready",
        commit: "base",
        total: 1,
        ready: 1,
        blockers: [],
      } as never;
    if (
      path === "/environment/options?configuration=true" &&
      configurationFailure
    )
      throw new Error("configuration temporarily unavailable");
    if (path === "/environment/options" && pendingReadiness)
      return new Promise(() => {});
    const response = (
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
              document_path: "SteerlineFlow/environment.md",
              document: "Shared project requirements",
            }
          : path.split("?")[0] === "/environment"
            ? { records: emptyHistory ? [] : [{ ...record, rounds: [] }] }
            : record
    ) as never;
    if (path.startsWith("/environment/options")) configureOptions?.(response);
    return response;
  });
  const client = testClient();
  vi.spyOn(api, "submit").mockImplementation(async (envelope) => {
    const accepted = await send(
      envelope.action,
      envelope.target_id,
      envelope.expected_version,
      envelope.payload,
    );
    return {
      ...envelope,
      original: envelope,
      status: accepted ? "applied" : "outcome_unknown",
      sampled_at: new Date().toISOString(),
      environment: record,
    };
  });
  function ObservedEnvironment() {
    const workspace = useWorkspace();
    const original = useOriginalRequests(api, online);
    return (
      <WorkspaceContext.Provider
        value={{
          ...workspace,
          submit: original.submit,
          requests: original.observedRequests,
          pendingRequests: original.pendingRequests,
        }}
      >
        <ProjectEnvironment />
      </WorkspaceContext.Provider>
    );
  }
  mountFeature(
    <>
      {path && <Navigate to={path} />}
      <ObservedEnvironment />
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
