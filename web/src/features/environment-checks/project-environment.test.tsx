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
  await screen.findByText("Orca · Codex · 检测阻断");
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
  fireEvent.click(await screen.findByRole("button", { name: /环境 ffffffff/ }));
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
  await screen.findByText("Orca · Codex · 检测阻断");
  const options = structuredClone(
    client.getQueryData<EnvironmentOptions>(optionsKey)!,
  );
  options.environments[0].stage = "ready";
  options.environments[0].status = "ready";
  client.setQueryData(optionsKey, options);
  vi.mocked(api.get).mockImplementation(async (path) => {
    if (path === "/environment/options") throw new Error("options unavailable");
    return (
      path.split("?")[0] === "/environment"
        ? { records: [{ ...record, rounds: [] }] }
        : record
    ) as never;
  });
  await client.refetchQueries({ queryKey: optionsKey });
  expect(await screen.findByText(/上次已知状态，当前待核实/)).toBeVisible();
  fireEvent.click(screen.getByText("重新检测", { selector: "summary" }));
  expect(screen.getByRole("button", { name: "提交检测" })).toBeDisabled();
  fireEvent.click(await screen.findByRole("button", { name: "停止原活动" }));
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
  await screen.findByText("Orca · Codex · 检测阻断");
  const options = structuredClone(
    client.getQueryData<EnvironmentOptions>(optionsKey)!,
  );
  Object.assign(options.environments[0], {
    stage: "stale",
    activity_settled: false,
  });
  client.setQueryData(optionsKey, options);
  fireEvent.click(await screen.findByText("重新检测", { selector: "summary" }));
  expect(screen.getByRole("button", { name: "提交检测" })).toBeDisabled();
  expect(
    await screen.findByRole("button", { name: "停止原活动" }),
  ).toBeEnabled();
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
  fireEvent.click(await screen.findByRole("button", { name: /环境 ffffffff/ }));
  fireEvent.click(await screen.findByText("重新检测", { selector: "summary" }));
  expect(screen.getByLabelText("检测所用方案")).toHaveValue("");
  expect(screen.getByRole("button", { name: "提交检测" })).toBeDisabled();
  fireEvent.change(screen.getByLabelText("检测所用方案"), {
    target: { value: "another" },
  });
  expect(screen.getByRole("button", { name: "提交检测" })).toBeEnabled();
  expect(send).not.toHaveBeenCalled();
});

it("locates the linked profile and returns to the original task without executing anything", async () => {
  const { send } = fixture(
    true,
    vi.fn(),
    false,
    "/projects/project/environment?profile=remote&task=38",
  );
  expect(await screen.findByLabelText("检测所用方案")).toHaveValue("remote");
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
  pendingReadiness = false,
  configurationFailure = false,
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
          }
        : path.startsWith("/environment/materials")
          ? {
              document_path:
                "SteerlineFlow/environments/" +
                "e".repeat(64) +
                "/environment.md",
              document: "Only selected target requirements",
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
it("keeps progress separate from binary admission and stops the exact original activity", async () => {
  const { send, record } = fixture();
  expect(await screen.findByText("Orca · Codex · 检测阻断")).toBeVisible();
  fireEvent.click(await screen.findByRole("button", { name: "查看活动详情" }));
  expect(await screen.findByText(/Run original-run/)).toBeVisible();
  fireEvent.click(screen.getByText("原执行文件与技术详情"));
  expect(screen.getByText(/检测结果文件/)).toBeVisible();
  expect(screen.getByText(/停止后封存一次/)).toBeVisible();
  expect(screen.getByText("原执行说明：缺少必需工具")).toBeVisible();
  expect(
    screen.getByText(/Agent 回复（原生来源已核实，仅供排障）/),
  ).toBeVisible();
  fireEvent.click(await screen.findByRole("button", { name: "停止原活动" }));
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
  fireEvent.click(await screen.findByText(/检测结果文件/));
  expect(await screen.findByText("结果采集诊断：file_missing")).toBeVisible();
  expect(screen.getByText("Orca · Codex · 检测阻断")).toBeVisible();
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
  await screen.findByText("Orca · Codex · 检测阻断");
  fireEvent.click(screen.getByText("重新检测", { selector: "summary" }));
  fireEvent.change(screen.getByLabelText("检测所用方案"), {
    target: { value: "remote" },
  });
  fireEvent.change(screen.getByLabelText("本次检测总超时（秒）"), {
    target: { value: "120" },
  });
  const start = screen.getByRole("button", { name: "提交检测" });
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
it("keeps history readable while offline and disables mutations", async () => {
  fixture(false);
  fireEvent.click(screen.getByRole("button", { name: "项目全部活动" }));
  fireEvent.click(await screen.findByRole("button", { name: "查看活动详情" }));
  await screen.findByText(/Run original-run/);
  expect(
    screen.queryByRole("button", { name: "提交检测" }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "生成文档和脚本" }),
  ).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "停止原活动" })).toBeDisabled();
});

