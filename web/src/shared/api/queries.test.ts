import { afterEach, expect, it, vi } from "vitest";
import { QueryObserver } from "@tanstack/react-query";
import { Api } from "./client";
import {
  invalidateCommand,
  projectQueries,
  receiptChanged,
  refreshProject,
} from "./queries";
import type { Receipt } from "./types";
import { identity, run, tasks, testClient } from "../../test/workspace";

afterEach(() => vi.useRealTimers());

it("retains an in-flight bootstrap read when the old page observer unmounts before the next subscribes", async () => {
  const client = testClient();
  const api = new Api(identity, "csrf", "home");
  const options = projectQueries(api).tasks();
  let resolve!: (value: typeof tasks) => void;
  const get = vi.spyOn(api, "get").mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }) as never,
  );
  const previousPage = new QueryObserver(client, {
    ...options,
    enabled: false,
  });
  const unsubscribe = previousPage.subscribe(() => {});
  const prefetch = client.prefetchQuery(options);
  unsubscribe();
  const nextPage = new QueryObserver(client, options);
  const stop = nextPage.subscribe(() => {});
  expect(get).toHaveBeenCalledTimes(1);
  resolve(tasks);
  await prefetch;
  expect(client.getQueryData(options.queryKey)).toEqual(tasks);
  stop();
  client.clear();
});

it("shares a pending task page between prefetch and page consumers, isolating parameters and identities", async () => {
  const api = new Api(identity, "csrf", "home");
  const client = testClient();
  let resolve!: (data: typeof tasks) => void;
  const get = vi.spyOn(api, "get").mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }) as never,
  );
  const first = client.prefetchQuery(projectQueries(api).tasks());
  const second = client.fetchQuery(projectQueries(api).tasks("", 100));
  expect(get).toHaveBeenCalledTimes(1);
  resolve(tasks);
  await first;
  expect(await second).toEqual(tasks);
  for (const key of [
    projectQueries(api).tasks("next").queryKey,
    projectQueries(api).tasks("", 50).queryKey,
    projectQueries(new Api(identity, "csrf", "other-home")).tasks().queryKey,
    projectQueries(
      new Api({ ...identity, project_id: "other" }, "csrf", "home"),
    ).tasks().queryKey,
    projectQueries(
      new Api({ ...identity, instance_id: "new" }, "csrf", "home"),
    ).tasks().queryKey,
  ])
    expect(client.getQueryData(key)).toBeUndefined();
  client.setQueryData(projectQueries(api).final(run).queryKey, {
    workflow_id: run.run_id,
    execution_id: run.execution_id,
    attempt_id: run.attempt_id,
    bodies: [],
    page: { events: [], has_more: false },
  });
  expect(
    client.getQueryData(
      projectQueries(api).final({ ...run, attempt_id: "new" }).queryKey,
    ),
  ).toBeUndefined();
  client.clear();
});

it("reuses presentation for sixty seconds then revalidates on access", async () => {
  vi.useFakeTimers();
  const api = new Api(identity, "csrf", "home");
  const get = vi
    .spyOn(api, "get")
    .mockResolvedValue({ presentation: { profiles: [] } });
  const client = testClient();
  await client.fetchQuery(projectQueries(api).presentation());
  await vi.advanceTimersByTimeAsync(59000);
  await client.prefetchQuery(projectQueries(api).presentation());
  expect(get).toHaveBeenCalledTimes(1);
  await vi.advanceTimersByTimeAsync(1001);
  await client.fetchQuery(projectQueries(api).presentation());
  expect(get).toHaveBeenCalledTimes(2);
  client.clear();
});

