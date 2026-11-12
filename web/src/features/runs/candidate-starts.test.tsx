import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import { WorkspaceContext } from "../../app/context";
import { useCandidateStarts } from "../../app/use-candidate-starts";
import { Api } from "../../shared/api/client";
import { projectQueries } from "../../shared/api/queries";
import type {
  Candidate,
  Envelope,
  Profile,
  Receipt,
  StoredRequest,
  TaskPage,
} from "../../shared/api/types";
import {
  identity,
  run,
  snapshot,
  tasks,
  testClient,
} from "../../test/workspace";
import { Runs } from "./runs";

afterEach(cleanup);
function setup(existing = false, requests: StoredRequest[] = []) {
  const api = new Api(identity, "csrf");
  const client = testClient();
  let materialStatus = "ready";
  const taskPage = (): TaskPage => ({
    ...tasks,
    tasks: [
      {
        ...tasks.tasks[0],
        status: existing ? "in_progress" : "open",
        start_eligibility: {
          ...tasks.tasks[0].start_eligibility!,
          action: existing ? "start_candidate" : "start_task",
          blockers: materialStatus === "ready"
            ? []
            : [{ code: "project_not_ready", diagnostic: "项目未就绪" }],
        },
        workflow_runs: existing
          ? [
              {
                workflow_run_id: run.run_id,
                participation: "active",
                task_id: "1",
                candidate_number: 1,
                submission_status: "accepted",
                stop_requested: false,
              },
            ]
          : null,
      },
    ],
  });
  const presentation = {
    presentation: {
      default_profile_id: "one",
      profiles: [
        {
          id: "one",
          name: "本次方案",
          unavailable_reason: "",
          host: "local",
          agent: "codex",
          details: [],
        } as Profile,
      ],
    },
  };
  const options = projectQueries(api);
  client.setQueryData(options.task("1").queryKey, taskPage());
  client.setQueryData(options.tasks().queryKey, taskPage());
  client.setQueryData(options.presentation().queryKey, presentation);
  client.setQueryData(options.projectReadiness().queryKey, {
    status: "ready",
    commit: "base",
    total: 1,
    ready: 1,
    blockers: [],
  });
  vi.spyOn(api, "get").mockImplementation(
    async (path) =>
      (path === "/environment/options"
        ? {
            profiles: [
              { id: "one", status: "ready" },
              { id: "orca-remote-desktop64g-xhigh", status: "ready" },
            ],
            environments: [],
          }
        : path === "/presentation"
          ? presentation
          : path === "/maintenance/readiness"
            ? {
                status: materialStatus === "ready" ? "ready" : "not_ready",
                commit: "base",
                total: 1,
                ready: materialStatus === "ready" ? 1 : 0,
                blockers: [],
              }
            : path.includes("/activity?")
              ? {
                  page: {
                    activity: path.startsWith("/runs/run/")
                      ? [
                          {
                            seq: 1,
                            kind: "activity",
                            title: "旧候选活动",
                            ts: "2026-10-20T00:00:00Z",
                          },
                        ]
                      : [],
                    diagnostics: [],
                  },
                }
              : path.includes("/last-message?")
                ? { run_id: "execution", attempt_id: "attempt", message: null }
                : path.endsWith("/tests")
                  ? { status: "missing" }
                  : taskPage()) as never,
  );
  const calls: { envelope: Envelope; resolve: (value: Receipt) => void }[] = [];
  const submit = vi.fn(
    (envelope: Envelope) =>
      new Promise<Receipt>((resolve) => {
        calls.push({ envelope, resolve });
      }),
  );
  function Harness({ runs }: { runs: Candidate[] }) {
    const candidateStarts = useCandidateStarts(api, true, requests, submit);
    const observation = snapshot(api, runs);
    if (!runs.length) observation.connection.snapshot.page!.runs = null;
    observation.connection.snapshot.page!.service.capabilities.push(
      "start_candidate",
    );
    return (
      <WorkspaceContext.Provider
        value={{
          api,
          identity,
          online: true,
          snapshot: observation,
          candidateStarts,
          busy: true,
          send: async () => true,
          submit,
          refresh: () => {},
        }}
      >
        <Runs />
      </WorkspaceContext.Provider>
    );
  }
  const initial = existing ? [run] : [];
  const view = render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/projects/project/runs?task=1"]}>
        <Harness runs={initial} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  const showRuns = (runs: Candidate[]) =>
    view.rerender(
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <Harness runs={runs} />
        </MemoryRouter>
      </QueryClientProvider>,
    );
  const confirm = async (first = false) => {
    fireEvent.click(
      await screen.findByRole("button", {
        name: first ? "手动启动" : "新增并行候选",
      }),
    );
    fireEvent.click(await screen.findByRole("button", { name: "确认启动" }));
  };
  return {
    client,
    calls,
    submit,
    showRuns,
    confirm,
    setMaterials(status: string) {
      materialStatus = status;
      client.setQueryData(options.task("1").queryKey, taskPage());
      client.setQueryData(options.projectReadiness().queryKey, {
        status: status === "ready" ? "ready" : "not_ready",
        commit: "base",
        total: 1,
        ready: status === "ready" ? 1 : 0,
        blockers: [],
      });
    },
  };
}

