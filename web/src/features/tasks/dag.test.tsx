import { afterEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { projectQueries } from "../../shared/api/queries";
import {
  graph,
  identity,
  mountFeature,
  tasks,
  testClient,
} from "../../test/workspace";
import { Dag } from "./dag";

vi.mock("@xyflow/react", () => ({
  ReactFlow: () => <div>任务图画布</div>,
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
