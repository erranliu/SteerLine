import { afterEach, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, testClient } from "../../test/workspace";
import { rememberAbandonedActivities } from "../activities/abandonment-marks";
import {
  RefactoringMaintenance,
  type MaintenanceRecord,
} from "./refactoring-maintenance";
import { activityDestination, requestDestination } from "../activities/model";
import type { Activity } from "../activities/model";
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
const id = "a".repeat(32);
const builtIn = {
  id: "update-stale-docs",
  name: "更新过期文档",
  prompt: "核实并更新过期文档。\n",
  revision: "b".repeat(64),
};
const custom = {
  id: "c".repeat(32),
  name: "收拢共享夹具",
  prompt: "# 收拢共享夹具\r\n\r\n第一段。  \r\n第二段。\r\n",
  revision: "c".repeat(64),
};
const record: MaintenanceRecord = {
  id,
  revision: 7,
  phase: "waiting_input",
  direction_name: builtIn.name,
  direction_prompt: builtIn.prompt,
  input: {
    direction: builtIn.id,
    profile: "local",
    strategy: "cautious",
    budget: {},
  },

  active: true,
  source_commit: "b".repeat(40),
  elapsed_seconds: 65,
  remaining_budget: { exhausted: false },
  cost_complete: false,
  question: {
    id: "question",
    prompt: "保留原公共接口还是允许调整？建议保留。",
    facts: ["有调用方"],
    attempts: [],
    options: [{ id: "keep", label: "保留公共接口", impact: "维持兼容" }],
    recommended_option_id: "keep",
    recommendation_reason: "易验证",
  },
  can_finish: true,
  finish_action: "early_finish",
  can_stop: true,
  can_conflict: false,
  operations: [],
  reports: [],
};
const local = { id: "local", name: "本机 Codex", version: "one" };
const key = "env-local";
const ready = {
  branch: "main",
  project: { status: "ready", commit: "b".repeat(40), blockers: [] },
  default_profile: local.id,
  environments: [
    {
      environment_key: key,
      name: "本机 · Codex 直连",
      target: { kind: "local" },
      status: "ready",
      reason: "",
      current_revision: 2,
      profiles: [local, { id: "second", name: "另一方案", version: "two" }],
    },
  ],
};
function setup(
  records: MaintenanceRecord[] = [],
  readiness = vi.fn(async () => ready),
) {
  const api = new Api(identity, "token", "home");
  const directions = [builtIn, custom];
  vi.spyOn(api, "getRefactoringDirections").mockImplementation(async () => ({
    directions: [...directions],
  }));
  vi.spyOn(api, "changeRefactoringDirection").mockImplementation(
    async (input) => {
      const change = input as {
        action: string;
        id?: string;
        name?: string;
        prompt?: string;
      };
      const index = directions.findIndex((row) => row.id === change.id);
      if (change.action === "delete") return directions.splice(index, 1)[0];
      const saved = {
        id: change.id ?? "d".repeat(32),
        revision: "d".repeat(64),
        name: change.name!,
        prompt: change.prompt!,
      };
      if (index === -1) directions.push(saved);
      else directions[index] = saved;
      return saved;
    },
  );
  vi.spyOn(api, "get").mockImplementation(async (path: string) => {
    if (path === "/refactoring") {
      return {
        ongoing: records.filter((r) => r.active),
        history: records.filter((r) => !r.active),
        slots_used: 3,
        slot_limit: 3,
      };
    }
    if (path === "/refactoring/readiness") return readiness();
    if (path.startsWith("/activities?")) return {};
    return records.find((r) => path.endsWith(r.id));
  });
  const submit = vi.fn(async (envelope) => ({
    ...envelope,
    status: "applied",
    refactoring: { id, revision: 8 },
    sampled_at: "now",
  }));
  const client = testClient();
  mountFeature(<RefactoringMaintenance />, api, client, true, { submit });
  return { submit, client, api };
}
function openSheet() {
  fireEvent.click(screen.getByRole("button", { name: "新建重构任务" }));
  return within(screen.getByRole("dialog"));
}

