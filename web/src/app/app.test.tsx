import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  act,
  fireEvent,
} from "@testing-library/react";
import { StrictMode, type ComponentProps } from "react";
import {
  onlineManager,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { Identity } from "../shared/api/types";
import { Workspace } from "./app";
import { configSnapshot } from "../test/execution-config";
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

function workspaceRead(api: Api, path: string): unknown {
  if (path === "/snapshot") return makeSnapshot(api);
  if (path === "/presentation")
    return { presentation: { profiles: [], default_profile_id: "" } };
  if (path === "/maintenance")
    return { observation: { commit: "" }, tasks: [] };
  if (path.startsWith("/tasks?")) return tasks;
  throw new Error("Unexpected Workspace read: " + path);
}

function mountWorkspace(
  api: Api,
  client: QueryClient,
  props: Partial<ComponentProps<typeof Workspace>> = {},
) {
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter
        initialEntries={[`/projects/${api.identity.project_id}/settings`]}
      >
        <Workspace
          home={api.home}
          initial={{ identity: api.identity, csrf_token: "csrf" }}
          {...props}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

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
    .mockImplementation(async (path) => workspaceRead(api, path) as never);
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
  expect(screen.getByRole("button", { name: /项目维护/ })).toBeVisible();
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
        path === "/maintenance",
    ),
  ).toBe(true);
  client.clear();
});

it("removes duplicated profiles from configuration and scopes manual refresh", async () => {
  const client = testClient();
  const identity = { ...fixtureIdentity, project_id: "manual" };
  const api = new Api(identity, "csrf", "manual-home");
  client.setQueryData([api.home, "execution-config"], configSnapshot);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json(configSnapshot)),
  );
  const other = [api.home, "another-project", "instance", "snapshot"];
  client.setQueryData(other, {});
  const refreshConnection = vi.fn();
  vi.spyOn(Api.prototype, "get").mockImplementation(
    async (path) => workspaceRead(api, path) as never,
  );
  mountWorkspace(api, client, { onRefreshConnection: refreshConnection });
  expect(await screen.findByLabelText("生成最大并发数")).toHaveValue(5);
  expect(screen.queryByText("当前服务已加载方案")).not.toBeInTheDocument();
  expect(screen.queryByText("全局执行配置")).not.toBeInTheDocument();
  await screen.findByText("服务已连接", { exact: true });
  fireEvent.click(screen.getByRole("button", { name: "刷新查询" }));
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
    async (path) => workspaceRead(api, path) as never,
  );
  vi.spyOn(Api.prototype, "reconcile").mockResolvedValue({
    ...envelope,
    original: envelope,
    status: "applied",
    sampled_at: "now",
  });
  mountWorkspace(api, client);
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
  const api = new Api(identity, "csrf");
  const envelope = api.envelope("start_task", "1", 1, {});
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
        url === "/api/web/execution-config"
          ? configSnapshot
          : url === "/auth/status"
            ? { identity, csrf_token: "csrf" }
            : workspaceRead(api, url.replace(/^.*\/projects\/[^/]+/, "")),
      ),
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  mountWorkspace(api, client);
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
  const api = new Api(identity, "csrf");
  const snapshot = makeSnapshot(api);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) =>
      Response.json(
        url === "/api/web/execution-config"
          ? configSnapshot
          : url === "/auth/status"
            ? { identity, csrf_token: "csrf" }
            : workspaceRead(api, url.replace(/^.*\/projects\/[^/]+/, "")),
      ),
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  mountWorkspace(api, client);
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
    async (path) => workspaceRead(api, path) as never,
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
  const mounted = mountWorkspace(api, client);
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
