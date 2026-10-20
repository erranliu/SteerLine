import { afterEach, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  render,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { WorkspaceContext, type Workspace } from "../../app/context";
import type { Candidate, Envelope, Receipt } from "../../shared/api/types";
import { Api } from "../../shared/api/client";
import { projectQueries } from "../../shared/api/queries";
import {
  identity,
  mountFeature,
  run,
  tasks,
  testClient,
  snapshot,
} from "../../test/workspace";
import { Runs } from "./runs";
import { RunnerWorkflow } from "./runner-workflow";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

it("shows frozen parameters and public audit sources without inventing a pass", () => {
  const candidate = {
    ...run,
    execution_profile: {
      id: "original",
      name: "原冻结方案",
      host: "local",
      agent: "codex",
      unavailable_reason: "",
      details: ["请求执行模型：execution-model · medium"],
    },
    runner_workflow: { id: "development-audited", version: 1 },
    requested_model: "audit-model",
    requested_reasoning_effort: "high",
    materials: [
      {
        kind: "git",
        path: "spec.md",
        commit: "fixed-commit",
        digest: "fixed-hash",
      },
    ],
    audit: {
      model: "audit-model",
      reasoning_effort: "high",
      repair_count: 1,
      results: [
        {
          step: "audit.code",
          source_report_command_id: "original-report",
          command_id: "audit-command",
          turn_id: "original-turn",
          failure: "原结果未核实",
          findings: [
            { severity: "P2", message: "审计发现", path: "file.go", line: 3 },
          ],
        },
      ],
    },
  };
  const view = render(<RunnerWorkflow run={candidate} />);
  expect(screen.getByText("冻结方案 · 原冻结方案")).toBeVisible();
  expect(screen.getByText("Workflow · development-audited/1")).toBeVisible();
  expect(
    screen.getByText("当前轮次请求模型 · audit-model · high"),
  ).toBeVisible();
  expect(screen.getByText("audit.code · 尚无结论")).toBeVisible();
  expect(screen.getByText("原结果未核实")).toBeVisible();
  fireEvent.click(screen.getByText("结果来源"));
  expect(
    screen.getByText(
      /报告 original-report · 命令 audit-command · 轮次 original-turn/,
    ),
  ).toBeVisible();
  view.rerender(
    <RunnerWorkflow
      run={{
        ...candidate,
        audit: {
          ...candidate.audit,
          results: [
            { ...candidate.audit.results[0], verdict: "pass", failure: "" },
          ],
        },
      }}
    />,
  );
  expect(screen.getByText("audit.code · pass")).toBeVisible();
  view.rerender(
    <RunnerWorkflow
      run={{
        ...candidate,
        audit: undefined,
        runner_workflow: { id: "development", version: 1 },
      }}
    />,
  );
  expect(screen.queryByText("Runner 审计观察")).not.toBeInTheDocument();
  expect(screen.queryByText("audit.code · pass")).not.toBeInTheDocument();
});

it("keeps cached output visible while refreshing and stops polling the hidden output tab", async () => {
  vi.useFakeTimers();
  const client = testClient();
  const api = new Api(identity, "csrf", "home");
  const options = projectQueries(api);
  client.setQueryData(options.tasks().queryKey, tasks);
  client.setQueryData(options.task("1").queryKey, tasks);
  client.setQueryData(options.presentation().queryKey, {
    presentation: { profiles: [], default_profile_id: "" },
  });
  const activity = {
    page: {
      diagnostics: [],
      activity: [
        {
          seq: 1,
          ts: "2026-10-15T00:00:00Z",
          title: "已缓存活动",
          kind: "activity",
          detail: "",
          status: "",
        },
      ],
    },
  };
  const final = {
    workflow_id: run.run_id,
    execution_id: run.execution_id,
    attempt_id: run.attempt_id,
    bodies: [
      {
        item_key: "body",
        revision: 1,
        text: "已缓存正文",
        complete: true,
        completeness: "complete",
      },
    ],
    page: { events: [], has_more: false },
  };
  client.setQueryData(options.activity(run).queryKey, activity);
  client.setQueryData(options.final(run).queryKey, final);
  const get = vi.spyOn(api, "get").mockImplementation(
    async (path) =>
      (path.includes("/activity?")
        ? activity
        : path.includes("/last-message?")
          ? {
              run_id: run.execution_id,
              attempt_id: run.attempt_id,
              message: null,
            }
          : tasks) as never,
  );
  let resolve!: (value: unknown) => void;
  const fetchFinal = vi.spyOn(api, "final").mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done as typeof resolve;
      }) as never,
  );
  mountFeature(<Runs />, api, client);
  await act(() => vi.advanceTimersByTimeAsync(2600));
  expect(screen.getByText("已缓存活动")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "最终回复" }));
  await act(() => vi.advanceTimersByTimeAsync(0));
  expect(fetchFinal).toHaveBeenCalledTimes(1);
  expect(screen.getByText("已缓存正文")).toBeVisible();
  expect(screen.queryByText("正在读取最终回复…")).not.toBeInTheDocument();
  const activityCalls = get.mock.calls.filter(([path]) =>
    path.includes("/activity?"),
  ).length;
  await act(() => vi.advanceTimersByTimeAsync(5200));
  expect(
    get.mock.calls.filter(([path]) => path.includes("/activity?")),
  ).toHaveLength(activityCalls);
  await act(async () => resolve(final));
  fireEvent.click(screen.getByRole("button", { name: "近期活动" }));
  await act(() => vi.advanceTimersByTimeAsync(2600));
  expect(
    get.mock.calls.filter(([path]) => path.includes("/activity?")).length,
  ).toBeGreaterThan(activityCalls);
  expect(fetchFinal).toHaveBeenCalledTimes(1);
  client.clear();
});