it.each([
  "set_dispatch_mode",
  "pause_dispatch",
  "resume_dispatch",
  "start_task",
  "start_candidate",
  "answer_decision",
  "stop_candidate",
  "open_native_session",
  "future_action",
  "answer-without-run",
])("invalidates the original resource scope for %s", async (action) => {
  const client = testClient();
  const api = new Api(identity, "csrf", "home");
  const envelope = api.envelope(
    action === "answer-without-run" ? "answer_decision" : action,
    "run",
    1,
    action === "answer-without-run" ? {} : { run_id: "run" },
  );
  const keys = [
    "snapshot",
    "tasks",
    "task",
    "graph",
    "presentation",
    "identity",
  ].map((resource) => ["home", "project", "instance", resource]);
  keys.push(
    ["home", "project", "instance", "activity", "run"],
    ["home", "project", "instance", "final", "other-run"],
    ["home", "project", "new-instance", "snapshot"],
    ["home", "other-project", "instance", "snapshot"],
    ["other-home", "project", "instance", "snapshot"],
    ["home", "local-requests"],
    ["home", "project", "identity"],
    ["home", "projects"],
    ["browser-session", "home"],
  );
  keys.forEach((key) => client.setQueryData(key, {}));
  await invalidateCommand(client, "home", envelope);
  const invalidated = keys.filter(
    (key) => client.getQueryState(key)?.isInvalidated,
  );
  const expected = [keys[11]]; // Only the original home's request log is always invalidated.
  if (action !== "open_native_session") expected.push(...keys.slice(0, 4));
  if (
    [
      "answer_decision",
      "stop_candidate",
      "future_action",
      "answer-without-run",
    ].includes(action)
  )
    expected.push(keys[6]);
  if (["future_action", "answer-without-run"].includes(action))
    expected.push(keys[7]);
  expect(invalidated).toEqual(keys.filter((key) => expected.includes(key)));
  client.clear();
});

it.each(["presentation", "taskMaterials", "candidateDocuments"] as const)(
  "scopes %s refresh and cancellation to the current home/project/instance",
  async (resource) => {
    const client = testClient();
    const api = new Api(identity, "csrf", "home");
    const key = (a: Api) =>
      (resource === "presentation"
        ? projectQueries(a).presentation()
        : projectQueries(a)[resource]("id")
      ).queryKey;
    const current = key(api);
    const others = [
      new Api(identity, "csrf", "other"),
      new Api({ ...identity, project_id: "other" }, "csrf", "home"),
      new Api({ ...identity, instance_id: "old" }, "csrf", "home"),
    ].map(key);
    [current, ...others].forEach((k) => client.setQueryData([...k], {}));
    client.setQueryData(["home", "local-requests"], []);
    await refreshProject(client, api);
    expect(client.getQueryState(current)?.isInvalidated).toBe(true);
    others.forEach((k) =>
      expect(client.getQueryState(k)?.isInvalidated).toBe(false),
    );
    expect(
      client.getQueryState(["home", "local-requests"])?.isInvalidated,
    ).toBe(true);
    if (resource !== "presentation") {
      client.removeQueries({ queryKey: current });
      let finish!: (value: unknown) => void;
      const get = vi.spyOn(api, "get").mockImplementation(
        () =>
          new Promise((resolve) => {
            finish = resolve;
          }) as never,
      );
      const pending =
        resource === "taskMaterials"
          ? client.prefetchQuery(projectQueries(api).taskMaterials("id"))
          : client.prefetchQuery(projectQueries(api).candidateDocuments("id"));
      const signal = get.mock.calls[0][1];
      await client.cancelQueries({
        queryKey: [api.home, identity.project_id, identity.instance_id],
      });
      expect(signal?.aborted).toBe(true);
      finish({ commit: "late" });
      await pending;
      expect(client.getQueryData(current)).toBeUndefined();
    }
    client.clear();
  },
);

it("refreshes changed outcomes including unknown and rejection without treating a sampling timestamp as a change", () => {
  const receipt = { status: "accepted", sampled_at: "first" } as Receipt;
  expect(receiptChanged(receipt, { ...receipt, sampled_at: "second" })).toBe(
    false,
  );
  for (const status of ["applied", "rejected", "outcome_unknown"] as const)
    expect(receiptChanged(receipt, { ...receipt, status })).toBe(true);
  expect(receiptChanged(undefined, receipt)).toBe(true);
});