it.each([
  ["queued", "", { exhausted: false }, "完成一项改进后交付", ""],
  ["first", "", { seconds: 0, exhausted: true }, "0 秒", "完成当前工作后推进 PR"],
  [
    "execute",
    "publish",
    { dollars: "0", exhausted: true },
    "$0",
    "正在验证并创建 PR",
  ],
  [
    "first",
    "",
    { dollars: "0.05", cost_pending: true, exhausted: false },
    "$0.05",
    "费用待结算",
  ],
] as const)(
  "shows the separate budget at %s %s",
  async (phase, stage, remaining_budget, balance, status) => {
    setup([{ ...record, phase, delivery_stage: stage, remaining_budget }]);
    const budget = await screen.findByRole("region", { name: "剩余重构预算" });
    expect(budget.previousElementSibling).toHaveTextContent(builtIn.name);
    expect(within(budget).getByText(balance, { exact: true })).toBeVisible();
    if (status) expect(budget).toHaveTextContent(status);
    expect(budget.textContent?.includes("重构预算已用完")).toBe(
      remaining_budget.exhausted,
    );
  },
);

it.each([
  ["early_finish", "提前结束并创建 PR", "execute", ""],
  ["resume_delivery", "继续交付 PR", "stopped", "publish"],
  ["retry_publish", "重试创建 PR", "blocked", "publish"],
  ["retry_conflict", "重试更新 PR", "blocked", "publish"],
  ["verify_pr", "重新核验 PR", "blocked", "verify"],
  ["verify_publication", "重新核验 PR", "blocked", "publish"],
  ["retry_review", "重试评审", "blocked", "review"],
] as const)(
  "offers %s as an explicit delivery operation",
  async (action, label, phase, stage) => {
    const { submit } = setup([
      {
        ...record,
        phase,
        question: undefined,
        active: phase === "execute",
        can_stop: phase === "execute",
        finish_action: action,
        delivery_stage: stage,
      },
    ]);
    const button = await screen.findByRole("button", {
      name: label,
    });
    expect(
      screen.queryByRole("button", { name: "结束并交付" }),
    ).not.toBeInTheDocument();
    fireEvent.click(button);
    await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
    expect(submit.mock.calls[0][0]).toMatchObject({
      action: "refactoring_finish",
      target_id: id,
      expected_version: record.revision,
      payload: {},
    });
  },
);

it("presents the created PR with its summary and current review as the task result", async () => {
  const { submit } = setup([
    {
      ...record,
      phase: "delivered",
      delivery_stage: "complete",
      delivery_result: "pr",
      finish_action: "",
      active: false,
      question: undefined,
      can_stop: false,
      can_finish: false,
      delivery_summary: "更新了安装和部署操作说明。",
      pr: {
        url: "https://example.test/repo/pull/42",
        number: 42,
        title: "更新部署文档",
        head_revision: "head",
        base_revision: "base",
      },
      reports: [
        {
          head: "head",
          base: "base",
          body: "文档与实现一致。",
          recommendation: "merge",
          created_at: "2026-11-18T08:00:00Z",
        },
      ],
    },
  ]);
  const result = within(
    await screen.findByRole("region", { name: "交付结果" }),
  );
  expect(result.getByRole("heading", { name: "已创建 PR" })).toBeVisible();
  expect(
    result.getByRole("heading", { name: "PR #42 · 更新部署文档" }),
  ).toBeVisible();
  expect(result.getByRole("link", { name: "查看 PR" })).toHaveAttribute(
    "href",
    "https://example.test/repo/pull/42",
  );
  expect(result.getByText("更新了安装和部署操作说明。")).toBeVisible();
  expect(result.getByText("评审建议合入")).toBeVisible();
  expect(
    within(screen.getByRole("region", { name: "重构任务详情" })).queryByRole(
      "button",
      { name: /交付|创建 PR/ },
    ),
  ).not.toBeInTheDocument();
  expect(submit).not.toHaveBeenCalled();
});