it("immediately selects each new card and queues confirmations while the first response is delayed", async () => {
  const { confirm, submit, client } = setup();
  await confirm(true);
  expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  expect(document.querySelector(".candidate-card.active")).toHaveTextContent(
    "正在提交",
  );
  for (let count = 2; count <= 5; count++) {
    await confirm();
    const cards = document.querySelectorAll(".candidate-card");
    expect(cards).toHaveLength(count);
    expect(cards[count - 1]).toHaveClass("active");
    expect(cards[count - 1]).toHaveTextContent("等待首次启动确认");
  }
  expect(submit).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("button", { name: "新增并行候选" })).toBeEnabled();
  expect(
    screen.queryByRole("button", { name: "近期活动" }),
  ).not.toBeInTheDocument();
  client.clear();
});

it("submits the selected remote profile when a fresh task and snapshot have null run lists", async () => {
  const { confirm, submit, calls, client } = setup();
  const remoteProfile = {
    id: "orca-remote-desktop64g-xhigh",
    name: "远端 Orca · GPT-6-Luna xhigh",
    host: "remote",
    agent: "orca-terminal",
    execution_model: "gpt-6-luna",
    execution_effort: "xhigh",
    unavailable_reason: "",
    details: [],
  };
  const api = new Api(identity, "csrf");
  client.setQueryData(projectQueries(api).presentation().queryKey, {
    presentation: {
      default_profile_id: remoteProfile.id,
      profiles: [remoteProfile],
    },
  });
  await confirm(true);
  expect(submit).toHaveBeenCalledTimes(1);
  expect(calls[0].envelope).toMatchObject({
    action: "start_task",
    target_id: "1",
    expected_version: 1,
    payload: { execution_profile_id: remoteProfile.id },
  });
  expect(document.querySelector(".candidate-card.active")).toHaveTextContent(
    "正在提交",
  );
  client.clear();
});

it("keeps startup colors through pending candidates and highlights only confirmed running candidates", async () => {
  const { confirm, calls, showRuns, client } = setup();
  await confirm(true);
  const card = () => document.querySelector(".candidate-card.active")!;
  expect(card()).toHaveAttribute("data-execution-state", "task_starting");
  expect(document.querySelector(".run-task")).toHaveAttribute(
    "data-execution-state",
    "task_starting",
  );
  const { envelope, resolve } = calls[0];
  await act(async () =>
    resolve({
      ...envelope,
      status: "outcome_unknown",
      sampled_at: "now",
    }),
  );
  expect(card()).toHaveAttribute("data-execution-state", "task_start_unknown");
  expect(
    within(card() as HTMLElement).getByText("启动结果待确认"),
  ).toBeVisible();

  const observed = { ...run, run_id: envelope.request_id };
  showRuns([{ ...observed, execution_status: "queued" }]);
  expect(card()).toHaveAttribute("data-execution-state", "task_starting");
  expect(card()).toHaveTextContent("排队中");
  showRuns([{ ...observed, execution_status: "running" }]);
  expect(card()).toHaveAttribute("data-execution-state", "running");
  expect(
    within(screen.getByRole("region", { name: "当前进度" })).getByText("运行中"),
  ).toHaveClass("status-running");
  showRuns([{ ...observed, execution_status: "running", stop_requested: true }]);
  expect(card()).not.toHaveAttribute("data-execution-state");
  expect(card()).toHaveTextContent("停止待核实");
  client.clear();
});

