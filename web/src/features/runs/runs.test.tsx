import { afterEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { projectQueries } from "../../shared/api/queries";
import {
  identity,
  mountFeature,
  run,
  tasks,
  testClient,
} from "../../test/workspace";
import { Runs } from "./runs";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
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
  const get = vi
    .spyOn(api, "get")
    .mockImplementation(
      async (path) => (path.includes("/activity?") ? activity : tasks) as never,
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