it("keeps the PR visible during review and withholds an outdated recommendation", async () => {
  setup([
    {
      ...record,
      phase: "report",
      question: undefined,
      can_finish: false,
      finish_action: "",
      delivery_stage: "review",
      pr: { url: "https://example.test/repo/pull/42" },
      reports: [
        {
          head: "old-head",
          base: "old-base",
          body: "旧评审",
          recommendation: "merge",
          created_at: "2026-11-18T08:00:00Z",
        },
      ],
      report_stale: true,
    },
  ]);
  const result = within(
    await screen.findByRole("region", { name: "交付结果" }),
  );
  expect(result.getByRole("link", { name: "查看 PR" })).toBeVisible();
  expect(
    result.getByRole("heading", { name: "PR 已创建，正在评审" }),
  ).toBeVisible();
  expect(result.queryByText("评审建议合入")).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: /提前结束|继续交付/ }),
  ).not.toBeInTheDocument();
});

it("shows an explicit no-change outcome and offers no PR operation after completion", async () => {
  setup([
    {
      ...record,
      phase: "delivered",
      delivery_stage: "complete",
      delivery_result: "no_changes",
      active: false,
      can_finish: false,
      can_stop: false,
      finish_action: "",
      question: undefined,
      summary: "检查部署文档后确认内容与当前实现一致。",
    },
  ]);
  const result = within(
    await screen.findByRole("region", { name: "交付结果" }),
  );
  expect(
    result.getByRole("heading", { name: "已完成 · 无需修改" }),
  ).toBeVisible();
  expect(
    result.getByText("检查部署文档后确认内容与当前实现一致。"),
  ).toBeVisible();
  expect(
    result.queryByRole("link", { name: "查看 PR" }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: /交付|创建 PR/ }),
  ).not.toBeInTheDocument();
});

it("waits for a stopped execution to settle before offering delivery recovery", async () => {
  setup([
    {
      ...record,
      phase: "stopping",
      delivery_stage: "publish",
      question: undefined,
      can_finish: false,
      can_stop: false,
      finish_action: "",
    },
  ]);
  await screen.findByText("停止待结算", { selector: "span" });
  const detail = within(await screen.findByRole("region", { name: "重构任务详情" }));
  expect(
    detail.queryByRole("region", { name: "交付结果" }),
  ).not.toBeInTheDocument();
  expect(
    detail.queryByRole("button", { name: /交付|创建 PR/ }),
  ).not.toBeInTheDocument();
});

it("copies the selected refactoring task and its Runner runs without submitting a command", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal("navigator", { clipboard: { writeText } });
  const historyID = "d".repeat(32);
  const { submit, api } = setup([
    {
      ...record,
      phase: "execute",
      environment_name: "本机 Codex",
      operations: [
        {
          id: "decision-operation",
          purpose: "first",

          workflow: "generate",
          state: "complete",
          feedback: "决策反馈正文",
          prompt: "只读选择一个切片。",
          run_id: "decision-run",
        },
        {
          id: "execution-operation",
          purpose: "implement",

          workflow: "execute",
          state: "submitted",
          feedback: "",
          prompt: "实施切片并验证。",
          run_id: "execution-run",
        },
      ],
    },
    {
      ...record,
      id: historyID,
      direction_name: "历史方向",
      phase: "blocked",
      active: false,
      diagnostic: "运行环境不可用",
      operations: [
        {
          id: "history-operation",
          purpose: "implement",

          workflow: "execute",
          state: "complete",
          feedback: "",
          prompt: "历史执行指令",
          run_id: "history-run",
        },
      ],
    },
  ]);

  const detail = () =>
    within(screen.getByRole("region", { name: "重构任务详情" }));
  await screen.findByRole("region", { name: "重构任务详情" });
  fireEvent.click(detail().getByRole("button", { name: "复制调试信息" }));
  await detail().findByText("已复制调试信息");
  expect(writeText).toHaveBeenLastCalledWith(
    [
      "repo · 重构任务 · 更新过期文档",
      `请求 / 重构任务 ID：${id}（本次提交及结果）`,
      "状态：Runner 执行中",
      "运行环境：本机 Codex",
      `起始提交：${record.source_commit}`,
      "",
      "初始选题 · 已完成",
      "操作 ID：decision-operation",
      "Runner Run ID：decision-run",
      "",
      "实施 · 已提交",
      "操作 ID：execution-operation",
      "Runner Run ID：execution-run",
    ].join("\n"),
  );

  const historyPath = `/refactoring/${historyID}`;
  expect(api.get).not.toHaveBeenCalledWith(historyPath, expect.anything());
  const historyTask = screen.getByRole("button", { name: /历史方向/ });
  const row = within(historyTask.closest("li")!);
  fireEvent.click(row.getByRole("button", { name: "复制调试信息" }));
  await row.findByText("已复制调试信息");
  expect(api.get).toHaveBeenCalledWith(historyPath, expect.anything());
  expect(detail().getByRole("heading", { name: builtIn.name })).toBeVisible();
  const historyText = [
    "repo · 重构任务 · 历史方向",
    `请求 / 重构任务 ID：${historyID}（本次提交及结果）`,
    "状态：已阻断",
    `起始提交：${record.source_commit}`,
    "诊断：运行环境不可用",
    "",
    "实施 · 已完成",
    "操作 ID：history-operation",
    "Runner Run ID：history-run",
  ].join("\n");
  expect(writeText).toHaveBeenLastCalledWith(historyText);

  fireEvent.click(historyTask);
  await detail().findByRole("heading", { name: "历史方向" });
  fireEvent.click(detail().getByRole("button", { name: "复制调试信息" }));
  await detail().findByText("已复制调试信息");
  expect(writeText).toHaveBeenLastCalledWith(historyText);
  expect(submit).not.toHaveBeenCalled();
});

