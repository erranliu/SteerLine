import { StrictMode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Api } from "../shared/api/client";
import type {
  Envelope,
  Profile,
  Receipt,
  StoredRequest,
  TaskPage,
} from "../shared/api/types";
import { identity, tasks } from "../test/workspace";
import { useCandidateStarts } from "./use-candidate-starts";

afterEach(cleanup);
const profile = { id: "one", name: "方案一" } as Profile;
const task = tasks.tasks[0];
const receipt = (
  envelope: Envelope,
  status: Receipt["status"] = "applied",
): Receipt => ({ ...envelope, original: envelope, status, sampled_at: "now" });
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
function setup() {
  const api = new Api(identity, "csrf");
  const calls: {
    envelope: Envelope;
    response: ReturnType<typeof deferred<Receipt>>;
  }[] = [];
  const submit = vi.fn((envelope: Envelope) => {
    const response = deferred<Receipt>();
    calls.push({ envelope, response });
    return response.promise;
  });
  const hook = renderHook(
    ({ online, requests, client }) =>
      useCandidateStarts(client, online, requests, submit),
    {
      initialProps: {
        online: true,
        requests: [] as StoredRequest[],
        client: api,
      },
      wrapper: ({ children }) => <StrictMode>{children}</StrictMode>,
    },
  );
  return { api, calls, submit, ...hook };
}

it("shows five confirmations immediately, waits only for the original claim, and sends each subsequent candidate once at the claimed version", async () => {
  const { result, api, calls, submit } = setup();
  const ids: (string | undefined)[] = [];
  act(() => {
    for (let i = 0; i < 6; i++)
      ids.push(
        result.current.start(task, { ...profile, id: `profile-${i}` }, 1, 3),
      );
  });
  expect(result.current.items).toHaveLength(5);
  expect(ids[5]).toBeUndefined();
  expect(new Set(ids.slice(0, 5)).size).toBe(5);
  expect(result.current.items[0].status).toBe("正在提交");
  expect(
    result.current.items
      .slice(1)
      .every((entry) => entry.status === "等待首次启动确认"),
  ).toBe(true);
  expect(submit).toHaveBeenCalledTimes(1);
  const first = calls[0];
  const claimed: TaskPage = {
    ...tasks,
    tasks: [
      {
        ...task,
        status: "in_progress",
        version: 2,
        workflow_runs: [
          {
            workflow_run_id: ids[0]!,
            participation: "active",
            task_id: task.task_id,
            candidate_number: 1,
            submission_status: "pending",
            stop_requested: false,
          },
        ],
      },
    ],
  };
  vi.spyOn(api, "get").mockResolvedValue(claimed);
  await act(async () => {
    first.response.resolve(receipt(first.envelope));
  });
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(5));
  expect(
    calls
      .slice(1)
      .map(({ envelope }) => [
        envelope.request_id,
        envelope.action,
        envelope.expected_version,
        envelope.expected_graph_revision,
        envelope.payload.execution_profile_id,
      ]),
  ).toEqual(
    ids
      .slice(1, 5)
      .map((id, i) => [id, "start_candidate", 2, 1, `profile-${i + 1}`]),
  );
  expect(first.envelope).toMatchObject({
    action: "start_task",
    expected_version: 1,
  });
  // An unrelated render while later POSTs are delayed must not submit twice.
  await act(async () => {
    calls[4].response.resolve(receipt(calls[4].envelope));
  });
  expect(submit).toHaveBeenCalledTimes(5);
});

it.each(["version", "graph", "identity"])(
  "does not send waiting confirmations when the claimed %s differs",
  async (change) => {
    const { result, api, calls, submit } = setup();
    act(() => {
      result.current.start(task, profile, 1, 3);
      result.current.start(task, profile, 1, 3);
    });
    vi.spyOn(api, "get").mockResolvedValue({
      ...tasks,
      graph_revision: change === "graph" ? 2 : 1,
      tasks: [
        {
          ...task,
          status: "in_progress",
          version: change === "version" ? 3 : 2,
          workflow_runs: [
            {
              workflow_run_id:
                change === "identity"
                  ? "someone-else"
                  : calls[0].envelope.request_id,
            },
          ],
        },
      ],
    });
    await act(async () => {
      calls[0].response.resolve(receipt(calls[0].envelope));
    });
    await waitFor(() => expect(result.current.items[1].status).toBe("未提交"));
    expect(result.current.items[1].participating).toBe(false);
    expect(submit).toHaveBeenCalledTimes(1);
  },
);