it("clears old output immediately, keeps selection through late receipts, and replaces journal cards by the same server identity without duplicates", async () => {
  const { confirm, submit, calls, showRuns, client } = setup(true);
  await screen.findByText("旧候选活动");
  await confirm();
  expect(screen.queryByText("旧候选活动")).not.toBeInTheDocument();
  expect(document.querySelectorAll(".candidate-card")).toHaveLength(2);
  await confirm();
  expect(submit).toHaveBeenCalledTimes(2);
  expect(document.querySelectorAll(".candidate-card")).toHaveLength(3);
  fireEvent.click(document.querySelectorAll(".candidate-card")[0]);
  await act(async () => {
    calls[1].resolve({
      ...calls[1].envelope,
      status: "applied",
      sampled_at: "now",
    });
  });
  expect(document.querySelectorAll(".candidate-card")[0]).toHaveClass("active");
  const confirmed = calls.map(({ envelope }, i) => ({
    ...run,
    run_id: envelope.request_id,
    candidate_number: i + 2,
    title: `正式候选${i + 2}`,
    execution_id: "",
    attempt_id: "",
  }));
  showRuns([run, ...confirmed]);
  await waitFor(() =>
    expect(document.querySelectorAll(".candidate-card")).toHaveLength(3),
  );
  expect(document.querySelectorAll(".candidate-card")[0]).toHaveClass("active");
  fireEvent.click(document.querySelectorAll(".candidate-card")[2]);
  expect(screen.getByRole("heading", { name: "正式候选3" })).toBeVisible();
  expect(
    screen.queryByText("执行信息将在服务器确认后显示。"),
  ).not.toBeInTheDocument();
  client.clear();
});

it("shows server candidates after restart without restoring obsolete journal cards or consuming their slots", async () => {
  const old = new Api({ ...identity, instance_id: "previous" }, "csrf");
  const requests: StoredRequest[] = Array.from({ length: 19 }, (_, index) => {
    const envelope = old.envelope("start_candidate", "1", 2, {
      execution_profile_id: "obsolete-profile",
    });
    if (index === 0) envelope.request_id = run.run_id;
    return {
      envelope,
      created_at: "now",
      response: { ...envelope, status: "applied", sampled_at: "now" },
    };
  });
  const { confirm, client, submit } = setup(true, requests);
  await screen.findByText("旧候选活动");
  expect(document.querySelectorAll(".candidate-card")).toHaveLength(1);
  expect(screen.getByText("1 个候选")).toBeVisible();
  expect(screen.getByText("全部：1 个任务 / 1 个候选")).toBeVisible();
  expect(screen.queryByText("新候选")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "新增并行候选" })).toBeEnabled();
  expect(submit).not.toHaveBeenCalled();
  await confirm();
  expect(document.querySelectorAll(".candidate-card")).toHaveLength(2);
  expect(screen.getByText("2 个候选")).toBeVisible();
  expect(screen.getByText("全部：1 个任务 / 2 个候选")).toBeVisible();
  expect(submit).toHaveBeenCalledTimes(1);
  client.clear();
});

it("keeps a rejected start as history while observing current preparation and permits a fresh start after readiness", async () => {
  const { client, confirm, calls, submit, setMaterials } = setup();
  await confirm(true);
  const original = structuredClone(calls[0].envelope);
  const diagnostic = "测试安排尚未就绪：accepted";
  await act(async () => {
    calls[0].resolve({
      ...original,
      status: "rejected",
      code: "project_not_ready",
      diagnostic,
      sampled_at: "now",
    });
    setMaterials("accepted");
  });
  const history = await screen.findByText("上次启动结果 · 启动被拒绝");
  expect(document.querySelector(".candidate-card.active")).toHaveTextContent(
    "启动被拒绝（历史）",
  );
  expect(screen.getByRole("button", { name: "手动启动" })).toBeDisabled();
  const current = within(history.closest(".detail-section")!).getByRole(
    "status",
    { name: "项目就绪状态" },
  );
  expect(current).toHaveTextContent("项目未就绪");
  expect(current).not.toHaveTextContent(diagnostic);

  await act(async () => {
    setMaterials("ready");
  });
  await waitFor(() => expect(current).toHaveTextContent("项目已就绪"));
  expect(current).toHaveTextContent("可在任务详情重新选择方案启动");
  expect(current).not.toHaveTextContent(diagnostic);
  fireEvent.click(history);
  expect(
    within(history.closest("details")!).getByText(diagnostic),
  ).toBeVisible();
  expect(submit).toHaveBeenCalledTimes(1);
  await confirm(true);
  expect(submit).toHaveBeenCalledTimes(2);
  expect(calls[1].envelope.request_id).not.toBe(original.request_id);
  expect(original).toEqual(calls[0].envelope);
  expect(document.querySelectorAll(".candidate-card")).toHaveLength(2);
  client.clear();
});