it("moves a followed action to history and keeps its occupied slot linked", async () => {
  const action = { ...record, phase: "stopping", slot: true };
  const { client } = setup([action]);
  const rowName = new RegExp(builtIn.name);
  const row = await screen.findByRole("button", { name: rowName });
  await waitFor(() => expect(row).toHaveAttribute("aria-current", "true"));

  act(() => {
    rememberAbandonedActivities(
      client,
      ["home", identity.project_id, identity.instance_id],
      [
        {
          source: "refactoring",
          source_id: id,
          abandoned_at: "2026-11-15T00:00:00Z",
        },
      ],
    );
  });
  await screen.findByText("选择一个重构任务查看详情");
  expect(screen.getByRole("heading", { name: "进行中 0" })).toBeInTheDocument();
  expect(
    screen.getByRole("heading", { name: "重构任务历史" }).nextElementSibling,
  ).toContainElement(screen.getByRole("button", { name: rowName }));
  expect(screen.getByText("推进名额 3 / 3")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("link", { name: "核实原重构任务" }));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: rowName })).toHaveAttribute(
      "aria-current",
      "true",
    ),
  );
});

it("starts at full capacity using a versioned ready environment and the default plan", async () => {
  const { submit } = setup();
  const sheet = openSheet();
  const start = sheet.getByRole("button", { name: "启动重构任务" });
  await waitFor(() => expect(start).toBeEnabled());
  expect(sheet.getByText("推进名额已满，提交后进入队列。")).toBeInTheDocument();
  expect(sheet.getByLabelText("运行环境")).toHaveValue(key);
  expect(sheet.getByLabelText("决策策略")).toHaveValue("cautious");
  expect(sheet.queryByLabelText("重构轮次")).not.toBeInTheDocument();
  expect(sheet.getByText(/预算可选/)).toBeVisible();
  fireEvent.click(start);
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0]).toMatchObject({
    action: "refactoring_start",
    target_id: identity.repository_identity,
    expected_version: 1,
    payload: {
      direction: builtIn.id,
      direction_revision: builtIn.revision,
      direction_prompt: builtIn.prompt,
      strategy: "cautious",
      profile: local.id,
      profile_version: local.version,
      environment_key: key,
      environment_revision: 2,
      base_commit: ready.project.commit,
      budget: {},
    },
  });
});

it("keeps the full direction preview and drafts when the sheet closes", async () => {
  setup();
  const sheet = openSheet();
  const preview = await sheet.findByLabelText("方向正文");
  expect(sheet.getByRole("heading", { name: "维护方向" })).toBeVisible();
  expect(sheet.getByRole("heading", { name: "执行指令" })).toBeVisible();
  expect(
    sheet.getByText(
      "启动后自动实施和验证，完成后创建 PR 并评审。没有有效改动时保存调查结论。",
    ),
  ).toBeVisible();
  expect(preview).toHaveValue(builtIn.prompt);
  fireEvent.change(sheet.getByLabelText("重构方向"), {
    target: { value: custom.id },
  });
  expect(preview).toHaveValue(custom.prompt.replaceAll("\r\n", "\n"));
  fireEvent.change(preview, {
    target: { value: "先检查共享夹具" },
  });
  fireEvent.click(sheet.getByRole("button", { name: "关闭" }));
  const reopened = openSheet();
  expect(reopened.getByLabelText("方向正文")).toHaveValue("先检查共享夹具");
});

