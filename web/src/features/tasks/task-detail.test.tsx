import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { WorkspaceContext, type Workspace } from "../../app/context";
import { Api } from "../../shared/api/client";
import type {
  Candidate,
  Identity,
  Profile,
  TaskPage,
} from "../../shared/api/types";
import { TaskDetail } from "./task-detail";
import { projectQueries } from "../../shared/api/queries";
import {
  identity as fixtureIdentity,
  snapshot as fixtureSnapshot,
  tasks as fixtureTasks,
  testClient,
} from "../../test/workspace";

const clients: QueryClient[] = [];

it("preserves cached task content on refresh failure but never uses it for a different task", async () => {
  const client = testClient();
  clients.push(client);
  const api = new Api(fixtureIdentity, "csrf", "cache-home");
  client.setQueryData(projectQueries(api).task("1").queryKey, fixtureTasks, {
    updatedAt: Date.now() - 2000,
  });
  let reject!: (error: Error) => void;
  vi.spyOn(api, "get").mockImplementation((path) => {
    if (path === "/presentation")
      return Promise.resolve({
        presentation: { profiles: [], default_profile_id: "" },
      }) as never;
    return new Promise((_resolve, fail) => {
      if (path === "/tasks/1") reject = fail;
    }) as never;
  });
  const workspace: Workspace = {
    api,
    identity: api.identity,
    online: true,
    snapshot: fixtureSnapshot(api),
    send: async () => true,
    busy: false,
    refresh: () => {},
  };
  const content = (id: string) => (
    <QueryClientProvider client={client}>
      <WorkspaceContext.Provider value={workspace}>
        <MemoryRouter>
          <TaskDetail id={id} />
        </MemoryRouter>
      </WorkspaceContext.Provider>
    </QueryClientProvider>
  );
  const view = render(content("1"));
  expect(screen.getByRole("heading", { name: "缓存任务" })).toBeVisible();
  reject(new Error("详情刷新失败"));
  await screen.findByText("Error: 详情刷新失败");
  expect(screen.getByRole("heading", { name: "缓存任务" })).toBeVisible();
  view.rerender(content("2"));
  expect(
    screen.queryByRole("heading", { name: "缓存任务" }),
  ).not.toBeInTheDocument();
  expect(screen.getByText("正在读取任务详情…")).toBeVisible();
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
});

function setup({
  unavailableDefault = false,
  count = 1,
  firstStart = false,
} = {}) {
  const api = new Api(
    { project_id: "candidate-project", instance_id: "instance" } as Identity,
    "csrf",
  );
  const profiles: Profile[] = [
    {
      id: "default",
      name: "默认方案",
      host: "local",
      agent: "codex",
      details: [],
      unavailable_reason: unavailableDefault ? "默认方案不可用" : "",
    },
    {
      id: "other",
      name: "另一方案",
      host: "local",
      agent: "codex",
      details: [],
      unavailable_reason: "",
    },
  ];
  const runs = firstStart
    ? []
    : Array.from(
        { length: count },
        (_, i) =>
          ({
            run_id: `run-${i}`,
            task_id: "38",
            domain_task_id: "38",
            participation: "active",
            submission_status: "accepted",
            stop_requested: false,
            workflow_status: "running",
            workflow_stage: "implement.execute",
            execution_profile: profiles[0],
            pr: { state: "none" },
          }) as Candidate,
      );
  const taskPage: TaskPage = {
    graph_revision: 7,
    has_more: false,
    database_revision: 1,
    database_id: "database",
    sampled_at: "2026-10-15T03:00:00Z",
    tasks: [
      {
        task_id: "38",
        title: "候选任务",
        version: 2,
        status: firstStart ? "open" : "in_progress",
        dependency_ready: true,
        spec_path: "docs/spec/task.md",
        startup_blockers: [],
        predecessors: [],
        successors: [],
        workflow_runs: runs.map((run, i) => ({
          workflow_run_id: run.run_id,
          participation: run.participation,
          submission_status: run.submission_status,
          stop_requested: run.stop_requested,
          task_id: "38",
          candidate_number: i + 1,
        })),
      },
    ],
  };
  vi.spyOn(api, "get").mockImplementation(
    async (path) =>
      (path === "/presentation"
        ? { presentation: { default_profile_id: "default", profiles } }
        : taskPage) as never,
  );
  const send = vi.fn().mockResolvedValue(true);
  const workspace = {
    api,
    identity: api.identity,
    online: true,
    busy: false,
    send,
    snapshot: {
      connection: {
        snapshot: {
          page: {
            service: { capabilities: ["start_task", "start_candidate"] },
            control: {
              revision: 3,
              dispatch_mode: "manual",
              dispatch_paused: false,
            },
            runs,
          },
        },
      },
    },
  } as unknown as Workspace;
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <WorkspaceContext.Provider value={workspace}>
        <MemoryRouter>
          <TaskDetail id="38" allowCandidate />
        </MemoryRouter>
      </WorkspaceContext.Provider>
    </QueryClientProvider>,
  );
  return { send };
}

