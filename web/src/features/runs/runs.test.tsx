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
import type {
  Candidate,
  Envelope,
  Receipt,
  TaskPage,
} from "../../shared/api/types";
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

function stopWorkspace(
  initialRuns?: Candidate[],
  taskPage: TaskPage = tasks,
  initialEntry = "/",
) {
  const api = new Api(identity, "csrf", "stop-home");
  const client = testClient();
  const options = projectQueries(api);
  client.setQueryData(options.tasks().queryKey, taskPage);
  for (const task of taskPage.tasks)
    client.setQueryData(options.task(task.task_id).queryKey, taskPage);
  client.setQueryData(options.presentation().queryKey, {
    presentation: { profiles: [], default_profile_id: "" },
  });
  vi.spyOn(api, "get").mockImplementation(
    async (path) =>
      (path.includes("/activity?")
        ? { page: { activity: [], diagnostics: [] } }
        : path.includes("/last-message?")
          ? { run_id: "execution", attempt_id: "attempt", message: null }
          : taskPage) as never,
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
    snapshot: state(initialRuns ?? runs),
    busy: false,
    send: async () => true,
    submit,
    requests: [],
    refresh: () => {},
  };
  const content = () => (
    <QueryClientProvider client={client}>
      <WorkspaceContext.Provider value={workspace}>
        <MemoryRouter initialEntries={[initialEntry]}>
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
  expect(screen.getByText("3 个候选")).toBeVisible();
  expect(screen.getByText("全部：1 个任务 / 3 个候选")).toBeVisible();
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

it("rechecks remaining participants while accepting observation version growth", () => {
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
    screen.queryByText("候选或服务状态已变化，请取消后重新确认"),
  ).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "确认停止" })).toBeEnabled();
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

it("releases pending stop on its later rejected receipt", async () => {
  const w = stopWorkspace();
  w.submit.mockImplementation(async (envelope) =>
    receipt(envelope, "submitted"),
  );
  fireEvent.click(screen.getByRole("button", { name: "停止参与" }));
  fireEvent.click(screen.getByRole("button", { name: "确认停止" }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  expect(screen.getByRole("button", { name: "停止参与" })).toBeDisabled();
  const envelope = w.submit.mock.calls[0][0];
  w.update({
    requests: [
      {
        envelope,
        created_at: "now",
        response: {
          ...receipt(envelope, "rejected"),
          code: "last_candidate",
          diagnostic: "last participant",
        },
      },
    ],
  });
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "停止参与" })).toBeEnabled(),
  );
  expect(w.submit).toHaveBeenCalledTimes(1);
  w.client.clear();
});

it("separates applied stop from a delayed candidate snapshot", async () => {
  const w = stopWorkspace();
  w.submit.mockImplementation(async (envelope) => receipt(envelope, "applied"));
  fireEvent.click(screen.getByRole("button", { name: "停止参与" }));
  fireEvent.click(screen.getByRole("button", { name: "确认停止" }));
  await waitFor(() =>
    expect(screen.getByText(/停止已生效，列表刷新中/)).toBeVisible(),
  );
  expect(
    screen.queryByText("停止请求待核实，请在操作回执中查询原请求"),
  ).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "停止参与" })).toBeDisabled();
  expect(w.submit).toHaveBeenCalledTimes(1);
  w.client.clear();
});

function mixedWorkspace() {
  const question = (id: string, optional = false): Candidate["decision"] => ({
    id,
    attempt_id: "attempt",
    version: 1,
    status: "pending",
    request: {
      question: "问题 " + id,
      context: "",
      options: [],
      interaction_kind: optional ? "async" : "business_feedback",
      blocks_progress: !optional,
      allow_free_text: true,
      has_native_questions: false,
    },
  });
  const candidate = (
    task: string,
    number: number,
    change: Partial<Candidate>,
  ): Candidate => ({
    ...run,
    task_id: task,
    domain_task_id: task,
    run_id: `task${task}-c${number}`,
    candidate_number: number,
    execution_id: `execution-${task}-${number}`,
    title: `任务${task}候选${number}`,
    execution_status: "running",
    ...change,
  });
  const candidates = [
    candidate("1", 1, {}),
    candidate("1", 2, {
      execution_status: "waiting_input",
      workflow_status: "waiting_input",
      decision: question("blocking"),
    }),
    candidate("1", 3, {
      execution_status: "pr_ready",
      workflow_status: "pr_ready",
      workflow_stage: "await_merge",
    }),
    // Reverse snapshot order to prove navigation uses stable candidate numbers.
    candidate("2", 2, {
      execution_status: "waiting_input",
      workflow_status: "waiting_input",
      decision: question("second"),
    }),
    candidate("2", 1, { decision: question("optional", true) }),
    candidate("2", 3, {
      execution_status: "failed",
      workflow_status: "blocked",
      recovery_required: true,
    }),
  ];
  const taskPage = {
    ...tasks,
    tasks: ["1", "2"].map((id) => ({
      ...tasks.tasks[0],
      task_id: id,
      title: `混合任务${id}`,
      status: "in_progress",
    })),
  };
  return { ...stopWorkspace(candidates, taskPage), candidates, taskPage };
}