it("starts with the edited body without saving it to the catalogue", async () => {
  const { api, submit } = setup();
  const sheet = openSheet();
  const body = await sheet.findByLabelText("方向正文");
  fireEvent.change(body, { target: { value: "本次只更新部署文档" } });
  const start = sheet.getByRole("button", { name: "启动重构任务" });
  await waitFor(() => expect(start).toBeEnabled());
  fireEvent.click(start);
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0].payload).toMatchObject({
    direction: builtIn.id,
    direction_prompt: "本次只更新部署文档",
  });
  expect(api.changeRefactoringDirection).not.toHaveBeenCalled();
});

it("saves built-in edits as a new direction and can update its body before launch", async () => {
  const { api, submit } = setup();
  const sheet = openSheet();
  const body = await sheet.findByLabelText("方向正文");
  fireEvent.change(body, { target: { value: "检查部署文档" } });
  fireEvent.click(sheet.getByRole("button", { name: "另存为新方向" }));
  fireEvent.change(sheet.getByLabelText("新方向名称"), {
    target: { value: "部署文档" },
  });
  const start = sheet.getByRole("button", { name: "启动重构任务" });
  expect(start).toBeDisabled();
  fireEvent.submit(start.closest("form")!);
  expect(submit).not.toHaveBeenCalled();
  fireEvent.click(sheet.getByRole("button", { name: "保存新方向" }));
  await sheet.findByText("已保存方向");
  expect(api.changeRefactoringDirection).toHaveBeenLastCalledWith({
    action: "create",
    name: "部署文档",
    prompt: "检查部署文档",
  });
  expect(sheet.getByLabelText("重构方向")).toHaveValue("d".repeat(32));
  fireEvent.change(body, { target: { value: "检查手动部署文档" } });
  fireEvent.click(sheet.getByRole("button", { name: "保存到方向" }));
  await sheet.findByText("已保存方向");
  expect(api.changeRefactoringDirection).toHaveBeenLastCalledWith({
    action: "update",
    id: "d".repeat(32),
    revision: "d".repeat(64),
    name: "部署文档",
    prompt: "检查手动部署文档",
  });
  expect(submit).not.toHaveBeenCalled();
});

