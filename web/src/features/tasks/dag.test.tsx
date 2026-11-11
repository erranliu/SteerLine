import { afterEach, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { ComponentType } from "react";
import type { NodeProps } from "@xyflow/react";
import { useCandidateStarts } from "../../app/use-candidate-starts";
import { useWorkspace, WorkspaceContext } from "../../app/context";
import { Api } from "../../shared/api/client";
import { projectQueries } from "../../shared/api/queries";
import type { Envelope, Receipt, StoredRequest } from "../../shared/api/types";
import {
  graph,
  identity,
  mountFeature,
  snapshot,
  tasks,
  testClient,
} from "../../test/workspace";
import { Dag } from "./dag";

vi.mock("@xyflow/react", () => ({
  ReactFlow: ({
    nodes,
    nodeTypes: { task: Task },
  }: {
    nodes: NodeProps[];
    nodeTypes: { task: ComponentType<NodeProps> };
  }) => (
    <div aria-label="任务图画布">
      {nodes.map((node) => (
        <Task key={node.id} {...node} />
      ))}
    </div>
  ),
  Background: () => null,
  Controls: () => null,
  Handle: () => null,
  Position: { Top: "top", Bottom: "bottom" },
}));
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

it("polls only the visible graph or task list and resumes the graph on return", async () => {
  vi.useFakeTimers();
  const client = testClient();
  const api = new Api(identity, "csrf", "home");
  const options = projectQueries(api);
  client.setQueryData(options.graph().queryKey, graph);
  client.setQueryData(options.tasks().queryKey, tasks);
  const get = vi
    .spyOn(api, "get")
    .mockImplementation(
      async (path) => (path.startsWith("/graph") ? graph : tasks) as never,
    );
  mountFeature(<Dag />, api, client);
  await act(() => vi.advanceTimersByTimeAsync(2600));
  expect(
    get.mock.calls.filter(([path]) => path.startsWith("/graph")),
  ).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "任务列表" }));
  await act(() => vi.advanceTimersByTimeAsync(0));
  const graphCalls = get.mock.calls.filter(([path]) =>
    path.startsWith("/graph"),
  ).length;
  await act(() => vi.advanceTimersByTimeAsync(5200));
  expect(
    get.mock.calls.filter(([path]) => path.startsWith("/graph")),
  ).toHaveLength(graphCalls);
  expect(get.mock.calls.some(([path]) => path.startsWith("/tasks?"))).toBe(
    true,
  );
  expect(screen.getByText("缓存任务")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "任务图" }));
  await act(() => vi.advanceTimersByTimeAsync(0));
  const taskCalls = get.mock.calls.filter(([path]) =>
    path.startsWith("/tasks?"),
  ).length;
  await act(() => vi.advanceTimersByTimeAsync(2600));
  expect(
    get.mock.calls.filter(([path]) => path.startsWith("/graph")).length,
  ).toBeGreaterThan(graphCalls);
  expect(
    get.mock.calls.filter(([path]) => path.startsWith("/tasks?")),
  ).toHaveLength(taskCalls);
  client.clear();
});