it("shows a loading state rather than an empty task list until the first response arrives", () => {
  const client = testClient();
  const api = new Api(identity, "csrf", "home");
  vi.spyOn(api, "get").mockImplementation(() => new Promise(() => {}) as never);
  mountFeature(<Runs />, api, client);
  expect(screen.getByText("正在读取正式任务…")).toBeVisible();
  expect(screen.queryByText("还没有正式任务")).not.toBeInTheDocument();
  client.clear();
});

it("shows recent changes newest first and keeps each event's workflow context", async () => {
  const client = testClient();
  const api = new Api(identity, "csrf", "home");
  const options = projectQueries(api);
  client.setQueryData(options.tasks().queryKey, tasks);
  client.setQueryData(options.task("1").queryKey, tasks);
  client.setQueryData(options.presentation().queryKey, {
    presentation: { profiles: [], default_profile_id: "" },
  });
  const activity = {
    page: {
      diagnostics: [],
      activity: [
        {
          seq: 1,
          ts: "2026-10-15T00:00:00Z",
          kind: "worker_state",
          status: "blocked",
          title: "状态切换：执行中 → 已阻塞",
          detail: "GitHub HTTP 401 Requires authentication",
          meta: {
            from: {
              workflow_stage: "implement.report",
              workflow_status: "running",
            },
            to: {
              workflow_stage: "implement.report",
              workflow_status: "blocked",
            },
          },
        },
        {
          seq: 2,
          ts: "2026-10-15T00:00:01Z",
          kind: "runner_event",
          status: "operation_committed",
          title: "operation_committed",
          detail: "",
        },
        {
          seq: 3,
          ts: "2026-10-15T00:00:00Z",
          kind: "worker_state",
          status: "running",
          title: "Workflow 切换：implement.report → audit.execute（执行中）",
          detail: "",
          meta: {
            from: {
              workflow_stage: "implement.report",
              workflow_status: "blocked",
            },
            to: { workflow_stage: "audit.execute", workflow_status: "running" },
          },
        },
        {
          seq: 4,
          ts: "2026-10-15T00:00:01Z",
          kind: "runner_event",
          status: "operation_accepted",
          title: "operation_accepted",
          detail: "",
        },
      ],
    },
  };
  client.setQueryData(options.activity(run).queryKey, activity);
  vi.spyOn(api, "get").mockImplementation(
    async (path) =>
      (path.includes("/activity?")
        ? activity
        : path.includes("/last-message?")
          ? {
              run_id: run.execution_id,
              attempt_id: run.attempt_id,
              message: null,
            }
          : tasks) as never,
  );
  mountFeature(<Runs />, api, client);
  expect(screen.getByText("状态切换：执行中 → 已阻塞")).toBeVisible();
  const items = document.querySelectorAll(".activity-item");
  expect(items[0]).toHaveTextContent(
    "Workflow 切换：implement.report → audit.execute（执行中）",
  );
  expect(items[1]).toHaveTextContent("状态切换：执行中 → 已阻塞");
  expect(
    within(items[1] as HTMLElement).getByText("implement.report"),
  ).toBeVisible();
  expect(
    within(items[0] as HTMLElement).queryByText(/状态切换/),
  ).not.toBeInTheDocument();
  expect(screen.getByText(/GitHub HTTP 401/)).toBeVisible();
  expect(screen.getByText(/operation_committed/)).not.toBeVisible();
  fireEvent.click(screen.getByText("内部执行事件"));
  expect(screen.getByText(/operation_committed/)).toBeVisible();
  const internal = screen.getByText("内部执行事件").closest("details")!;
  expect(within(internal).getAllByRole("paragraph")[0]).toHaveTextContent(
    "operation_accepted",
  );
  expect(activity.page.activity.map((event) => event.seq)).toEqual([
    1, 2, 3, 4,
  ]);
  act(() => {
    client.setQueryData(options.activity(run).queryKey, {
      page: {
        ...activity.page,
        activity: [
          ...activity.page.activity,
          {
            seq: 5,
            ts: "2026-10-15T00:00:02Z",
            kind: "worker_state",
            status: "blocked",
            title: "诊断信息更新",
            detail: "最新诊断",
            meta: {
              from: {
                workflow_stage: "audit.execute",
                workflow_status: "blocked",
              },
              to: {
                workflow_stage: "audit.execute",
                workflow_status: "blocked",
              },
            },
          },
        ],
      },
    });
  });
  await waitFor(() =>
    expect(document.querySelectorAll(".activity-item")[0]).toHaveTextContent(
      "最新诊断",
    ),
  );
  expect(
    within(
      document.querySelectorAll(".activity-item")[0] as HTMLElement,
    ).getByText("audit.execute"),
  ).toBeVisible();
  client.clear();
});

