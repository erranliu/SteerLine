import { afterEach, expect, it, vi } from "vitest";
import {
  render,
  screen,
  waitFor,
  fireEvent,
  cleanup,
  within,
  act,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryRouter,
  RouterProvider,
  useLocation,
} from "react-router-dom";
import { App } from "./app";
import { Api } from "../shared/api/client";
import * as journal from "../shared/api/journal";
import {
  setJournalHome,
  writeDraft,
  readDraft,
  saveRequest,
  listRequests,
} from "../shared/api/journal";
import type { BrowserSession, Identity } from "../shared/api/types";
import { projectQueries } from "../shared/api/queries";
import { tasks } from "../test/workspace";
import { configSnapshot } from "../test/execution-config";

const a = "a".repeat(64),
  b = "b".repeat(64);
const session: BrowserSession = {
  csrf_token: "csrf",
  identity: {
    protocol: "steerline-web/v1",
    home_id: "switch-home",
    instance_id: "web-1",
    ready: true,
  },
};
const identity = (id: string) =>
  ({
    project_id: id,
    instance_id: "instance-" + id[0],
    repository_identity: "/" + id[0] + "/.git",
    ready: true,
  }) as Identity;
const projects = [a, b].map((id) => ({
  config_id: id + ".json",
  name: "同名项目",
  repository: "/" + id[0],
  project_id: id,
  online: true,
}));
function mock() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) =>
      Response.json(
        url === "/api/web/execution-config"
          ? configSnapshot
          : url.endsWith("/maintenance")
            ? {
                observation: { commit: "" },
                tasks: [],
                profiles: [],
                profile: "",
              }
            : url === "/auth/status"
              ? session
              : url.endsWith("/projects")
                ? { projects }
                : url.endsWith("/identity")
                  ? identity(url.includes(a) ? a : b)
                  : url.endsWith("/snapshot")
                    ? {
                        connection: {
                          state: "connected",
                          can_control: true,
                          identity_matched: true,
                          config_matched: true,
                          snapshot: {
                            page: {
                              service: {
                                instance_id:
                                  "instance-" + (url.includes(a) ? "a" : "b"),
                              },
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
                    : {
                        presentation: { profiles: [], default_profile_id: "" },
                      },
      ),
    ),
  );
}
function Location() {
  return (
    <output data-testid="location">
      {useLocation().pathname + useLocation().search + useLocation().hash}
    </output>
  );
}
function mount(route: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const router = createMemoryRouter(
    [
      {
        path: "*",
        element: (
          <>
            <App initial={session} />
            <Location />
          </>
        ),
      },
    ],
    { initialEntries: [route] },
  );
  const view = render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return { ...view, client, router };
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  localStorage.clear();
  sessionStorage.clear();
  setJournalHome("");
});

it("recognizes direct roadmap routes, keeps the page when switching projects, and restores its exact return target", async () => {
  mock();
  const original = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation((...args) =>
    String(args[0]).endsWith("/roadmaps")
      ? Promise.resolve(Response.json({ sources: [], records: [], profiles: [], documents_available: false, documents_reason: "" }))
      : original(...args),
  );
  mount(`/projects/${a}/roadmaps?source=draft-a`);
  await screen.findByRole("heading", { name: "Roadmap 草稿" }, { timeout: 5000 });
  expect(screen.getByRole("combobox", { name: "当前项目" })).toHaveValue(a);
  fireEvent.change(screen.getByRole("combobox", { name: "当前项目" }), { target: { value: b } });
  await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent(`/projects/${b}/roadmaps`));
  await screen.findByRole("heading", { name: "Roadmap 草稿" }, { timeout: 5000 });
  fireEvent.click(screen.getByRole("link", { name: "执行方案配置" }));
  await waitFor(() => expect(screen.getByRole("link", { name: "返回项目" })).toHaveAttribute("href", `/projects/${b}/roadmaps`));
  fireEvent.click(screen.getByRole("link", { name: "返回项目" }));
  await screen.findByRole("heading", { name: "Roadmap 草稿" }, { timeout: 5000 });
});