it("keeps the DAG open and shows the local start before the receipt or task refresh", async () => {
  const client = testClient();
  const api = new Api(identity, "csrf", "home");
  const options = projectQueries(api);
  const profile = {
    id: "default",
    name: "默认方案",
    host: "local",
    agent: "codex",
  };
  client.setQueryData(options.graph().queryKey, graph);
  client.setQueryData(options.tasks().queryKey, tasks);
  vi.spyOn(api, "get").mockImplementation(async (path) => {
    if (path === "/presentation")
      return {
        presentation: { profiles: [profile], default_profile_id: profile.id },
      } as never;
    if (path === "/maintenance/readiness")
      return { status: "ready", blockers: [] } as never;
    if (path.startsWith("/environment/options"))
      return {
        environment_readiness_required: false,
        profiles: [],
        environments: [],
      } as never;
    return (path.startsWith("/graph") ? graph : tasks) as never;
  });
  let finish!: (receipt: Receipt) => void;
  const submit = vi.fn(
    (_: Envelope) =>
      new Promise<Receipt>((resolve) => {
        finish = resolve;
      }),
  );
  function WorkspaceDag() {
    const workspace = useWorkspace();
    const candidateStarts = useCandidateStarts(api, true, [], submit);
    return (
      <WorkspaceContext.Provider value={{ ...workspace, candidateStarts }}>
        <Dag />
      </WorkspaceContext.Provider>
    );
  }
  mountFeature(<WorkspaceDag />, api, client, true, {
    snapshot: snapshot(api),
  });
  fireEvent.click(screen.getByRole("button", { name: "任务列表" }));
  fireEvent.click(screen.getByRole("button", { name: /缓存任务.*待执行/ }));
  const start = await screen.findByRole("button", { name: "手动启动" });
  await waitFor(() => expect(start).toBeEnabled());
  fireEvent.click(start);
  const confirm = screen.getByRole("button", { name: "确认启动" });
  await waitFor(() => expect(confirm).toBeEnabled());
  fireEvent.click(confirm);
  expect(screen.getByRole("heading", { name: "任务 DAG" })).toBeVisible();
  expect(
    screen.getByRole("button", { name: /缓存任务.*启动中/ }),
  ).toHaveAttribute("data-execution-state", "task_starting");
  expect(screen.getByText("正在提交")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "任务图" }));
  const canvas = within(screen.getByLabelText("任务图画布"));
  expect(canvas.getByText("启动中")).toBeVisible();

  const envelope = submit.mock.calls[0][0];
  await act(async () =>
    finish({
      ...envelope,
      original: envelope,
      status: "applied",
      sampled_at: "now",
    }),
  );
  expect(screen.getByText("等待执行信息")).toBeVisible();
  expect(canvas.getByText("启动中")).toBeVisible();

  client.setQueryData(options.graph().queryKey, {
    ...graph,
    nodes: [{ ...graph.nodes[0], status: "in_progress" }],
  });
  expect(await canvas.findByText("执行中")).toBeVisible();
  expect(canvas.queryByText("启动中")).not.toBeInTheDocument();
  client.clear();
});

it("restores the newest original start and displays an explicit rejection", async () => {
  const client = testClient();
  const api = new Api(identity, "csrf", "home");
  client.setQueryData(projectQueries(api).graph().queryKey, graph);
  vi.spyOn(api, "get").mockResolvedValue(graph as never);
  const old = api.envelope("start_task", "1", 1, {});
  const latest = api.envelope("start_task", "1", 1, {});
  const unknown: StoredRequest = {
    envelope: latest,
    created_at: "2026-10-15T00:00:02Z",
    error: "请求超时",
  };
  const earlier: StoredRequest = {
    envelope: old,
    created_at: "2026-10-15T00:00:01Z",
  };
  const submit = vi.fn();
  function WorkspaceDag({ requests }: { requests: StoredRequest[] }) {
    const workspace = useWorkspace();
    const candidateStarts = useCandidateStarts(api, true, requests, submit);
    return (
      <WorkspaceContext.Provider value={{ ...workspace, candidateStarts }}>
        <Dag />
      </WorkspaceContext.Provider>
    );
  }
  mountFeature(<WorkspaceDag requests={[unknown, earlier]} />, api, client);
  const canvas = within(screen.getByLabelText("任务图画布"));
  expect(canvas.getByText("启动结果待确认")).toBeVisible();
  cleanup();

  const rejected: StoredRequest = {
    ...unknown,
    response: {
      ...latest,
      original: latest,
      status: "rejected",
      diagnostic: "执行槽已满",
      sampled_at: "now",
    },
  };
  mountFeature(<WorkspaceDag requests={[rejected, earlier]} />, api, client);
  expect(screen.getByText("待执行")).toBeVisible();
  expect(screen.getByText("启动失败")).toBeVisible();
  expect(submit).not.toHaveBeenCalled();
  client.clear();
});
