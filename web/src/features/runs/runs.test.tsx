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
import { RunnerWorkflow } from "./runner-workflow";
import { render } from "@testing-library/react";

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

it("shows workflow changes and blocker reasons while folding technical event names", async () => {
  const client = testClient();
  const api = new Api(identity, "csrf", "home");
  const options = projectQueries(api);
  client.setQueryData(options.tasks().queryKey, tasks);
  client.setQueryData(options.task("1").queryKey, tasks);
  const activity = {
    page: {
      diagnostics: [],
      activity: [
        {
          seq: 1,
          ts: "2026-10-15T00:00:00Z",
          kind: "worker_state",
          status: "blocked",
          title: "实施报告 · 状态：执行中 → 已阻塞",
          detail: "GitHub HTTP 401 Requires authentication",
        },
        {
          seq: 2,
          ts: "2026-10-15T00:00:01Z",
          kind: "runner_event",
          status: "operation_committed",
          title: "operation_committed",
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
  expect(screen.getByText("实施报告 · 状态：执行中 → 已阻塞")).toBeVisible();
  expect(screen.getByText(/GitHub HTTP 401/)).toBeVisible();
  expect(screen.getByText(/operation_committed/)).not.toBeVisible();
  fireEvent.click(screen.getByText("内部执行事件"));
  expect(screen.getByText(/operation_committed/)).toBeVisible();
  client.clear();
});
