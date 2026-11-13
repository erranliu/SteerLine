import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  act,
  fireEvent,
  within,
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
  graph,
  identity as fixtureIdentity,
  snapshot as makeSnapshot,
  tasks,
  testClient,
} from "../test/workspace";

function workspaceRead(api: Api, path: string): unknown {
  if (path.startsWith("/environment/options"))
    return { profiles: [], environments: [], generators: [] };
  if (path.startsWith("/environment/materials?"))
    return { document: "# 项目运行环境" };
  if (path === "/snapshot") return makeSnapshot(api);
  if (path === "/dispatch")
    return {
      mode: "manual",
      revision: 1,
      state: "closed",
      default_profile_id: "default",
      default_profile_name: "默认方案",
      environment_readiness_required: true,
    };
  if (path === "/presentation")
    return { presentation: { profiles: [], default_profile_id: "" } };
  if (path === "/maintenance/readiness")
    return {
      status: "not_ready",
      commit: "",
      total: 0,
      ready: 0,
      blockers: [{ category: "remote", code: "remote_unchecked", diagnostic: "远端尚未检查" }],
    };
  if (path === "/maintenance")
    return {
      readiness: {
        status: "not_ready",
        commit: "",
        total: 0,
        ready: 0,
        blockers: [{ category: "remote", code: "remote_unchecked", diagnostic: "远端尚未检查" }],
      },
      observation: { commit: "" },
      tasks: [],
    };
  if (path.startsWith("/tasks?")) return tasks;
  if (path.startsWith("/graph?")) return graph;
  throw new Error("Unexpected Workspace read: " + path);
}