it("keeps unknown requests pending and releases waiting slots only after explicit rejection", async () => {
  const { result, calls, submit, rerender, api } = setup();
  act(() => {
    result.current.start(task, profile, 1, 3);
    result.current.start(task, profile, 1, 3);
  });
  await act(async () => {
    calls[0].response.resolve(receipt(calls[0].envelope, "outcome_unknown"));
  });
  expect(result.current.items[0].status).toBe("提交结果待核实");
  expect(result.current.items[1].status).toBe("等待首次启动确认");
  expect(result.current.items.every((entry) => entry.participating)).toBe(true);
  rerender({
    online: true,
    client: api,
    requests: [
      {
        envelope: calls[0].envelope,
        created_at: "now",
        response: receipt(calls[0].envelope, "rejected"),
      },
    ],
  });
  await waitFor(() => expect(result.current.items[1].status).toBe("未提交"));
  expect(result.current.items.every((entry) => !entry.participating)).toBe(
    true,
  );
  expect(submit).toHaveBeenCalledTimes(1);
  act(() => {
    result.current.start(task, profile, 1, 3);
  });
  expect(calls[1].envelope.action).toBe("start_task");
  expect(calls[1].envelope.request_id).not.toBe(calls[0].envelope.request_id);
});

it("restores journal cards without submitting and drops only unsent in-memory waits on project change", async () => {
  const { result, calls, submit, rerender, api } = setup();
  act(() => {
    result.current.start(task, profile, 1, 3);
    result.current.start(task, profile, 1, 3);
  });
  const pendingRead = deferred<TaskPage>();
  vi.spyOn(api, "get").mockReturnValue(pendingRead.promise);
  await act(async () => {
    calls[0].response.resolve(receipt(calls[0].envelope));
  });
  const other = new Api({ ...identity, project_id: "other" }, "csrf");
  const requests = [{ envelope: calls[0].envelope, created_at: "now" }];
  rerender({ online: true, client: other, requests });
  expect(result.current.items).toEqual([]);
  await act(async () => {
    pendingRead.resolve(tasks);
  });
  expect(submit).toHaveBeenCalledTimes(1);
  rerender({ online: true, client: api, requests });
  expect(result.current.items).toHaveLength(1);
  expect(result.current.items[0].id).toBe(calls[0].envelope.request_id);
  expect(submit).toHaveBeenCalledTimes(1);
});

it("allows independent additions while another addition is unresolved", async () => {
  const { result, calls, submit } = setup();
  const running = { ...task, status: "in_progress", version: 2 };
  act(() => {
    result.current.start(running, profile, 1, 3);
    result.current.start(running, profile, 1, 3);
  });
  expect(submit).toHaveBeenCalledTimes(2);
  expect(
    calls.every(({ envelope }) => envelope.action === "start_candidate"),
  ).toBe(true);
  await act(async () => {
    calls[1].response.resolve(receipt(calls[1].envelope, "rejected"));
  });
  expect(result.current.items[1].status).toBe("启动被拒绝");
  expect(result.current.items[0].status).toBe("正在提交");
});

it("scopes restored start cards and the five-candidate limit to the current service instance", () => {
  const { result, rerender, api, submit } = setup();
  const old = new Api({ ...identity, instance_id: "previous" }, "csrf");
  const requests: StoredRequest[] = Array.from({ length: 18 }, (_, index) => {
    const envelope = old.envelope(
      index % 2 ? "start_candidate" : "start_task",
      task.task_id,
      1,
      { execution_profile_id: "old-profile" },
    );
    return {
      envelope,
      created_at: "now",
      response: receipt(envelope, index === 0 ? "outcome_unknown" : "applied"),
    };
  });
  const envelope = api.envelope("start_candidate", task.task_id, 2, {
    execution_profile_id: profile.id,
  });
  requests.push({ envelope, created_at: "now" });
  const original = structuredClone(requests);
  rerender({ online: true, client: api, requests });
  expect(result.current.items).toMatchObject([
    { id: envelope.request_id, status: "正在提交", participating: true },
  ]);
  expect(submit).not.toHaveBeenCalled();
  const running = { ...task, status: "in_progress", version: 2 };
  const ids: (string | undefined)[] = [];
  act(() => {
    for (let i = 0; i < 5; i++)
      ids.push(result.current.start(running, profile, 1, 3));
  });
  expect(ids.slice(0, 4).every(Boolean)).toBe(true);
  expect(ids[4]).toBeUndefined();
  expect(result.current.items).toHaveLength(5);
  expect(submit).toHaveBeenCalledTimes(4);
  expect(requests).toEqual(original);
});