function stopWorkspace() {
  const api = new Api(identity, "csrf", "stop-home");
  const client = testClient();
  const options = projectQueries(api);
  client.setQueryData(options.tasks().queryKey, tasks);
  client.setQueryData(options.task("1").queryKey, tasks);
  client.setQueryData(options.presentation().queryKey, {
    presentation: { profiles: [], default_profile_id: "" },
  });
  vi.spyOn(api, "get").mockImplementation(
    async (path) =>
      (path.includes("/activity?")
        ? { page: { activity: [], diagnostics: [] } }
        : path.includes("/last-message?")
          ? { run_id: "execution", attempt_id: "attempt", message: null }
          : tasks) as never,
  );
  const runs = [1, 2, 3].map((number) => ({
    ...run,
    run_id: String(number).repeat(32),
    candidate_number: number,
    title: "详情候选" + number,
    control_revision: number + 3,
    execution_id: "execution-" + number,
  }));
  const state = (candidates: Candidate[]) => {
    const value = snapshot(api, candidates);
    value.connection.snapshot.page!.service.capabilities.push("stop_candidate");
    return value;
  };
  const submit = vi.fn<(envelope: Envelope) => Promise<Receipt | undefined>>();
  let workspace: Workspace = {
    api,
    identity,
    online: true,
    snapshot: state(runs),
    busy: false,
    send: async () => true,
    submit,
    requests: [],
    refresh: () => {},
  };
  const content = () => (
    <QueryClientProvider client={client}>
      <WorkspaceContext.Provider value={workspace}>
        <MemoryRouter>
          <Runs />
        </MemoryRouter>
      </WorkspaceContext.Provider>
    </QueryClientProvider>
  );
  const view = render(content());
  return {
    api,
    client,
    runs,
    state,
    submit,
    update: (overrides: Partial<Workspace>) => {
      workspace = { ...workspace, ...overrides };
      view.rerender(content());
    },
  };
}

function receipt(envelope: Envelope, status: Receipt["status"]): Receipt {
  return { ...envelope, status, sampled_at: "2026-10-17T00:00:00Z" };
}

