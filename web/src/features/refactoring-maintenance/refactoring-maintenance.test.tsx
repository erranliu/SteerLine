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
  id: "simplify",
  name: "简化设计",
  prompt: "# 简化设计\n\n简化不必要的抽象与控制流。\n",
  revision: "b".repeat(64),
};
const custom = {
  id: "custom:fixtures",
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
    direction: "simplify",
    requirements: "",
    profile: "local",
    strategy: "cautious",
    budget: { rounds: 1 },
  },
  round: 1,
  active: true,
  source_commit: "b".repeat(40),
  elapsed_seconds: 65,
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
  vi.spyOn(api, "get").mockImplementation(async (path: string) => {
    if (path === "/refactoring/directions")
      return { directions: [builtIn, custom] };
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
          round: 1,
          workflow: "generate",
          state: "complete",
          feedback: "决策反馈正文",
          prompt: "决策提示词正文",
          run_id: "decision-run",
        },
        {
          id: "execution-operation",
          purpose: "implement",
          round: 1,
          workflow: "execute",
          state: "submitted",
          feedback: "",
          prompt: "执行提示词正文",
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
          round: 1,
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
      "repo · 重构任务 · 简化设计",
      `请求 / 重构任务 ID：${id}（本次提交及结果）`,
      "状态：Runner 执行中 · 第 1 轮",
      "运行环境：本机 Codex",
      `起始提交：${record.source_commit}`,
      "",
      "第 1 轮 · 首轮选题 · 已完成",
      "操作 ID：decision-operation",
      "Runner Run ID：decision-run",
      "",
      "第 1 轮 · 实施 · 已提交",
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
    "状态：已阻断 · 第 1 轮",
    `起始提交：${record.source_commit}`,
    "诊断：运行环境不可用",
    "",
    "第 1 轮 · 实施 · 已完成",
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
  expect(sheet.getByLabelText("重构轮次")).toHaveValue(1);
  fireEvent.click(start);
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0]).toMatchObject({
    action: "refactoring_start",
    target_id: identity.repository_identity,
    expected_version: 1,
    payload: {
      direction: builtIn.id,
      direction_revision: builtIn.revision,
      strategy: "cautious",
      profile: local.id,
      profile_version: local.version,
      environment_key: key,
      environment_revision: 2,
      base_commit: ready.project.commit,
      budget: { rounds: 1 },
    },
  });
});

it("keeps the full direction preview and drafts when the sheet closes", async () => {
  setup();
  const sheet = openSheet();
  const preview = await sheet.findByLabelText("方向提示词");
  expect(
    sheet.getByRole("heading", { name: "一级 · 决策提示词" }),
  ).toBeVisible();
  expect(
    sheet.getByRole("heading", { name: "二级 · 执行提示词" }),
  ).toBeVisible();
  expect(
    sheet.getByText("启动后由 SteerAgent 生成，交给 Runner 执行。"),
  ).toBeVisible();
  expect(preview.textContent).toBe(builtIn.prompt);
  fireEvent.change(sheet.getByLabelText("重构方向"), {
    target: { value: custom.id },
  });
  expect(preview.textContent).toBe(custom.prompt);
  fireEvent.change(sheet.getByLabelText("补充要求"), {
    target: { value: "先检查共享夹具" },
  });
  fireEvent.click(sheet.getByRole("button", { name: "关闭" }));
  const reopened = openSheet();
  expect(reopened.getByLabelText("补充要求")).toHaveValue("先检查共享夹具");
  expect(reopened.getByLabelText("方向提示词").textContent).toBe(custom.prompt);
});

