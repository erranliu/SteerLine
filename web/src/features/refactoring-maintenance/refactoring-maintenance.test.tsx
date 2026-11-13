import { afterEach, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
  question: { id: "question", prompt: "保留原公共接口还是允许调整？建议保留。", facts: ["已有公共调用"], attempts: [], options: [{ id: "keep", label: "保留公共接口", impact: "保持兼容" }], recommended_option_id: "keep", recommendation_reason: "兼容现有调用" },
  can_finish: true,
  can_stop: true,
  can_conflict: false,
  operations: [],
  reports: [],
};
const ready = {
  branch: "main",
  project: { status: "ready", commit: "b".repeat(40), blockers: [] },
  environment: "ready",
};
const indexKey = [
  "home",
  identity.project_id,
  identity.instance_id,
  "refactoring",
];
const localReadinessKey = [...indexKey, "readiness", "local"];
function setup(
  records: MaintenanceRecord[] = [],
  readiness = vi.fn(async (_path: string) => ready),
) {
  const api = new Api(identity, "token", "home");
  vi.spyOn(api, "get").mockImplementation(async (path: string) =>
    path === "/refactoring/directions"
      ? { directions: [builtIn, custom] }
      : path === "/refactoring"
        ? {
            records,
            profiles: [
              { id: "local", name: "本机 Codex" },
              { id: "second", name: "另一个配置" },
            ],
            default_profile: "local",
          }
        : path.startsWith("/refactoring/readiness")
          ? readiness(path)
          : records[0],
  );
  const submit = vi.fn(async (envelope) => ({
    ...envelope,
    status: "applied",
    refactoring: { id, revision: 8 },
    sampled_at: "now",
  }));
  const client = testClient();
  mountFeature(<RefactoringMaintenance />, api, client, true, { submit });
  return { api, submit, client };
}
it("defaults to cautious and one round, submits the original project identity", async () => {
  const { submit } = setup();
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "开始维护" })).not.toBeDisabled(),
  );
  expect(screen.getByLabelText("决策策略")).toHaveValue("cautious");
  expect(screen.getByLabelText("重构轮次")).toHaveValue(1);
  fireEvent.click(screen.getByRole("button", { name: "开始维护" }));
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0]).toMatchObject({
    action: "refactoring_start",
    target_id: identity.repository_identity,
    expected_version: 1,
    payload: {
      direction: builtIn.id,
      direction_revision: builtIn.revision,
      strategy: "cautious",
      profile: "local",
      budget: { rounds: 1 },
    },
  });
});
it("previews the full selected Markdown and submits only its identity and version with the requirements", async () => {
  const { submit } = setup();
  const preview = await screen.findByLabelText("方向提示词");
  expect(preview.textContent).toBe(builtIn.prompt);
  fireEvent.change(screen.getByLabelText("重构方向"), {
    target: { value: custom.id },
  });
  expect(preview.textContent).toBe(custom.prompt);
  fireEvent.change(screen.getByLabelText("补充要求"), {
    target: { value: "先检查共享夹具" },
  });
  const start = screen.getByRole("button", { name: "开始维护" });
  await waitFor(() => expect(start).toBeEnabled());
  fireEvent.click(start);
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0].payload).toEqual({
    direction: custom.id,
    direction_revision: custom.revision,
    requirements: "先检查共享夹具",
    strategy: "cautious",
    profile: "local",
    budget: { dollars: "", minutes: "", rounds: 1 },
  });
});
it("blocks clicking, Enter and form submission until the selected environment is ready", async () => {
  const reason = "环境未就绪：尚未准备当前项目环境";
  const readiness = vi.fn(async (path: string) =>
    path.endsWith("profile=local")
      ? { ...ready, environment: "not_ready", reason }
      : ready,
  );
  const { submit, client } = setup([], readiness);
  const start = screen.getByRole("button", { name: "开始维护" });
  await screen.findByText(reason, { exact: false });
  expect(screen.getByText(/项目基线已就绪 · main/)).toBeInTheDocument();
  expect(screen.getByLabelText("Runner 执行配置")).toHaveValue("local");
  expect(
    screen.getByRole("link", { name: "打开项目运行环境" }),
  ).toHaveAttribute(
    "href",
    `/projects/${identity.project_id}/environment?profile=local`,
  );
  expect(start).toBeDisabled();
  const user = userEvent.setup();
  await user.click(start);
  await user.click(screen.getByLabelText("重构轮次"));
  await user.keyboard("{Enter}");
  fireEvent.submit(start.closest("form")!);
  expect(submit).not.toHaveBeenCalled();

  fireEvent.change(screen.getByLabelText("Runner 执行配置"), {
    target: { value: "second" },
  });
  await waitFor(() => expect(start).toBeEnabled());
  fireEvent.change(screen.getByLabelText("Runner 执行配置"), {
    target: { value: "local" },
  });
  expect(start).toBeDisabled();
  readiness.mockResolvedValue(ready);
  await act(async () =>
    client.invalidateQueries({
      queryKey: [
        "home",
        identity.project_id,
        identity.instance_id,
        "refactoring",
        "readiness",
        "local",
      ],
    }),
  );
  await waitFor(() => expect(start).toBeEnabled());
  fireEvent.submit(start.closest("form")!);
  expect(submit.mock.calls[0][0].payload.profile).toBe("local");
});
it("keeps start enabled during a readiness refresh until the result becomes not ready", async () => {
  const readiness = vi.fn(async (_path: string) => ready);
  const { client } = setup([], readiness);
  const start = screen.getByRole("button", { name: "开始维护" });
  await waitFor(() => expect(start).toBeEnabled());

  const reason = "环境检测资格已撤销";
  let complete!: (result: typeof ready & { reason: string }) => void;
  readiness.mockReturnValueOnce(
    new Promise((resolve) => {
      complete = resolve;
    }),
  );
  act(() => {
    void client.invalidateQueries({ queryKey: localReadinessKey });
  });
  expect(client.getQueryState(localReadinessKey)?.fetchStatus).toBe("fetching");
  fireEvent.change(screen.getByLabelText("重构方向"), {
    target: { value: custom.id },
  });
  expect(screen.getByLabelText("方向提示词")).toHaveTextContent("收拢共享夹具");
  expect(start).toBeEnabled();

  complete({ ...ready, environment: "not_ready", reason });
  await screen.findByText(reason, { exact: false });
  expect(start).toBeDisabled();
});
it("waits for readiness on first entry and on switching back to a cached ready profile", async () => {
  let complete!: (result: typeof ready) => void;
  const pending = new Promise<typeof ready>((resolve) => {
    complete = resolve;
  });
  const readiness = vi.fn(async (path: string) =>
    path.endsWith("profile=local") ? pending : ready,
  );
  const { client } = setup([], readiness);
  const start = screen.getByRole("button", { name: "开始维护" });
  await waitFor(() =>
    expect(readiness).toHaveBeenCalledWith(
      "/refactoring/readiness?profile=local",
    ),
  );
  expect(start).toBeDisabled();
  complete(ready);
  await waitFor(() => expect(start).toBeEnabled());

  const profile = screen.getByLabelText("Runner 执行配置");
  fireEvent.change(profile, { target: { value: "second" } });
  await waitFor(() => expect(start).toBeEnabled());
  readiness.mockReturnValueOnce(new Promise(() => {}));
  fireEvent.change(profile, { target: { value: "local" } });
  expect(client.getQueryData(localReadinessKey)).toEqual(ready);
  expect(client.getQueryState(localReadinessKey)?.fetchStatus).toBe("fetching");
  expect(start).toBeDisabled();
});
it("blocks a previously ready profile when its latest query fails", async () => {
  const readiness = vi.fn(async (_path: string) => ready);
  const { submit, client } = setup([], readiness);
  const start = screen.getByRole("button", { name: "开始维护" });
  await waitFor(() => expect(start).toBeEnabled());
  readiness.mockRejectedValue(new Error("offline"));
  await act(async () =>
    client.invalidateQueries({
      queryKey: [
        "home",
        identity.project_id,
        identity.instance_id,
        "refactoring",
        "readiness",
        "local",
      ],
    }),
  );
  expect(await screen.findByText(/运行环境状态查询失败/)).toBeInTheDocument();
  expect(start).toBeDisabled();
  fireEvent.submit(start.closest("form")!);
  expect(submit).not.toHaveBeenCalled();
});
it("refreshes the selected qualification after a rejected start and keeps the form", async () => {
  const readiness = vi.fn(async (_path: string) => ({ ...ready, reason: "" }));
  const { submit } = setup([], readiness);
  const start = screen.getByRole("button", { name: "开始维护" });
  await waitFor(() => expect(start).toBeEnabled());
  fireEvent.change(screen.getByLabelText("补充要求"), {
    target: { value: "检查共享夹具" },
  });
  submit.mockImplementationOnce(async (envelope) => {
    readiness.mockResolvedValue({
      ...ready,
      environment: "not_ready",
      reason: "环境检测资格已撤销",
    });
    return {
      ...envelope,
      status: "rejected",
      diagnostic: "环境已变化，请重新检测",
      sampled_at: "now",
    };
  });
  fireEvent.click(start);
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "环境已变化，请重新检测",
  );
  await screen.findByText(/环境检测资格已撤销/);
  expect(start).toBeDisabled();
  expect(screen.getByLabelText("补充要求")).toHaveValue("检查共享夹具");
});
it("keeps the preview usable during a catalogue refresh, then waits for a newer version if start is rejected", async () => {
  const { api, client, submit } = setup();
  const start = screen.getByRole("button", { name: "开始维护" });
  await waitFor(() => expect(start).toBeEnabled());
  const read = vi.mocked(api.get).getMockImplementation()!;
  let complete!: (value: { directions: (typeof builtIn)[] }) => void;
  const refreshing = new Promise<{ directions: (typeof builtIn)[] }>(
    (resolve) => {
      complete = resolve;
    },
  );
  vi.mocked(api.get).mockImplementation((path, signal) =>
    path === "/refactoring/directions" ? refreshing : read(path, signal),
  );
  const catalogueKey = [...indexKey, "directions"];
  act(() => {
    void client.invalidateQueries({ queryKey: catalogueKey });
  });
  expect(client.getQueryState(catalogueKey)?.fetchStatus).toBe("fetching");
  fireEvent.change(screen.getByLabelText("补充要求"), {
    target: { value: "保留这段要求" },
  });
  expect(start).toBeEnabled();
  submit.mockImplementationOnce(async (envelope) => ({
    ...envelope,
    status: "rejected",
    code: "refactoring_direction_changed",
    diagnostic: "方向提示词已更新",
    sampled_at: "now",
  }));
  fireEvent.click(start);
  expect(submit.mock.calls[0][0].payload.direction_revision).toBe(
    builtIn.revision,
  );
  await screen.findByText("请等待新版方向提示词，查看后再开始维护");
  expect(start).toBeDisabled();
  expect(screen.getByLabelText("补充要求")).toHaveValue("保留这段要求");

  const updated = {
    ...builtIn,
    revision: "d".repeat(64),
    prompt: "新版方向正文",
  };
  complete({ directions: [updated, custom] });
  await waitFor(() => expect(start).toBeEnabled());
  expect(screen.getByLabelText("方向提示词")).toHaveTextContent(updated.prompt);
  expect(screen.getByRole("alert")).toHaveTextContent(
    "上次提交结果：方向提示词已更新",
  );
  fireEvent.click(start);
  expect(submit.mock.calls[1][0].payload.direction_revision).toBe(
    updated.revision,
  );
});
it("explains why a failed maintenance index refresh prevents a start", async () => {
  const { api, client, submit } = setup();
  const start = screen.getByRole("button", { name: "开始维护" });
  await waitFor(() => expect(start).toBeEnabled());
  const read = vi.mocked(api.get).getMockImplementation()!;
  vi.mocked(api.get).mockImplementation(async (path, signal) => {
    if (path === "/refactoring") throw new Error("暂时无法读取列表");
    return read(path, signal);
  });
  await act(() => client.refetchQueries({ queryKey: indexKey, exact: true }));
  await screen.findByText("暂时无法确认是否已有维护行动");
  expect(start).toHaveAccessibleDescription("暂时无法确认是否已有维护行动");
  expect(start).toBeDisabled();
  expect(screen.getByText("所选运行环境已就绪")).toBeVisible();
  fireEvent.submit(start.closest("form")!);
  expect(submit).not.toHaveBeenCalled();
});
it("shows coverage exclusions and answers the original question without another chat", async () => {
  const { submit } = setup([record]);
  await screen.findByText(record.question!.prompt);
  expect(screen.getByText(/不覆盖子 Agent/)).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "开始维护" }),
  ).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("radio", { name: "保留公共接口" }));
  fireEvent.change(screen.getByLabelText("补充说明（可选）"), {
    target: { value: "维持兼容" },
  });
  fireEvent.click(screen.getByRole("button", { name: "答复并继续" }));
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0]).toMatchObject({
    action: "refactoring_answer",
    target_id: id,
    expected_version: 7,
    payload: { question_id: "question", option_id: "keep", note: "维持兼容" },
  });
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