it("cancels the old project's pending preload and never installs its late response in the new cache", async () => {
  mock();
  const original = vi.mocked(fetch).getMockImplementation()!;
  let resolve!: (response: Response) => void;
  let oldSignal: AbortSignal | undefined;
  vi.mocked(fetch).mockImplementation((...args) => {
    const url = String(args[0]);
    if (url.includes(a) && url.includes("/tasks?")) {
      oldSignal = args[1]?.signal as AbortSignal;
      return new Promise((done) => {
        resolve = done;
      });
    }
    if (url.includes(b) && url.includes("/tasks?"))
      return Promise.resolve(
        Response.json({
          ...tasks,
          tasks: [{ ...tasks.tasks[0], title: "仅 B 项目" }],
        }),
      );
    return original(...args);
  });
  const { client } = mount("/projects/" + a + "/settings");
  await waitFor(() => expect(oldSignal).toBeDefined());
  fireEvent.change(screen.getByRole("combobox", { name: "当前项目" }), {
    target: { value: b },
  });
  await waitFor(() => expect(oldSignal?.aborted).toBe(true));
  const bKey = projectQueries(
    new Api(identity(b), "csrf", "switch-home"),
  ).tasks().queryKey;
  await waitFor(() =>
    expect(client.getQueryData(bKey)?.tasks[0].title).toBe("仅 B 项目"),
  );
  await act(async () => resolve(Response.json(tasks)));
  expect(client.getQueryData(bKey)?.tasks[0].title).toBe("仅 B 项目");
  expect(
    client.getQueryData(
      projectQueries(new Api(identity(a), "csrf", "switch-home")).tasks()
        .queryKey,
    ),
  ).toBeUndefined();
  client.clear();
});

it("switches same-name projects by ID, preserving page type and dropping task selection", async () => {
  mock();
  mount("/projects/" + a + "/settings?task=1");
  await screen.findByText("配置", { selector: "h1" });
  fireEvent.change(screen.getByRole("combobox", { name: "当前项目" }), {
    target: { value: b },
  });
  await waitFor(() =>
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/projects/" + b + "/settings",
    ),
  );
  expect(screen.getByTestId("location")).not.toHaveTextContent("task=");
  await waitFor(() => expect(screen.getByText("/b/.git")).toBeVisible());
  expect(screen.queryByText("/a/.git")).not.toBeInTheDocument();
});
it("keeps two tab selections independent even when recent preference changes", async () => {
  mock();
  const first = mount("/projects/" + a + "/settings");
  const second = mount("/projects/" + b + "/settings");
  await waitFor(() =>
    expect(within(first.container).getByRole("combobox")).toHaveValue(a),
  );
  await waitFor(() =>
    expect(within(second.container).getByRole("combobox")).toHaveValue(b),
  );
  window.dispatchEvent(
    new StorageEvent("storage", {
      key: "steerline-recent:switch-home",
      newValue: b,
    }),
  );
  expect(within(first.container).getByTestId("location")).toHaveTextContent(a);
});
it("shows inaccessible deep links without selecting a different project", async () => {
  mock();
  mount("/projects/" + "c".repeat(64) + "/runs?task=1");
  await screen.findByText("项目不可访问，请重新选择");
  expect(screen.getByTestId("location")).toHaveTextContent("c".repeat(64));
});
it("waits for the asynchronous catalog before restoring the recent project", async () => {
  mock();
  localStorage.setItem("steerline-recent:switch-home", a);
  const original = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation((...args) =>
    String(args[0]).endsWith("/projects")
      ? Promise.resolve(Response.json({ projects: [] }))
      : original(...args),
  );
  const { client } = mount("/");
  await screen.findByText("尚无已配置项目");
  act(() => client.setQueryData(["switch-home", "projects"], { projects }));
  await waitFor(() =>
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/projects/" + a + "/dag",
    ),
  );
});
it("falls back to a valid recent project when this tab's saved project was removed", async () => {
  mock();
  sessionStorage.setItem("steerline-project:switch-home", "removed-project");
  localStorage.setItem("steerline-recent:switch-home", b);
  mount("/");
  await waitFor(() =>
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/projects/" + b + "/dag",
    ),
  );
});
it("keeps project selection usable when optional browser preferences are unavailable", async () => {
  mock();
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new DOMException("blocked", "SecurityError");
  });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new DOMException("quota", "QuotaExceededError");
  });
  mount("/");
  const selector = await screen.findByRole("combobox", { name: "当前项目" });
  await waitFor(() =>
    expect(within(selector).getAllByRole("option")).toHaveLength(3),
  );
  fireEvent.change(selector, { target: { value: a } });
  await waitFor(() =>
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/projects/" + a + "/dag",
    ),
  );
});
it("partitions persisted envelopes and drafts by home under the same Origin", async () => {
  setJournalHome("first-home");
  const envelope = new Api(identity(a), "csrf").envelope(
    "start_task",
    "1",
    1,
    {},
  );
  await saveRequest(envelope);
  await writeDraft("same-task", "first draft");
  setJournalHome("second-home");
  expect(await listRequests()).toEqual([]);
  expect(await readDraft("same-task")).toBe("");
  await writeDraft("same-task", "second draft");
  setJournalHome("first-home");
  expect((await listRequests())[0].envelope).toEqual(envelope);
  expect(await readDraft("same-task")).toBe("first draft");
});
it("cancels a queued command before its original lock is acquired after switching", async () => {
  let release!: () => void;
  let first = true;
  vi.stubGlobal("navigator", {
    locks: {
      request: async (
        _key: string,
        options: LockOptions | (() => unknown),
        callback?: () => unknown,
      ) => {
        const work = typeof options === "function" ? options : callback!;
        if (first) {
          first = false;
          await new Promise<void>((done) => {
            release = done;
          });
        }
        return work();
      },
    },
  });
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  const api = new Api(identity(a), "csrf", "switch-home");
  const sent = api.submit(api.envelope("start_task", "1", 1, {}));
  api.deactivate();
  release();
  await expect(sent).rejects.toThrow("项目已切换");
  expect(fetch).not.toHaveBeenCalled();
});