it("shows the execution dialogue in order and expands each message and its own decision prompt", async () => {
  const instruction = "补充验证。  \r\n核对调用方。\r\n";
  const operation = { round: 1, state: "complete", feedback: "", prompt: "" };
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
      round: 1,
      input: { ...record.input, requirements: "保持原接口。" },
      operations: [
        {
          ...operation,
          id: "first",
          workflow: "generate",
          purpose: "first",
          decision_kind: "execute",
          decision_summary: "选定一个切片",
          decision_rules: { rules: "首轮规则" },
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
          decision_rules: { rules: "审计规则" },
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
          decision_rules: { rules: "合入评审规则" },
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
  expect(detail.getByText("重构轮次 1", { exact: true })).toBeVisible();
  expect(detail.getByText("Runner 执行 2 次")).toBeVisible();
  const dialogue = within(detail.getByRole("region", { name: "执行对话" }));
  expect(
    dialogue.getAllByRole("heading", { level: 4 }).map((h) => h.textContent),
  ).toEqual([
    "SteerAgent · 实施",
    "Runner · 实施反馈",
    "SteerAgent · 成果审计 → 修复或补证",
    "Runner · 修复或补证反馈",
    "SteerAgent · 成果审计",
    "SteerAgent · PR 合入评审",
  ]);
  for (const preview of [
    "回执未通过核验",
    "缺少可核对的测试证据",
    "补证完成",
    "审计通过",
    "建议合入 · 此评审已过期或正在复核",
  ]) {
    expect(dialogue.getByText(preview, { exact: true })).toBeVisible();
  }
  const [implement, , repair, response, audit, review] = dialogue
    .getAllByRole("listitem")
    .map((message) => within(message));
  expect(repair.getByText(/核对调用方/)).not.toBeVisible();
  fireEvent.click(repair.getByText("展开指令"));
  expect(repair.getByText(/核对调用方/).textContent).toBe(instruction);
  fireEvent.click(repair.getByText("一级提示词"));
  expect(repair.getByText("审计规则")).toBeVisible();
  expect(repair.getByText("保持原接口。")).toBeVisible();
  expect(implement.getByText("首轮规则")).not.toBeVisible();
  expect(response.getByText("补证回执原文")).not.toBeVisible();
  fireEvent.click(response.getByText("展开回执"));
  expect(response.getByText("补证回执原文")).toBeVisible();
  expect(response.getByText("run-repair")).toBeVisible();
  fireEvent.click(audit.getByText("展开结论"));
  expect(audit.getByText("审计结论全文")).toBeVisible();
  expect(review.getByText("review-head")).not.toBeVisible();
  fireEvent.click(review.getByText("展开结论"));
  expect(review.getByText("review-head")).toBeVisible();
  fireEvent.click(review.getByText("一级提示词"));
  expect(review.getByText("合入评审规则")).toBeVisible();
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
          round: 1,
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

it("shows a waiting state before SteerAgent generates an execution prompt", async () => {
  setup([{ ...record, phase: "first", question: undefined }]);
  expect(
    await screen.findByText("等待 SteerAgent 生成执行提示词"),
  ).toBeVisible();
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
  fireEvent.change(sheet.getByLabelText("补充要求"), {
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
      direction_prompt: undefined,
      round: 0,
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
  expect(screen.getByText("检查共享夹具")).not.toBeVisible();
  fireEvent.click(screen.getByText("维护要求", { exact: true }));
  expect(screen.getByText("检查共享夹具")).toBeVisible();
  const requestedCommit = screen.getByText(/申请的起始提交/);
  expect(requestedCommit).not.toBeVisible();
  fireEvent.click(screen.getByText("运行信息", { exact: true }));
  expect(requestedCommit).toHaveTextContent(ready.project.commit);
  expect(requestedCommit).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "带入新建" }));
  expect(screen.getByRole("dialog")).toBeInTheDocument();
  expect(screen.getByLabelText("补充要求")).toHaveValue("检查共享夹具");
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
      round: 0,
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
  fireEvent.click(screen.getByRole("button", { name: /简化设计.*待答复/ }));
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
  fireEvent.click(screen.getByRole("button", { name: /简化设计.*待答复/ }));
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
  fireEvent.click(screen.getByRole("button", { name: /简化设计.*待答复/ }));
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