it("shows the selected current report even outside the recent history window", async () => {
  const { send, record } = fixture(true, vi.fn().mockResolvedValue(true), true);
  fireEvent.click(await screen.findByRole("button", { name: "查看当前检测" }));
  expect(await screen.findByText(/Run original-run/)).toBeVisible();
  fireEvent.click(await screen.findByRole("button", { name: "停止原活动" }));
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
  await screen.findByText("Orca · Codex · 检测阻断");
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

it("keeps model schemes inside operations and changes the history filter with the environment", async () => {
  const { api, client, send } = fixture();
  await screen.findByText("Orca · Codex · 检测阻断");
  const options = structuredClone(
    client.getQueryData<EnvironmentOptions>(optionsKey)!,
  );
  options.environments.push({
    ...options.environments[0],
    environment_key: "f".repeat(64),
    profiles: [
      {
        ...options.profiles[0],
        id: "alternate-model",
        name: "Different model",
        model: "model-c",
        reasoning_effort: "high",
      },
    ],
  });
  client.setQueryData(optionsKey, options);
  const chooser = screen.getByRole("region", { name: "选择运行机与链路" });
  expect(chooser).not.toHaveTextContent("Different model");
  expect(chooser).not.toHaveTextContent("model-c");
  fireEvent.click(
    await within(chooser).findByRole("button", { name: /环境 ffffffff/ }),
  );
  await waitFor(() =>
    expect(api.get).toHaveBeenCalledWith(
      "/environment?environment_key=" + "f".repeat(64),
      expect.anything(),
    ),
  );
  expect(screen.getByText(/共用方案：Different model/)).toBeVisible();
  expect(
    screen.getByRole("option", { name: "Different model · model-c · high" }),
  ).toBeInTheDocument();
  expect(send).not.toHaveBeenCalled();
});

it("shows project failure and opaque diagnostics without reviewing their contents", async () => {
  const { record, send } = fixture();
  Object.assign(record.rounds[0], {
    report: {
      completed: true,
      ready: false,
      message: "服务连接失败：database unavailable",
      details: {
        project_checks: [{ id: "business-only", status: "passed" }],
        report_url: "http://gitea.local/pulls/1",
      },
    },
  });
  fireEvent.click(await screen.findByRole("button", { name: "查看活动详情" }));
  expect(
    await screen.findByText("服务连接失败：database unavailable"),
  ).toBeVisible();
  expect(screen.getByText("项目结论：未就绪")).toBeVisible();
  fireEvent.click(screen.getByText("项目诊断明细"));
  expect(screen.getByText(/business-only/)).toBeVisible();
  expect(
    screen.queryByRole("link", { name: "http://gitea.local/pulls/1" }),
  ).not.toBeInTheDocument();
  expect(screen.getByText("Orca · Codex · 检测阻断")).toBeVisible();
  expect(send).not.toHaveBeenCalled();
});
it("keeps a project ready report separate from unsettled platform qualification", async () => {
  const { record } = fixture();
  Object.assign(record.rounds[0], {
    report: {
      completed: true,
      ready: true,
      message: "project decided ready",
      details: {
        checks: [{ status: "failed", format: "opaque-project-diagnostic" }],
      },
    },
  });
  fireEvent.click(await screen.findByRole("button", { name: "查看活动详情" }));
  expect(await screen.findByText("项目结论：已就绪")).toBeVisible();
  expect(screen.getByText("Orca · Codex · 检测阻断")).toBeVisible();
  fireEvent.click(screen.getByText("项目诊断明细"));
  expect(screen.getByText(/opaque-project-diagnostic/)).toBeVisible();
});
it("provides project interface guidance without generation or protocol synchronization actions", async () => {
  fixture();
  await screen.findByText("Orca · Codex · 检测阻断");
  expect(screen.queryByText("生成或更新准备材料")).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: /生成文档|导出.*上下文|同步协议/ }),
  ).not.toBeInTheDocument();
});

it("shows configured machines and links while project qualification is pending without enabling detection", async () => {
  const { send } = fixture(true, vi.fn(), false, "", true);
  const chooser = screen.getByRole("region", { name: "选择运行机与链路" });
  expect(
    await within(chooser).findByRole("button", { name: /环境 eeeeeeee/ }),
  ).toBeVisible();
  await screen.findByText(/共用方案：远端一/);
  expect(screen.getByRole("button", { name: "提交检测" })).toBeDisabled();
  expect(screen.getByText("项目运行环境状态待核实")).toBeVisible();
  expect(send).not.toHaveBeenCalled();
});

it("retries configuration on explicit refresh while project qualification remains pending", async () => {
  const { api, client, send, recoverConfiguration } = fixture(
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
  expect(
    screen.queryByRole("button", { name: /环境 eeeeeeee/ }),
  ).not.toBeInTheDocument();
  const callsBeforeRefresh = vi
    .mocked(api.get)
    .mock.calls.filter(
      ([path]) => path === "/environment/options?configuration=true",
    ).length;
  recoverConfiguration();
  fireEvent.click(screen.getByRole("button", { name: "刷新环境" }));
  expect(
    await screen.findByRole("button", { name: /环境 eeeeeeee/ }),
  ).toBeVisible();
  await screen.findByText(/共用方案：远端一/);
  expect(screen.getByRole("button", { name: "提交检测" })).toBeDisabled();
  expect(
    vi
      .mocked(api.get)
      .mock.calls.filter(
        ([path]) => path === "/environment/options?configuration=true",
      ),
  ).toHaveLength(callsBeforeRefresh + 1);
  expect(send).not.toHaveBeenCalled();
});