it("cancels a switch during journal persistence without leaving an unsent request for replay", async () => {
  vi.stubGlobal("navigator", {
    locks: {
      request: async (
        _id: string,
        options: LockOptions | (() => unknown),
        work?: () => unknown,
      ) => (typeof options === "function" ? options : work!)(),
    },
  });
  setJournalHome("cancel-during-persistence");
  const api = new Api(identity(a), "csrf", "cancel-during-persistence");
  const originalSave = journal.saveRequest;
  vi.spyOn(journal, "saveRequest").mockImplementation(async (envelope) => {
    const result = await originalSave(envelope);
    api.deactivate();
    return result;
  });
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  const envelope = api.envelope("start_task", "1", 1, {});
  await expect(api.submit(envelope)).rejects.toThrow("项目已切换");
  expect(fetch).not.toHaveBeenCalled();
  expect(await journal.requestByID(envelope.request_id)).toBeUndefined();
  api.activate();
  expect(await listRequests()).toEqual([]);
});

it("reads an original envelope after native restart and never replays it into the new instance", async () => {
  vi.stubGlobal("navigator", {
    locks: {
      request: async (
        _id: string,
        options: LockOptions | (() => unknown),
        work?: () => unknown,
      ) => (typeof options === "function" ? options : work!)(),
    },
  });
  const old = new Api(identity(a), "csrf");
  const envelope = old.envelope("start_task", "1", 1, {});
  await saveRequest(envelope);
  const current = new Api(
    { ...identity(a), instance_id: "new-native" },
    "csrf",
  );
  const fetch = vi.fn(async () =>
    Response.json(
      { code: "command_not_found", reason: "absent" },
      { status: 404 },
    ),
  );
  vi.stubGlobal("fetch", fetch);
  await expect(current.reconcile(envelope)).rejects.toThrow("不能向新实例补交");
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0]).toBeDefined();
  expect(
    (await listRequests()).find(
      (row) => row.envelope.request_id === envelope.request_id,
    )?.envelope,
  ).toEqual(envelope);
});

it("TR recognizes test-record deep links and cancels late record reads when switching projects", async () => {
  mock();
  const original = vi.mocked(fetch).getMockImplementation()!;
  let resolveOld!: (response: Response) => void;
  let oldSignal: AbortSignal | undefined;
  vi.mocked(fetch).mockImplementation((...args) => {
    const url = String(args[0]);
    if (url.endsWith("/tests/catalogue"))
      return Promise.resolve(
        Response.json({
          tests: [
            {
              id: "rules",
              name: "Rules",
              level: "L1",
              environment: "isolated",
              command: "test",
              check: "assert",
              current: true,
            },
          ],
          path: "test-list.json",
          commit: "sha",
          observed_at: "2026-10-21T00:00:00Z",
        }),
      );
    if (url.includes("/tests/records?")) {
      if (url.includes(a)) {
        oldSignal = args[1]?.signal as AbortSignal;
        return new Promise((done) => {
          resolveOld = done;
        });
      }
      return Promise.resolve(
        Response.json({
          records: [],
          summary: { total: 0, pending: 0 },
          observed_at: "2026-10-21T00:00:00Z",
        }),
      );
    }
    return original(...args);
  });
  mount(`/projects/${a}/tests?test=rules`);
  expect(
    await screen.findByRole("heading", { name: "测试管理" }),
  ).toBeVisible();
  await waitFor(() => expect(resolveOld).toBeDefined());
  fireEvent.change(screen.getByRole("combobox", { name: "当前项目" }), {
    target: { value: b },
  });
  await waitFor(() =>
    expect(screen.getByTestId("location")).toHaveTextContent(
      `/projects/${b}/tests`,
    ),
  );
  await waitFor(() => expect(oldSignal?.aborted).toBe(true));
  await act(async () =>
    resolveOld(
      Response.json({
        records: [{ id: "late-old-project", status: "failed" }],
        summary: { total: 1, pending: 1 },
      }),
    ),
  );
  expect(screen.queryByText("late-old-project")).toBeNull();
  expect(await screen.findByText("尚无执行记录")).toBeVisible();
});

