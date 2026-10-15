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
import { MemoryRouter, useLocation } from "react-router-dom";
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
        url === "/auth/status"
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
                : { presentation: { profiles: [], default_profile_id: "" } },
      ),
    ),
  );
}
function Location() {
  return (
    <output data-testid="location">
      {useLocation().pathname + useLocation().search}
    </output>
  );
}
function mount(route: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[route]}>
        <App initial={session} />
        <Location />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { ...view, client };
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  localStorage.clear();
  sessionStorage.clear();
  setJournalHome("");
});

it("switches same-name projects by ID, preserving page type and dropping task selection", async () => {
  mock();
  mount("/projects/" + a + "/settings?task=1");
  await screen.findByText("执行设置", { selector: "h1" });
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
      request: async (_key: string, work: () => unknown) => {
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
    locks: { request: async (_id: string, work: () => unknown) => work() },
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
    locks: { request: async (_id: string, work: () => unknown) => work() },
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