it("stops an unknown candidate from its heading using a fixed confirmation and waits for original receipt plus authoritative snapshot", async () => {
  const w = stopWorkspace();
  const showStopped = screen.getByRole("checkbox", {
    name: "显示已停止参与候选",
  });
  expect(showStopped).not.toBeChecked();
  w.update({
    snapshot: w.state([
      {
        ...w.runs[0],
        run_id: "0".repeat(32),
        candidate_number: 4,
        participation: "abandoned",
      },
      ...w.runs.map((candidate) => ({
        ...candidate,
        workflow_status: "unknown",
      })),
    ]),
  });
  expect(
    screen.queryByRole("button", { name: /候选 C4/ }),
  ).not.toBeInTheDocument();
  expect(screen.getAllByText("3 个候选")).toHaveLength(2);
  const button = within(
    screen
      .getByRole("heading", { name: "详情候选1" })
      .closest(".candidate-heading") as HTMLElement,
  ).getByRole("button", { name: "停止参与" });
  fireEvent.click(button);
  const dialog = screen.getByRole("dialog", { name: "确认停止候选参与" });
  expect(within(dialog).getByText(/项目 project · 任务 1/)).toBeVisible();
  expect(within(dialog).getByText(/原 worker 可能继续/)).toBeVisible();
  w.submit.mockImplementation(async (envelope) =>
    receipt(envelope, "outcome_unknown"),
  );
  fireEvent.click(within(dialog).getByRole("button", { name: "确认停止" }));
  await waitFor(() => expect(w.submit).toHaveBeenCalledTimes(1));
  const envelope = w.submit.mock.calls[0][0];
  expect(envelope).toMatchObject({
    action: "stop_candidate",
    project_id: "project",
    instance_id: "instance",
    target_id: w.runs[0].run_id,
    expected_version: 4,
    expected_control_revision: 1,
    payload: { task_id: "1" },
  });
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  expect(screen.getByRole("heading", { name: "详情候选1" })).toBeVisible();
  expect(screen.getByRole("button", { name: "停止参与" })).toBeDisabled();
  const stopped = {
    ...w.runs[0],
    participation: "abandoned",
    execution_status: "running",
    candidate_stop_reason: "manual",
    candidate_stopped_by: "operator",
    decision: {
      id: "historical-question",
      version: 1,
      attempt_id: "attempt",
      status: "pending",
      request: {
        question: "已退出候选的历史问题",
        context: "",
        interaction_kind: "business_feedback",
        blocks_progress: true,
        allow_free_text: true,
        has_native_questions: false,
        options: [],
      },
    },
  };
  w.update({ snapshot: w.state([stopped, w.runs[1], w.runs[2]]) });
  expect(screen.getByRole("heading", { name: "详情候选1" })).toBeVisible();
  expect(
    screen.queryByRole("button", { name: /候选 C1/ }),
  ).not.toBeInTheDocument();
  w.update({
    requests: [
      { envelope, created_at: "now", response: receipt(envelope, "applied") },
    ],
  });
  await screen.findByRole("heading", { name: "详情候选3" });
  fireEvent.click(showStopped);
  fireEvent.click(screen.getByRole("button", { name: /候选 C1/ }));
  expect(screen.getByText(/历史只读 · 资源保留/)).toBeVisible();
  expect(screen.getByText("执行事实：running")).toBeVisible();
  expect(screen.getByText("已退出候选的历史问题")).toBeVisible();
  expect(
    screen.queryByRole("button", { name: "打开回答" }),
  ).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "停止参与" })).toBeDisabled();
  fireEvent.click(showStopped);
  expect(
    screen.queryByRole("button", { name: /候选 C1/ }),
  ).not.toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "详情候选2" })).toBeVisible();
  expect(w.submit).toHaveBeenCalledTimes(1);
  w.client.clear();
});

it("rechecks remaining participants and frozen versions while confirmation is open", () => {
  const w = stopWorkspace();
  fireEvent.click(screen.getByRole("button", { name: "停止参与" }));
  w.update({ snapshot: w.state([w.runs[0]]) });
  expect(screen.getByRole("button", { name: "确认停止" })).toBeDisabled();
  expect(
    screen.getAllByText("只剩一个参与候选，不能停止").length,
  ).toBeGreaterThan(0);
  w.update({
    snapshot: w.state([{ ...w.runs[0], control_revision: 99 }, w.runs[1]]),
  });
  expect(
    screen.getByText("候选或服务状态已变化，请取消后重新确认"),
  ).toBeVisible();
  expect(screen.getByRole("button", { name: "确认停止" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(w.submit).not.toHaveBeenCalled();
  w.client.clear();
});

it("keeps a user's later selection when the old stop completes", async () => {
  const w = stopWorkspace();
  let resolve!: (value: Receipt) => void;
  w.submit.mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  fireEvent.click(screen.getByRole("button", { name: "停止参与" }));
  fireEvent.click(screen.getByRole("button", { name: "确认停止" }));
  fireEvent.click(screen.getByRole("button", { name: /候选 C2/ }));
  const envelope = w.submit.mock.calls[0][0];
  await act(async () => resolve(receipt(envelope, "applied")));
  w.update({
    snapshot: w.state([
      { ...w.runs[0], participation: "abandoned" },
      w.runs[1],
      w.runs[2],
    ]),
  });
  expect(screen.getByRole("heading", { name: "详情候选2" })).toBeVisible();
  expect(w.submit).toHaveBeenCalledTimes(1);
  w.client.clear();
});

it("shows offline and capability reasons and preserves selection on a rejected stop", async () => {
  const w = stopWorkspace();
  w.update({ online: false });
  expect(screen.getByText("服务离线，连接后可停止参与")).toBeVisible();
  w.update({ online: true, snapshot: snapshot(w.api, w.runs) });
  expect(screen.getByText("当前服务未提供停止参与能力")).toBeVisible();
  w.update({ snapshot: w.state(w.runs) });
  w.submit.mockImplementation(async (envelope) =>
    receipt(envelope, "rejected"),
  );
  fireEvent.click(screen.getByRole("button", { name: "停止参与" }));
  fireEvent.click(screen.getByRole("button", { name: "确认停止" }));
  await waitFor(() => expect(w.submit).toHaveBeenCalledTimes(1));
  expect(screen.getByRole("heading", { name: "详情候选1" })).toBeVisible();
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "确认停止" })).toBeEnabled(),
  );
  w.client.clear();
});