it("shows direction, instructions and feedback by default and expands decisions individually", async () => {
  const instruction = "补充验证。  \r\n核对调用方。\r\n";
  const operation = { state: "complete", feedback: "", prompt: "" };
  const reports = [
    {
      body: "固定 PR 版本验证通过。\n附录：测试结果。",
      head: "review-head",
      base: "review-base",
      created_at: "2026-11-16T11:39:00Z",
      recommendation: "merge" as const,
    },
  ];
  setup([
    {
      ...record,

      operations: [
        {
          ...operation,
          id: "first",
          workflow: "generate",
          purpose: "first",
          decision_kind: "execute",
          decision_summary: "选定一个切片",
          prompt: "首轮选题指令",
        },
        {
          ...operation,
          id: "implement",
          workflow: "execute",
          purpose: "implement",
          prompt: "最初的实施指令",
          feedback: "原始回执",
          run_id: "run-implement",
        },
        {
          ...operation,
          id: "audit-repair",
          workflow: "generate",
          purpose: "audit",
          decision_kind: "execute",
          decision_summary: "缺少可核对的测试证据",
          prompt: "按随附材料判断。",
        },
        {
          ...operation,
          id: "repair",
          workflow: "execute",
          purpose: "repair",
          prompt: instruction,
          feedback: "补证回执原文",
          run_id: "run-repair",
          result: { summary: "补证完成" },
        },
        {
          ...operation,
          id: "audit",
          workflow: "generate",
          purpose: "audit",
          decision_summary: "审计通过",
          feedback: "审计结论全文",
        },
        {
          ...operation,
          id: "report",
          workflow: "generate",
          purpose: "report",
          prompt: "合入评审指令",
          report_index: 0,
        },
      ],
      reports,
      report_stale: true,
    },
  ]);
  const detail = within(
    await screen.findByRole("region", { name: "重构任务详情" }),
  );
  expect(detail.queryByText(/重构轮次/)).not.toBeInTheDocument();
  expect(detail.getByText("Runner 执行 2 次")).toBeVisible();
  const dialogue = within(detail.getByRole("region", { name: "执行对话" }));
  expect(
    dialogue.getAllByRole("heading", { level: 4 }).map((h) => h.textContent),
  ).toEqual([
    "维护方向",
    "SteerAgent · 实施",
    "Runner · 实施 · 执行反馈",
    "SteerAgent · 成果审计 → 修复或补证",
    "Runner · 修复或补证 · 执行反馈",
    "SteerAgent · 成果审计",
    "SteerAgent · PR 合入评审",
  ]);
  for (const preview of [
    "原始回执",
    "缺少可核对的测试证据",
    "补证回执原文",
    "审计通过",
    "建议合入 · 此评审已过期或正在复核",
  ]) {
    expect(dialogue.getByText(preview, { exact: true })).toBeVisible();
  }
  expect(dialogue.getByRole("heading", { name: "维护方向" })).toBeVisible();
  const direction = dialogue.getByText(/核实并更新过期文档/);
  expect(direction).toBeVisible();
  expect(direction.textContent).toBe(builtIn.prompt);
  const [implement, , repair, response, audit, review] = dialogue
    .getAllByRole("listitem")
    .map((message) => within(message));
  expect(repair.getByText("执行指令")).toBeVisible();
  expect(repair.getByText(/核对调用方/)).toBeVisible();
  expect(repair.getByText(/核对调用方/).textContent).toBe(instruction);
  expect(repair.getByText("按随附材料判断。")).not.toBeVisible();
  fireEvent.click(repair.getByText("决策指令"));
  expect(repair.getByText("按随附材料判断。")).toBeVisible();
  expect(implement.getByText("首轮选题指令")).not.toBeVisible();
  expect(response.getByText("补证回执原文")).toBeVisible();
  expect(response.getByText("run-repair")).not.toBeVisible();
  fireEvent.click(response.getByText("运行回执"));
  expect(response.getByText("run-repair")).toBeVisible();
  fireEvent.click(audit.getByText("展开结论"));
  expect(audit.getByText("审计结论全文")).toBeVisible();
  expect(review.getByText("review-head")).not.toBeVisible();
  fireEvent.click(review.getByText("展开结论"));
  expect(review.getByText("review-head")).toBeVisible();
  expect(review.getByText("合入评审指令")).not.toBeVisible();
  fireEvent.click(review.getByText("决策指令"));
  expect(review.getByText("合入评审指令")).toBeVisible();
  expect(detail.getByText(record.question!.prompt)).toBeVisible();
});

it("shows pending execution without adding to the Runner count", async () => {
  setup([
    {
      ...record,
      operations: [
        {
          id: "pending",
          purpose: "implement",
          workflow: "execute",

          state: "pending",
          prompt: "实施指令",
          feedback: "",
        },
      ],
    },
  ]);
  expect(await screen.findByText("待提交")).toBeVisible();
  expect(screen.getByText("Runner 执行 0 次")).toBeVisible();
});

it("shows the maintenance direction before SteerAgent generates an execution instruction", async () => {
  setup([{ ...record, phase: "first", question: undefined }]);
  expect(await screen.findByText("等待 SteerAgent 生成执行指令")).toBeVisible();
  expect(screen.getByText(/核实并更新过期文档/)).toBeVisible();
});

it("requires explicit confirmation of a changed qualification and blocks stale cached data", async () => {
  const readiness = vi.fn(async () => ready);
  const { submit, client } = setup([], readiness);
  const sheet = openSheet();
  const start = sheet.getByRole("button", { name: "启动重构任务" });
  await waitFor(() => expect(start).toBeEnabled());
  const refresh = () =>
    act(async () =>
      client.invalidateQueries({
        queryKey: [
          "home",
          identity.project_id,
          identity.instance_id,
          "refactoring",
          "readiness",
        ],
      }),
    );
  readiness.mockResolvedValue({
    ...ready,
    environments: [{ ...ready.environments[0], current_revision: 3 }],
  });
  await refresh();
  const confirm = await sheet.findByRole("button", { name: "确认当前选择" });
  expect(start).toBeDisabled();
  fireEvent.click(confirm);
  expect(start).toBeEnabled();
  readiness.mockRejectedValue(new Error("offline"));
  await refresh();
  expect(await sheet.findByText(/运行环境状态查询失败/)).toBeInTheDocument();
  expect(start).toBeDisabled();
  fireEvent.submit(start.closest("form")!);
  expect(submit).not.toHaveBeenCalled();
});

