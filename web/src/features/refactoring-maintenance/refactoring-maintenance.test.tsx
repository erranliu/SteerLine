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
import {
  RefactoringMaintenance,
  type MaintenanceRecord,
} from "./refactoring-maintenance";
import { activityDestination, requestDestination } from "../activities/model";
import type { Activity } from "../activities/model";
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
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
  fireEvent.click(screen.getByRole("button", { name: "新建维护行动" }));
  return within(screen.getByRole("dialog"));
}

it("starts at full capacity using a versioned ready environment and the default plan", async () => {
  const { submit } = setup();
  const sheet = openSheet();
  const start = sheet.getByRole("button", { name: "开始维护" });
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

it("requires explicit confirmation of a changed qualification and blocks stale cached data", async () => {
  const readiness = vi.fn(async () => ready);
  const { submit, client } = setup([], readiness);
  const sheet = openSheet();
  const start = sheet.getByRole("button", { name: "开始维护" });
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

it("keeps draft and diagnosis when a previously ready environment is revoked", async () => {
  const readiness = vi.fn(async () => ready);
  const { submit } = setup([], readiness);
  const sheet = openSheet();
  const start = sheet.getByRole("button", { name: "开始维护" });
  await waitFor(() => expect(start).toBeEnabled());
  fireEvent.change(sheet.getByLabelText("补充要求"), {
    target: { value: "检查共享夹具" },
  });
  submit.mockImplementationOnce(async (envelope) => {
    readiness.mockResolvedValue({
      ...ready,
      environments: [
        {
          ...ready.environments[0],
          status: "not_ready",
          reason: "环境检测资格已撤销",
        },
      ],
    });
    return {
      ...envelope,
      status: "rejected",
      diagnostic: "环境已变化，请重新检测",
      sampled_at: "now",
    };
  });
  fireEvent.click(start);
  expect(await sheet.findByRole("alert")).toHaveTextContent(
    "环境已变化，请重新检测",
  );
  expect(start).toBeDisabled();
  expect(sheet.getByLabelText("补充要求")).toHaveValue("检查共享夹具");
});

it("offers the action link when a receipt arrives after the sheet closes", async () => {
  const { submit } = setup();
  let release: () => void = () => undefined;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  submit.mockImplementationOnce(async (envelope) => {
    await held;
    return {
      ...envelope,
      status: "applied",
      refactoring: { id, revision: 1 },
      sampled_at: "now",
    };
  });
  const sheet = openSheet();
  const start = sheet.getByRole("button", { name: "开始维护" });
  await waitFor(() => expect(start).toBeEnabled());
  fireEvent.click(start);
  fireEvent.click(sheet.getByRole("button", { name: "关闭" }));
  await act(async () => release());
  expect(await screen.findByRole("link", { name: "查看行动" })).toHaveAttribute(
    "href",
    expect.stringContaining(id),
  );
  expect(
    screen.queryByRole("region", { name: "维护详情" }),
  ).not.toBeInTheDocument();
});

it("shows coverage exclusions and answers the original question without another chat", async () => {
  const { submit } = setup([record]);
  await screen.findByText(record.question!.prompt);
  expect(screen.getByText(/不覆盖子 Agent/)).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "开始维护" }),
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
    question: { ...record.question!, id: "second", prompt: "第二条行动的决定" },
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
  await screen.findByText(/行动列表当前待核实/);
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