it("returns from execution profiles to the same project, page, query and fragment", async () => {
  mock();
  const target = `/projects/${a}/settings?task=1#details`;
  mount(target);
  await screen.findByRole("heading", { name: "配置" });
  fireEvent.click(screen.getByRole("link", { name: "执行方案配置" }));
  await screen.findByRole("heading", { name: "执行方案配置" });
  expect(screen.getByRole("link", { name: "返回项目" })).toHaveAttribute(
    "href",
    target,
  );
  expect(screen.getByRole("link", { name: "配置" })).toHaveAttribute(
    "href",
    `/projects/${a}/settings`,
  );
  expect(screen.queryByLabelText("生成最大并发数")).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("link", { name: "返回项目" }));
  await waitFor(() =>
    expect(screen.getByTestId("location")).toHaveTextContent(target),
  );
});

it("restores a valid recent project on direct entry and rejects a removed return target", async () => {
  mock();
  sessionStorage.setItem(
    "steerline-return:switch-home",
    `/projects/${"c".repeat(64)}/runs?task=2`,
  );
  localStorage.setItem("steerline-recent:switch-home", b);
  mount("/settings/execution-configs");
  await waitFor(() =>
    expect(screen.getByRole("link", { name: "返回项目" })).toHaveAttribute(
      "href",
      `/projects/${b}/dag`,
    ),
  );
});

it("keeps configuration available without projects and returns to project selection", async () => {
  mock();
  const original = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation((...args) =>
    String(args[0]).endsWith("/projects")
      ? Promise.resolve(Response.json({ projects: [] }))
      : original(...args),
  );
  mount("/settings/execution-configs");
  await screen.findByRole("heading", { name: "执行方案配置" });
  expect(
    screen.queryByRole("link", { name: "返回项目" }),
  ).not.toBeInTheDocument();
  expect(
    screen
      .getAllByRole("link", { name: "选择项目" })
      .every((link) => link.getAttribute("href") === "/projects"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("link", { name: "配置" }));
  expect(await screen.findByLabelText("生成最大并发数")).toHaveValue(5);
  expect(screen.getByText("所有项目共享", { exact: true })).toBeVisible();
});

it("guards return links and browser back while an execution profile has unsaved changes", async () => {
  mock();
  const { router } = mount(`/projects/${a}/settings`);
  await screen.findByRole("heading", { name: "配置" });
  fireEvent.click(screen.getByRole("link", { name: "执行方案配置" }));
  const name = await screen.findByLabelText("配置名称");
  fireEvent.change(name, { target: { value: "未保存方案" } });
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
  fireEvent.click(screen.getByRole("link", { name: "返回项目" }));
  await waitFor(() => expect(confirm).toHaveBeenCalledTimes(1));
  expect(screen.getByLabelText("配置名称")).toHaveValue("未保存方案");
  await act(async () => {
    await router.navigate(-1);
  });
  await waitFor(() => expect(confirm).toHaveBeenCalledTimes(2));
  expect(screen.getByTestId("location")).toHaveTextContent(
    "/settings/execution-configs",
  );
  confirm.mockReturnValue(true);
  fireEvent.click(screen.getByRole("link", { name: "返回项目" }));
  await screen.findByRole("heading", { name: "配置" });
});

it("keeps the exact return route when browser preference storage is unavailable", async () => {
  mock();
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new DOMException("blocked");
  });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new DOMException("blocked");
  });
  mount(`/projects/${a}/settings?task=4`);
  await screen.findByRole("heading", { name: "配置" });
  fireEvent.click(screen.getByRole("link", { name: "执行方案配置" }));
  await waitFor(() =>
    expect(screen.getByRole("link", { name: "返回项目" })).toHaveAttribute(
      "href",
      `/projects/${a}/settings?task=4`,
    ),
  );
});