it("keeps the startup failure on the action and can copy its input into a new draft", async () => {
  const records: MaintenanceRecord[] = [];
  const { submit } = setup(records);
  const sheet = openSheet();
  const start = sheet.getByRole("button", { name: "启动重构任务" });
  await waitFor(() => expect(start).toBeEnabled());
  fireEvent.change(sheet.getByLabelText("方向正文"), {
    target: { value: "检查共享夹具" },
  });
  submit.mockImplementationOnce(async (envelope) => {
    const actionID = envelope.request_id;
    records.push({
      ...record,
      id: actionID,
      phase: "blocked",
      startup_failed: true,
      source_commit: "",
      direction_prompt: envelope.payload.direction_prompt,

      input: envelope.payload,
      active: false,
      can_stop: false,
      can_finish: false,
      question: undefined,
      diagnostic: "环境已变化，请重新检测",
    });
    return {
      ...envelope,
      status: "applied",
      refactoring: { id: actionID, revision: 1 },
      sampled_at: "now",
    };
  });
  fireEvent.click(start);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "环境已变化，请重新检测",
  );
  expect(screen.getByText("检查共享夹具")).toBeVisible();
  const requestedCommit = screen.getByText(/申请的起始提交/);
  expect(requestedCommit).not.toBeVisible();
  fireEvent.click(screen.getByText("运行信息", { exact: true }));
  expect(requestedCommit).toHaveTextContent(ready.project.commit);
  expect(requestedCommit).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "带入新建" }));
  expect(screen.getByRole("dialog")).toBeInTheDocument();
  expect(screen.getByLabelText("方向正文")).toHaveValue("检查共享夹具");
});

it("closes immediately and keeps the current view when the start receipt arrives late", async () => {
  const records = [record];
  const { submit, client } = setup(records);
  let release: () => void = () => undefined;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  submit.mockImplementationOnce(async (envelope) => {
    await held;
    records.push({
      ...record,
      id: envelope.request_id,
      phase: "checking_start",
      direction_name: custom.name,
      slot: false,

      elapsed_seconds: 0,
      question: undefined,
    });
    return {
      ...envelope,
      status: "applied",
      refactoring: { id: envelope.request_id, revision: 1 },
      sampled_at: "now",
    };
  });
  const sheet = openSheet();
  const start = sheet.getByRole("button", { name: "启动重构任务" });
  await waitFor(() => expect(start).toBeEnabled());
  fireEvent.change(sheet.getByLabelText("重构方向"), {
    target: { value: custom.id },
  });
  fireEvent.click(start);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "进行中 1" })).toBeInTheDocument();
  expect(
    within(screen.getByRole("region", { name: "重构任务详情" })).getByText(
      "提交待确认",
    ),
  ).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: /更新过期文档.*待答复/ }));
  await screen.findByText(record.question!.prompt);
  openSheet();
  await act(async () => release());
  expect(screen.getByRole("dialog")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "关闭" }));
  expect(await screen.findByText(record.question!.prompt)).toBeInTheDocument();
  const action = await screen.findByRole("button", {
    name: /收拢共享夹具.*正在核验启动条件/,
  });
  fireEvent.click(action);
  await waitFor(() =>
    expect(
      within(screen.getByRole("region", { name: "重构任务详情" })).getByText(
        "正在核验启动条件",
      ),
    ).toBeInTheDocument(),
  );
  // The list can eventually drop a completed action as server history advances.
  fireEvent.click(screen.getByRole("button", { name: /更新过期文档.*待答复/ }));
  records.pop();
  await act(async () => {
    await client.refetchQueries({
      queryKey: [
        "home",
        identity.project_id,
        identity.instance_id,
        "refactoring",
      ],
      exact: true,
    });
  });
  expect(
    await screen.findByRole("heading", { name: "进行中 0" }),
  ).toBeInTheDocument();
});