it("aggregates mixed candidate facts per task and navigates to matches while retaining sibling candidates", () => {
  const w = mixedWorkspace();
  const list = within(document.querySelector(".run-task-list") as HTMLElement);
  const firstTask = list.getByRole("button", { name: /混合任务1/ });
  expect(firstTask).toHaveTextContent("运行中 1 · 待回答 1 · 待合并 1");
  expect(screen.getByRole("button", { name: "待回答" })).toHaveAttribute(
    "title",
    "待回答：2 个任务 / 3 个候选",
  );
  fireEvent.click(screen.getByRole("button", { name: "待回答" }));
  expect(screen.getByRole("heading", { name: "任务1候选2" })).toBeVisible();
  expect(firstTask).toHaveTextContent("1 / 3 个候选命中");
  expect(screen.getByText("待回答：2 个任务 / 3 个候选")).toBeVisible();
  expect(document.querySelectorAll(".candidate-card")).toHaveLength(3);
  expect(
    document.querySelectorAll(".candidate-card.filter-match"),
  ).toHaveLength(1);
  expect(
    document.querySelector(".candidate-card.filter-match"),
  ).toHaveTextContent("候选 C2");
  fireEvent.click(list.getByRole("button", { name: /混合任务2/ }));
  expect(screen.getByRole("heading", { name: "任务2候选1" })).toBeVisible();
  expect(
    document.querySelectorAll(".candidate-card.filter-match"),
  ).toHaveLength(2);
  const optional = screen.getByRole("button", { name: /候选 C1/ });
  expect(within(optional).getByText("运行中")).toBeVisible();
  expect(within(optional).getByText("待回答")).toBeVisible();
  expect(within(optional).getByText("可稍后回答")).toBeVisible();
  expect(
    screen.getByText("当前执行可继续，方便时回答这个问题。"),
  ).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "待合并" }));
  expect(screen.getByRole("heading", { name: "任务1候选3" })).toBeVisible();
  expect(
    list.queryByRole("button", { name: /混合任务2/ }),
  ).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "需处理" }));
  expect(screen.getByRole("heading", { name: "任务2候选3" })).toBeVisible();
  expect(screen.getByText("需处理：1 个任务 / 1 个候选")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "运行中" }));
  expect(screen.getByRole("heading", { name: "任务2候选1" })).toBeVisible();
  expect(screen.getByText("运行中：2 个任务 / 2 个候选")).toBeVisible();
  w.client.clear();
});

it("allows inspecting a nonmatching sibling and returns to the matched candidate on task selection", () => {
  const w = mixedWorkspace();
  fireEvent.click(screen.getByRole("button", { name: "待回答" }));
  fireEvent.click(screen.getByRole("button", { name: /候选 C1/ }));
  expect(screen.getByRole("heading", { name: "任务1候选1" })).toBeVisible();
  expect(
    screen.getByText("当前候选已不符合筛选，保留当前查看内容。"),
  ).toBeVisible();
  fireEvent.click(
    within(document.querySelector(".run-task-list") as HTMLElement).getByRole(
      "button",
      { name: /混合任务1/ },
    ),
  );
  expect(screen.getByRole("heading", { name: "任务1候选2" })).toBeVisible();
  expect(
    screen.queryByText("当前候选已不符合筛选，保留当前查看内容。"),
  ).not.toBeInTheDocument();
  w.client.clear();
});

