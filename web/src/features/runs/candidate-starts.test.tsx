import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
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
function setup(existing = false) {
  const api = new Api(identity, "csrf");
  const client = testClient();
  const taskPage = {
    ...tasks,
    tasks: [
      {
        ...tasks.tasks[0],
        status: existing ? "in_progress" : "open",
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
          : [],
      },
    ],
  };
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
  client.setQueryData(options.task("1").queryKey, taskPage);
  client.setQueryData(options.tasks().queryKey, taskPage);
  client.setQueryData(options.presentation().queryKey, presentation);
  vi.spyOn(api, "get").mockImplementation(
    async (path) =>
      (path === "/presentation"
        ? presentation
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
              : taskPage) as never,
  );
  const calls: { envelope: Envelope; resolve: (value: Receipt) => void }[] = [];
  const submit = vi.fn(
    (envelope: Envelope) =>
      new Promise<Receipt>((resolve) => {
        calls.push({ envelope, resolve });
      }),
  );
  const requests: [] = [];
  function Harness({ runs }: { runs: Candidate[] }) {
    const candidateStarts = useCandidateStarts(api, true, requests, submit);
    const observation = snapshot(api, runs);
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
  return { client, calls, submit, showRuns, confirm };
}

it("immediately selects each new card and lets an unclaimed task reserve five confirmations while its first response is delayed", async () => {
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
  expect(screen.getByRole("button", { name: "新增并行候选" })).toBeDisabled();
  expect(
    screen.queryByRole("button", { name: "近期活动" }),
  ).not.toBeInTheDocument();
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
