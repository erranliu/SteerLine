import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  act,
  fireEvent,
} from "@testing-library/react";
import { StrictMode } from "react";
import {
  onlineManager,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { Identity } from "../shared/api/types";
import { Workspace } from "./app";
import { Api } from "../shared/api/client";
import { saveRequest, listRequests } from "../shared/api/journal";
import type { Receipt } from "../shared/api/types";
import { projectQueries } from "../shared/api/queries";
import {
  identity as fixtureIdentity,
  snapshot as makeSnapshot,
  tasks,
  testClient,
} from "../test/workspace";

afterEach(() => {
  cleanup();
  onlineManager.setOnline(true);
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

it("preloads only after connection verification, shares presentation in StrictMode, and does not repeat on snapshot samples", async () => {
  const client = testClient();
  const identity = { ...fixtureIdentity, project_id: "preload" };
  const api = new Api(identity, "csrf", "preload-home");
  const get = vi
    .spyOn(Api.prototype, "get")
    .mockImplementation(
      async (path) =>
        (path === "/snapshot"
          ? makeSnapshot(api)
          : path === "/presentation"
            ? { presentation: { profiles: [], default_profile_id: "" } }
            : tasks) as never,
    );
  const mount = (connected: boolean) => (
    <StrictMode>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={["/projects/preload/settings"]}>
          <Workspace
            initial={{ identity, csrf_token: "csrf" }}
            home={api.home}
            connected={connected}
          />
        </MemoryRouter>
      </QueryClientProvider>
    </StrictMode>
  );
  const view = render(mount(false));
  expect(get).not.toHaveBeenCalled();
  view.rerender(mount(true));
  await screen.findByText("服务已连接", { exact: true });
  await waitFor(() =>
    expect(get.mock.calls.some(([path]) => path.startsWith("/tasks?"))).toBe(
      true,
    ),
  );
  expect(
    get.mock.calls.filter(([path]) => path === "/presentation"),
  ).toHaveLength(1);
  expect(
    get.mock.calls.filter(([path]) => path.startsWith("/tasks?")),
  ).toHaveLength(1);
  for (let index = 0; index < 3; index++)
    await act(async () => {
      client.setQueryData(
        projectQueries(api).snapshot().queryKey,
        makeSnapshot(api),
      );
    });
  vi.useFakeTimers();
  await act(() => vi.advanceTimersByTimeAsync(21000));
  expect(
    get.mock.calls.filter(([path]) => path === "/presentation"),
  ).toHaveLength(1);
  expect(
    get.mock.calls.filter(([path]) => path.startsWith("/tasks?")),
  ).toHaveLength(1);
  expect(
    get.mock.calls.every(
      ([path]) =>
        path === "/snapshot" ||
        path === "/presentation" ||
        path === "/tasks?limit=100&cursor=" ||
        path === "/maintenance?mode=baseline&plan=",
    ),
  ).toBe(true);
  client.clear();
});

it("keeps cached settings visible through a failed refresh and scopes manual refresh", async () => {
  const client = testClient();
  const identity = { ...fixtureIdentity, project_id: "manual" };
  const api = new Api(identity, "csrf", "manual-home");
  const options = projectQueries(api);
  client.setQueryData(options.presentation().queryKey, {
    presentation: {
      profiles: [
        {
          id: "profile",
          name: "已缓存方案",
          host: "local",
          agent: "codex",
          unavailable_reason: "",
          details: [],
        },
      ],
      default_profile_id: "profile",
    },
  });
  const other = [api.home, "another-project", "instance", "snapshot"];
  client.setQueryData(other, {});
  const refreshConnection = vi.fn();
  vi.spyOn(Api.prototype, "get").mockImplementation(async (path) => {
    if (path === "/snapshot") return makeSnapshot(api) as never;
    if (path === "/presentation") throw new Error("刷新失败");
    return tasks as never;
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/projects/manual/settings"]}>
        <Workspace
          initial={{ identity, csrf_token: "csrf" }}
          home={api.home}
          onRefreshConnection={refreshConnection}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(screen.getByText("已缓存方案")).toBeVisible();
  await screen.findByText("服务已连接", { exact: true });
  fireEvent.click(screen.getByRole("button", { name: "刷新查询" }));
  await screen.findByText("Error: 刷新失败");
  expect(screen.getByText("已缓存方案")).toBeVisible();
  expect(refreshConnection).toHaveBeenCalledTimes(1);
  expect(client.getQueryState(other)?.isInvalidated).toBe(false);
  client.clear();
});

it("invalidates an old receipt's original instance after reconciliation without changing the new instance cache", async () => {
  const client = testClient();
  const identity = {
    ...fixtureIdentity,
    project_id: "old-receipt",
    instance_id: "current",
  };
  const api = new Api(identity, "csrf");
  const original = new Api({ ...identity, instance_id: "old" }, "csrf");
  const envelope = original.envelope("start_task", "1", 1, {});
  await saveRequest(envelope);
  const oldKey = projectQueries(original).tasks().queryKey;
  client.setQueryData(oldKey, tasks);
  vi.spyOn(Api.prototype, "get").mockImplementation(
    async (path) =>
      (path === "/snapshot"
        ? makeSnapshot(api)
        : path === "/presentation"
          ? { presentation: { profiles: [] } }
          : tasks) as never,
  );
  vi.spyOn(Api.prototype, "reconcile").mockResolvedValue({
    ...envelope,
    original: envelope,
    status: "applied",
    sampled_at: "now",
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/projects/old-receipt/settings"]}>
        <Workspace initial={{ identity, csrf_token: "csrf" }} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  await waitFor(() =>
    expect(client.getQueryState(oldKey)?.isInvalidated).toBe(true),
  );
  expect(
    client.getQueryState(projectQueries(api).snapshot().queryKey)
      ?.isInvalidated,
  ).toBe(false);
  client.clear();
});

it("runs only one reconciliation round while journal observations refresh and aborts it offline", async () => {
  const identity = {
    project_id: "poll-project",
    instance_id: "poll-instance",
    repository_identity: "repo",
    ready: true,
  } as Identity;
  const envelope = new Api(identity, "csrf").envelope("start_task", "1", 1, {});
  await saveRequest(envelope);
  let resolve!: (receipt: Receipt) => void;
  const reconcile = vi.spyOn(Api.prototype, "reconcile").mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) =>
      Response.json(
        url === "/auth/status"
          ? { identity, csrf_token: "csrf" }
          : url.endsWith("/snapshot")
            ? {
                connection: {
                  state: "connected",
                  can_control: true,
                  identity_matched: true,
                  config_matched: true,
                  snapshot: {
                    page: {
                      service: { instance_id: identity.instance_id },
                      runs: [],
                      control: {
                        revision: 1,
                        dispatch_mode: "manual",
                        dispatch_paused: false,
                      },
                    },
                  },
                },
              }
            : { presentation: { profiles: [], default_profile_id: "" } },
      ),
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/projects/poll-project/settings"]}>
        <Workspace initial={{ identity, csrf_token: "csrf" }} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  await waitFor(() => expect(reconcile).toHaveBeenCalledTimes(1));
  const rows = await listRequests();
  for (let i = 1; i <= 5; i++)
    await act(async () => {
      client.setQueryData(["", "local-requests"], rows, {
        updatedAt: Date.now() + i,
      });
    });
  expect(reconcile).toHaveBeenCalledTimes(1);
  const signal = reconcile.mock.calls[0][1]!;
  act(() => onlineManager.setOnline(false));
  expect(signal.aborted).toBe(true);
  await act(async () => {
    resolve({
      ...envelope,
      original: envelope,
      status: "accepted",
      sampled_at: "now",
    });
  });
  expect(reconcile).toHaveBeenCalledTimes(1);
  client.clear();
});

it("disables commands when offline even while paused queries retain a successful snapshot", async () => {
  const identity = {
    project_id: "offline-project",
    instance_id: "original",
    repository_identity: "repo",
    ready: true,
  } as Identity;
  const snapshot = {
    connection: {
      state: "connected",
      can_control: true,
      identity_matched: true,
      config_matched: true,
      snapshot: {
        page: {
          service: {
            instance_id: "original",
            ready: true,
            capabilities: ["pause_dispatch", "set_dispatch_mode"],
          },
          runs: [],
          control: {
            revision: 1,
            dispatch_mode: "manual",
            dispatch_paused: false,
            blockers: [],
          },
        },
      },
    },
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) =>
      Response.json(
        url === "/auth/status"
          ? { identity, csrf_token: "csrf" }
          : url.endsWith("/snapshot")
            ? snapshot
            : { presentation: { profiles: [], default_profile_id: "" } },
      ),
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/projects/poll-project/settings"]}>
        <Workspace initial={{ identity, csrf_token: "csrf" }} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  await waitFor(() =>
    expect(screen.getByText("服务已连接", { exact: true })).toBeVisible(),
  );
  expect(screen.getByRole("button", { name: "暂停派发" })).toBeEnabled();
  act(() => onlineManager.setOnline(false));
  expect(
    screen.getByText("当前不可提交业务操作", { exact: true }),
  ).toBeVisible();
  expect(screen.getByRole("button", { name: "暂停派发" })).toBeDisabled();
  expect(
    client.getQueryData([
      "",
      identity.project_id,
      identity.instance_id,
      "snapshot",
    ]),
  ).toEqual(snapshot);
  act(() => onlineManager.setOnline(true));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "暂停派发" })).toBeEnabled(),
  );
  client.clear();
});

it("routes manual reconciliation through the original envelope while an automatic round is pending", async () => {
  const client = testClient();
  const identity = {
    ...fixtureIdentity,
    project_id: "manual-receipt",
    instance_id: "current",
  };
  const api = new Api(identity, "csrf", "manual-home");
  const original = new Api(
    { ...identity, instance_id: "old" },
    "csrf",
    api.home,
  );
  const envelope = original.envelope("start_task", "1", 1, {});
  await saveRequest(envelope);
  const originalKey = projectQueries(original).tasks().queryKey;
  client.setQueryData(originalKey, tasks);
  vi.spyOn(Api.prototype, "get").mockImplementation(
    async (path) =>
      (path === "/snapshot"
        ? makeSnapshot(api)
        : path === "/presentation"
          ? { presentation: { profiles: [] } }
          : tasks) as never,
  );
  let finishAutomatic!: (receipt: Receipt) => void;
  let finishManual!: (receipt: Receipt) => void;
  const reconcile = vi.spyOn(Api.prototype, "reconcile").mockImplementation(
    (_envelope, signal) =>
      new Promise((resolve) => {
        if (signal) finishAutomatic = resolve;
        else finishManual = resolve;
      }),
  );
  const mounted = render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/projects/manual-receipt/settings"]}>
        <Workspace home={api.home} initial={{ identity, csrf_token: "csrf" }} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  await waitFor(() => expect(reconcile).toHaveBeenCalledTimes(1));
  const button = await screen.findByRole("button", { name: "查询原请求" });
  fireEvent.click(button);
  await waitFor(() => expect(reconcile).toHaveBeenCalledTimes(2));
  expect(reconcile.mock.calls[1][0]).toEqual(envelope);
  expect(reconcile.mock.calls[1][1]).toBeUndefined();
  expect(button).toBeDisabled();
  await act(async () =>
    finishManual({
      ...envelope,
      original: envelope,
      status: "applied",
      sampled_at: "now",
    }),
  );
  await waitFor(() =>
    expect(client.getQueryState(originalKey)?.isInvalidated).toBe(true),
  );
  expect(
    client.getQueryState(projectQueries(api).snapshot().queryKey)
      ?.isInvalidated,
  ).toBe(false);
  expect(reconcile).toHaveBeenCalledTimes(2);
  const signal = reconcile.mock.calls[0][1]!;
  mounted.unmount();
  expect(signal.aborted).toBe(true);
  await act(async () =>
    finishAutomatic({
      ...envelope,
      original: envelope,
      status: "accepted",
      sampled_at: "now",
    }),
  );
  expect(reconcile).toHaveBeenCalledTimes(2);
  client.clear();
});