it("preserves the selected answer editor across reordered snapshots and keeps the selection after it stops matching", async () => {
  const w = mixedWorkspace();
  fireEvent.click(screen.getByRole("button", { name: "待回答" }));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "打开回答" })).toBeEnabled(),
  );
  fireEvent.click(screen.getByRole("button", { name: "打开回答" }));
  fireEvent.change(screen.getByRole("textbox", { name: "回答内容" }), {
    target: { value: "保留当前回答草稿" },
  });
  w.update({ snapshot: w.state([...w.candidates].reverse()) });
  expect(screen.getByRole("heading", { name: "任务1候选2" })).toBeVisible();
  expect(screen.getByRole("textbox", { name: "回答内容" })).toHaveValue(
    "保留当前回答草稿",
  );
  w.update({
    snapshot: w.state(
      w.candidates.map((candidate) =>
        candidate.run_id === "task1-c2"
          ? {
              ...candidate,
              execution_status: "running",
              workflow_status: "running",
              decision: { ...candidate.decision!, status: "answered" },
            }
          : candidate,
      ),
    ),
  });
  expect(screen.getByRole("heading", { name: "任务1候选2" })).toBeVisible();
  expect(
    screen.getByText("当前候选已不符合筛选，保留当前查看内容。"),
  ).toBeVisible();
  expect(screen.getByText("待回答：1 个任务 / 2 个候选")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "全部" }));
  expect(screen.getByRole("heading", { name: "任务1候选2" })).toBeVisible();
  w.client.clear();
});

it("pins the initial candidate identity even before the user clicks a task or filter", () => {
  const w = mixedWorkspace();
  w.update({ snapshot: w.state([...w.candidates].reverse()) });
  expect(screen.getByRole("heading", { name: "任务1候选1" })).toBeVisible();
  expect(
    within(document.querySelector(".run-task-list") as HTMLElement).getByRole(
      "button",
      { name: /混合任务1/ },
    ),
  ).toHaveTextContent("运行中 1 · 待回答 1 · 待合并 1");
  w.client.clear();
});

it("honors an explicit candidate deep link rather than selecting the first observed candidate", () => {
  const candidates = [1, 2].map((number) => ({
    ...run,
    run_id: `linked-${number}`,
    candidate_number: number,
    title: `链接候选${number}`,
    execution_status: "running",
  }));
  const w = stopWorkspace(candidates, tasks, "/?task=1&run=linked-2");
  expect(screen.getByRole("heading", { name: "链接候选2" })).toBeVisible();
  w.update({ snapshot: w.state([...candidates].reverse()) });
  expect(screen.getByRole("heading", { name: "链接候选2" })).toBeVisible();
  w.client.clear();
});

it("excludes ended task and abandoned candidate history from current filters even when history is visible", () => {
  const candidate = {
    ...run,
    execution_status: "running",
    workflow_status: "waiting_input",
    recovery_required: true,
  };
  const candidates = [
    { ...candidate, run_id: "abandoned", participation: "abandoned" },
    { ...candidate, run_id: "delivered", task_id: "2", domain_task_id: "2" },
    { ...candidate, run_id: "discarded", task_id: "3", domain_task_id: "3" },
  ];
  const taskPage = {
    ...tasks,
    tasks: [
      tasks.tasks[0],
      {
        ...tasks.tasks[0],
        task_id: "2",
        status: "closed",
        title: "已交付的任务",
      },
      {
        ...tasks.tasks[0],
        task_id: "3",
        status: "discarded",
        title: "已放弃的任务",
      },
    ],
  };
  const w = stopWorkspace(candidates, taskPage);
  expect(screen.getByText("全部：3 个任务 / 2 个候选")).toBeVisible();
  fireEvent.click(screen.getByRole("checkbox", { name: "显示已停止参与候选" }));
  expect(screen.getByText("全部：3 个任务 / 3 个候选")).toBeVisible();
  for (const name of ["运行中", "待回答", "待合并", "需处理"]) {
    fireEvent.click(screen.getByRole("button", { name }));
    expect(screen.getByText(`${name}：0 个任务 / 0 个候选`)).toBeVisible();
    expect(screen.getByText("没有符合筛选条件的任务")).toBeVisible();
    expect(document.querySelectorAll(".run-task")).toHaveLength(0);
  }
  w.client.clear();
});

it("keeps unconfirmed starts visible in all candidates without counting them as running", () => {
  const w = stopWorkspace([]);
  w.update({
    candidateStarts: {
      start: vi.fn(),
      items: ["等待执行信息", "提交结果待核实"].map((status, index) => ({
        id: "start" + index,
        taskID: "1",
        title: "启动中的任务",
        profile: "方案" + index,
        status,
        participating: true,
      })),
    },
  });
  expect(screen.getByText("全部：1 个任务 / 2 个候选")).toBeVisible();
  expect(screen.getByText("等待执行信息")).toBeVisible();
  expect(screen.getByText("提交结果待核实")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "运行中" }));
  expect(screen.getByText("运行中：0 个任务 / 0 个候选")).toBeVisible();
  expect(screen.getByText("没有符合筛选条件的任务")).toBeVisible();
  w.client.clear();
});