it.each(["default", "other"])(
  "opens the candidate profile picker and submits %s independently of existing candidates",
  async (profile) => {
    const { send } = setup();
    fireEvent.click(
      await screen.findByRole("button", { name: "新增并行候选" }),
    );
    const picker = screen.getByRole("combobox", { name: "本次执行方案" });
    await waitFor(() => expect(picker).toHaveValue("default"));
    expect(send).not.toHaveBeenCalled();
    fireEvent.change(picker, { target: { value: profile } });
    fireEvent.click(screen.getByRole("button", { name: "确认启动" }));
    await waitFor(() =>
      expect(send).toHaveBeenCalledWith(
        "start_candidate",
        "38",
        2,
        { execution_profile_id: profile },
        { expected_control_revision: 3, expected_graph_revision: 7 },
      ),
    );
    await waitFor(() =>
      expect(screen.queryByRole("combobox")).not.toBeInTheDocument(),
    );
  },
);

it("lets an unavailable default be replaced before confirming a candidate", async () => {
  const { send } = setup({ unavailableDefault: true });
  fireEvent.click(await screen.findByRole("button", { name: "新增并行候选" }));
  await screen.findByText("默认方案不可用");
  expect(screen.getByRole("button", { name: "确认启动" })).toBeDisabled();
  fireEvent.change(screen.getByRole("combobox"), {
    target: { value: "other" },
  });
  expect(screen.getByRole("button", { name: "确认启动" })).toBeEnabled();
  fireEvent.click(screen.getByRole("button", { name: "确认启动" }));
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "start_candidate",
      "38",
      2,
      { execution_profile_id: "other" },
      { expected_control_revision: 3, expected_graph_revision: 7 },
    ),
  );
});

it("keeps the five participating candidate limit", async () => {
  const { send } = setup({ count: 5 });
  const button = await screen.findByRole("button", { name: "新增并行候选" });
  expect(button).toBeDisabled();
  expect(button).toHaveAttribute("title", "已达到 5 个参与候选");
  fireEvent.click(button);
  expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  expect(send).not.toHaveBeenCalled();
});

it("keeps first startup using the selected profile", async () => {
  const { send } = setup({ firstStart: true });
  fireEvent.click(await screen.findByRole("button", { name: "手动启动" }));
  await waitFor(() =>
    expect(screen.getByRole("combobox")).toHaveValue("default"),
  );
  fireEvent.change(screen.getByRole("combobox"), {
    target: { value: "other" },
  });
  fireEvent.click(screen.getByRole("button", { name: "确认启动" }));
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "start_task",
      "38",
      2,
      { execution_profile_id: "other" },
      { expected_control_revision: 3 },
    ),
  );
});