it("shows coverage exclusions and answers the original question without another chat", async () => {
  const { submit } = setup([record]);
  await screen.findByText(record.question!.prompt);
  expect(screen.getByText(/不覆盖子 Agent/)).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "启动重构任务" }),
  ).not.toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("补充说明（可选）"), {
    target: { value: "保留公共接口" },
  });
  fireEvent.click(screen.getByRole("radio", { name: "保留公共接口" }));
  fireEvent.click(screen.getByRole("button", { name: "答复并继续" }));
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0]).toMatchObject({
    action: "refactoring_answer",
    target_id: id,
    expected_version: 7,
    payload: {
      question_id: "question",
      option_id: "keep",
      note: "保留公共接口",
    },
  });
});
it("keeps known details operable when the list fails and isolates drafts by question", async () => {
  const second = {
    ...record,
    id: "d".repeat(32),
    direction_name: "另一维护方向",
    question: {
      ...record.question!,
      id: "second",
      prompt: "第二个重构任务的决定",
    },
  };
  const { client, api } = setup([record, second]);
  await screen.findByText(record.question!.prompt);
  fireEvent.change(screen.getByLabelText("补充说明（可选）"), {
    target: { value: "第一条草稿" },
  });
  fireEvent.click(screen.getByRole("button", { name: /另一维护方向/ }));
  await screen.findByText(second.question.prompt);
  expect(screen.getByLabelText("补充说明（可选）")).toHaveValue("");
  fireEvent.click(screen.getByRole("button", { name: /更新过期文档.*待答复/ }));
  await screen.findByText(record.question!.prompt);
  expect(screen.getByLabelText("补充说明（可选）")).toHaveValue("第一条草稿");
  vi.mocked(api.get).mockRejectedValueOnce(new Error("index failure"));
  await act(async () =>
    client.invalidateQueries({
      queryKey: [
        "home",
        identity.project_id,
        identity.instance_id,
        "refactoring",
      ],
      exact: true,
    }),
  );
  await screen.findByText(/重构任务列表当前待核实/);
  expect(screen.getByRole("button", { name: "立即停止" })).toBeEnabled();
});
it("links maintenance activity and receipts to the same action", () => {
  expect(
    activityDestination("p", {
      details_ref: { kind: "refactoring", target: id },
    } as Activity),
  ).toBe(`/projects/p/refactoring?action=${id}`);
  const api = new Api(identity, "token", "home");
  const envelope = api.envelope(
    "refactoring_start",
    identity.repository_identity,
    1,
    {},
  );
  expect(requestDestination(envelope)).toBe(
    `/projects/project/refactoring?action=${envelope.request_id}`,
  );
});

it("expands the saved Runner request and decision schema separately from task text", async () => {
  setup([{ ...record, operations: [{
    id: "decision", purpose: "audit", workflow: "generate", state: "complete", prompt: "决策正文", feedback: "", decision_summary: "需要补证",
    sent_request: { instruction: "保存的决策正文", materials: [{kind: "file", path: "maintenance/context.json"}], output_schema: {type: "object"} },
  }, {
    id: "implementation", purpose: "repair", workflow: "execute", state: "complete", prompt: "补充验证", feedback: "已验证并提交。",
    sent_request: {instruction: "固定执行边界\n\n补充验证"},
  }] }]);
  const dialogue = within(await screen.findByRole("region", {name: "执行对话"}));
  expect(dialogue.getByText("已验证并提交。")).toBeVisible();
  expect(dialogue.getByText("保存的决策正文")).not.toBeVisible();
  fireEvent.click(dialogue.getByText("决策指令"));
  const requests = dialogue.getAllByText("发送给 Runner 的请求");
  fireEvent.click(requests[0]);
  expect(dialogue.getByText("保存的决策正文")).toBeVisible();
  expect(dialogue.getByText(/maintenance\/context.json/)).toBeVisible();
  fireEvent.click(dialogue.getByText("输出约束"));
  expect(dialogue.getByText(/"type": "object"/)).toBeVisible();
  fireEvent.click(requests[1]);
  expect(dialogue.getByText(/固定执行边界/)).toBeVisible();
});