function mountWorkspace(
  api: Api,
  client: QueryClient,
  props: Partial<ComponentProps<typeof Workspace>> = {},
  route = "settings",
) {
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter
        initialEntries={[`/projects/${api.identity.project_id}/${route}`]}
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

it("shows the backend rejection in the environment page alert and allows dismissing it", async () => {
  const client = testClient();
  const api = new Api(
    { ...fixtureIdentity, project_id: "environment-error" },
    "csrf",
    "environment-error-home",
  );
  const diagnostic =
    "environment.md requires one steerline-environment JSON block";
  const profile = {
    id: "project-default",
    name: "本机",
    environment_key: "e".repeat(64),
    status: "not_ready",
    stage: "awaiting_user",
    reason: "",
    current_revision: 0,
  };
  const options = {
    profiles: [profile],
    default_profile_id: profile.id,
    environments: [
      {
        ...profile,
        target: { kind: "local" },
        adapter: "codex-app-server",
        agent: "codex",
        activity_settled: true,
        document_path: "SteerlineFlow/environment.md",
        profiles: [profile],
      },
    ],
  };
  vi.spyOn(Api.prototype, "get").mockImplementation(
    async (path) =>
      (path.startsWith("/environment/options")
        ? options
        : workspaceRead(api, path)) as never,
  );
  const submit = vi
    .spyOn(Api.prototype, "submit")
    .mockImplementation(async (envelope) => ({
      ...envelope,
      original: envelope,
      status: "rejected",
      diagnostic,
      sampled_at: "now",
    }));
  mountWorkspace(api, client, {}, "environment");
  const start = await screen.findByRole("button", { name: "开始检测" });
  await waitFor(() => expect(start).toBeEnabled());
  fireEvent.click(start);
  const alert = await screen.findByRole("alert");
  expect(within(alert).getByText(diagnostic)).toBeVisible();
  expect(submit).toHaveBeenCalledTimes(1);
  expect(submit.mock.calls[0][0].action).toBe("environment_start");
  fireEvent.click(within(alert).getByRole("button", { name: "收起提示" }));
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(submit).toHaveBeenCalledTimes(1);
  client.clear();
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
  await screen.findByText("服务在线", { exact: true });
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
  expect(await screen.findByText("项目未就绪 · 远端尚未检查")).toBeVisible();
  expect(get.mock.calls.some(([path]) => path === "/maintenance")).toBe(false);
  vi.useFakeTimers();
  await act(() => vi.advanceTimersByTimeAsync(21000));
  expect(
    get.mock.calls.filter(([path]) => path === "/presentation"),
  ).toHaveLength(1);
  expect(
    get.mock.calls.filter(([path]) => path.startsWith("/tasks?")),
  ).toHaveLength(1);
  expect(
    get.mock.calls
      .map(([path]) => path)
      .filter(
        (path) =>
          !(
            path === "/snapshot" ||
            path === "/dispatch" ||
            path === "/presentation" ||
            path === "/tasks?limit=100&cursor=" ||
            path === "/maintenance" ||
            path === "/maintenance/readiness" ||
            path === "/environment/options" ||
            path === "/environment/options?configuration=true" ||
            path.startsWith("/activities?")
          ),
      ),
  ).toEqual([]);
  expect(get.mock.calls.filter(([path]) => path === "/dispatch")).toHaveLength(1);
  client.clear();
});

it("keeps project dispatch separate from shared configuration and scopes manual refresh", async () => {
  const client = testClient();
  const identity = { ...fixtureIdentity, project_id: "manual" };
  const api = new Api(identity, "csrf", "manual-home");
  const other = [api.home, "another-project", "instance", "snapshot"];
  client.setQueryData(other, {});
  const refreshConnection = vi.fn();
  vi.spyOn(Api.prototype, "get").mockImplementation(
    async (path) => workspaceRead(api, path) as never,
  );
  mountWorkspace(api, client, { onRefreshConnection: refreshConnection });
  expect(await screen.findByRole("heading", { name: "任务自动派发" })).toBeVisible();
  expect(screen.queryByLabelText("生成最大并发数")).not.toBeInTheDocument();
  expect(screen.queryByText("当前服务已加载方案")).not.toBeInTheDocument();
  expect(screen.queryByText("全局执行配置")).not.toBeInTheDocument();
  await screen.findByText("服务在线", { exact: true });
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

it("keeps the dispatch link in the project header and opens the switch in settings", async () => {
  await import("../features/runs/runs");
  const client = testClient();
  const api = new Api(
    { ...fixtureIdentity, project_id: "task-dispatch-routes" },
    "csrf",
    "task-dispatch-routes-home",
  );
  vi.spyOn(Api.prototype, "get").mockImplementation(
    async (path) => workspaceRead(api, path) as never,
  );
  mountWorkspace(api, client);
  const header = within(screen.getByRole("banner"));
  const closed = { name: "任务自动派发：关闭" };
  expect(await header.findByRole("link", closed)).toBeVisible();

  fireEvent.click(screen.getByRole("link", { name: "任务执行" }));
  expect(await screen.findByRole("heading", { name: "任务执行" })).toBeVisible();
  expect(header.getByRole("link", closed)).toBeVisible();
  expect(screen.queryByRole("switch", { name: "自动派发" })).toBeNull();

  fireEvent.click(header.getByRole("link", closed));
  const panel = await screen.findByRole("region", { name: "任务自动派发" });
  expect(panel).toHaveFocus();
  expect(within(panel).getByRole("switch", { name: "自动派发" })).toHaveAttribute(
    "aria-checked",
    "false",
  );
  expect(header.getByRole("link", closed)).toBeVisible();
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
            : url.endsWith("/maintenance/readiness")
              ? { status: "ready", blockers: [] }
              : workspaceRead(api, url.replace(/^.*\/projects\/[^/]+/, "")),
      ),
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  mountWorkspace(api, client);
  await waitFor(() =>
    expect(screen.getByText("服务在线", { exact: true })).toBeVisible(),
  );
  await waitFor(() =>
    expect(screen.getByRole("switch", { name: "自动派发" })).toBeEnabled(),
  );
  act(() => onlineManager.setOnline(false));
  expect(
    screen.getByText("当前不可提交业务操作", { exact: true }),
  ).toBeVisible();
  expect(screen.getByRole("switch", { name: "自动派发" })).toBeDisabled();
  expect(screen.getByRole("link", { name: "任务自动派发：待核实" })).toBeVisible();
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
    expect(screen.getByRole("switch", { name: "自动派发" })).toBeEnabled(),
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
  expect(
    screen.queryByRole("region", { name: "操作结果" }),
  ).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("link", { name: /^配置$/ }));
  await screen.findByRole("region", { name: "操作结果" });
  fireEvent.click(screen.getByText(/操作回执 · 最近/));
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
