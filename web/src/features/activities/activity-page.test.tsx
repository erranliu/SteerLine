import { afterEach, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClientProvider, focusManager } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { WorkspaceContext } from "../../app/context";
import { Api } from "../../shared/api/client";
import { identity, testClient } from "../../test/workspace";
import { ActivityPage } from "./activity-page";
import type { Activity, ActivityPage as ActivityListPage } from "./model";
import { readBody, type ReadingItem } from "./reading-api";

const clients: ReturnType<typeof testClient>[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((c) => c.clear());
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  focusManager.setFocused(undefined);
});
const activity: Activity = {
  activity_id: "a",
  project_id: "project",
  source_id: "candidate",
  source: "task_candidate",
  kind: "task_execution",
  title: "活动一",
  phase: "实施中",
  state: "running",
  needs_attention: false,
  request_ids: [],
  observation: "current",
  details_ref: { kind: "candidate", target: "candidate" },
  message: {
    text: "正在处理原任务",
    kind: "assistant",
    observed_at: "2026-11-10T00:00:00Z",
    truncated: false,
  },
};
const longItem: ReadingItem = {
  id: "final",
  role: "assistant",
  kind: "verified_final",
  label: "已核实的最终回复",
  group: "原轮次",
  preview: "正文预览",
  bytes: 300000,
  sha256: "a".repeat(64),
};
function setup(
  path = "/projects/project/activities",
  terminal = false,
  bound = false,
) {
  const current: Activity = {
    ...activity,
    state: terminal ? "succeeded" : activity.state,
    execution_ref: bound ? { run_id: "run", attempt_id: "attempt" } : undefined,
  };
  const api = new Api(identity, "csrf", "home");
  const calls: string[] = [];
  const client = testClient();
  clients.push(client);
  vi.spyOn(api, "get").mockImplementation(async (path) => {
    calls.push(path);
    if (path.startsWith("/activities?"))
      return {
        project_id: "project",
        instance_id: "instance",
        items: [current],
        request_links: {},
        counts: { ongoing: 1, attention: 0, recent: 1, history: 70 },
        counts_known: true,
        sources: [],
        has_more: !path.includes("cursor="),
        next_cursor: "next",
        sampled_at: "2026-11-10T00:00:00Z",
      };
    if (path.startsWith("/activities/a/conversation")) {
      const older = path.includes("cursor=before");
      const body = path.includes("cursor=body");
      return {
        activity_id: "a",
        run_id: "run",
        attempt_id: "attempt",
        session_id: "session",
        generation: "generation",
        partial: older,
        next_cursor: older || body ? undefined : "before",
        body_offset: body ? 0 : undefined,
        messages: [
          {
            id: older ? "earlier" : "latest",
            role: "assistant",
            text: body
              ? "最新的助手消息完整正文"
              : older
                ? "更早的助手消息"
                : "最新的助手消息",
            truncated: !older && !body,
            body_cursor: !older && !body ? "body" : undefined,
          },
        ],
      };
    }
    if (path.endsWith("/changes")) return { version: "2" };
    if (path.includes("/interactions?"))
      return {
        activity_id: "a",
        items: [longItem],
        version: "1",
        has_more: !path.includes("cursor="),
        next_cursor: "index-next",
        diagnostic: "仅显示已记录的交互",
      };
    if (path.includes("/body?")) {
      const offset = Number(
        new URL("http://local" + path).searchParams.get("offset"),
      );
      const next = Math.min(300000, offset + 256 * 1024);
      return {
        id: "final",
        sha256: longItem.sha256,
        offset,
        next_offset: next,
        bytes: 300000,
        text: "x".repeat(next - offset),
        complete: next === 300000,
      };
    }
    if (path === "/activities/a") return current;
    throw new Error("Unexpected read: " + path);
  });
  const submit = vi.spyOn(api, "submit");
  const rendered = render(
    <QueryClientProvider client={client}>
      <WorkspaceContext.Provider
        value={{
          api,
          identity,
          online: true,
          busy: false,
          send: async () => true,
          submit: (e) => api.submit(e),
          refresh: () => {},
        }}
      >
        <MemoryRouter initialEntries={[path]}>
          <ActivityPage />
        </MemoryRouter>
      </WorkspaceContext.Provider>
    </QueryClientProvider>,
  );
  return { calls, client, submit, api, ...rendered };
}
const indexReads = (calls: string[]) =>
  calls.filter((p) => p.includes("/interactions?"));
const bodyReads = (calls: string[]) =>
  calls.filter((p) => p.includes("/body?"));

it("keeps the pagination snapshot when a background refresh races with loading the next page", async () => {
  const { api, client, submit } = setup();
  await screen.findByRole("button", { name: /活动一/ });
  const scope = ["home", "project", "instance"];
  const headKey = [...scope, "async-activities", "head", "ongoing"];
  const initial = client.getQueryData<ActivityListPage>(headKey)!;
  let completePage!: (page: ActivityListPage) => void;
  const nextPage = new Promise<ActivityListPage>((resolve) => {
    completePage = resolve;
  });
  let head = { ...initial, sampled_at: "2026-11-11T00:00:00Z" };
  vi.mocked(api.get).mockImplementation(async (path) =>
    path.includes("cursor=") ? nextPage : head,
  );
  fireEvent.click(screen.getByRole("button", { name: "加载更多活动" }));
  await act(() => client.refetchQueries({ queryKey: headKey, exact: true }));
  expect(
    screen.queryByText("有更新，刷新后读取新快照。"),
  ).not.toBeInTheDocument();
  head = {
    ...head,
    items: [{ ...activity, title: "活动一已更新" }],
    counts: { ...head.counts, ongoing: 3 },
    next_cursor: "new-boundary",
  };
  await act(() => client.refetchQueries({ queryKey: headKey, exact: true }));
  expect(screen.queryByText("活动一已更新")).not.toBeInTheDocument();
  completePage({
    ...initial,
    items: [
      { ...activity, title: "重复活动旧记录" },
      { ...activity, activity_id: "b", title: "第二页活动" },
    ],
    next_cursor: "third",
  });
  await screen.findByText("第二页活动");
  expect(screen.getByText("活动一")).toBeVisible();
  expect(screen.queryByText("重复活动旧记录")).not.toBeInTheDocument();
  expect(screen.getByText("有更新，刷新后读取新快照。")).toBeVisible();
  expect(screen.getByRole("button", { name: "加载更多活动" })).toBeDisabled();
  expect(
    client.getQueryData<ActivityListPage>([...scope, "activity-counts"])?.counts
      .ongoing,
  ).toBe(3);
  fireEvent.click(screen.getByRole("button", { name: "刷新活动列表" }));
  await screen.findByText("活动一已更新");
  expect(screen.queryByText("第二页活动")).not.toBeInTheDocument();
  expect(
    screen.queryByText("有更新，刷新后读取新快照。"),
  ).not.toBeInTheDocument();
  expect(submit).not.toHaveBeenCalled();
});

it("loads only summary until explicit expansion and reads one index page and body segment per action", async () => {
  const { calls, submit } = setup();
  fireEvent.click(await screen.findByRole("button", { name: /活动一/ }));
  await screen.findByRole("heading", { name: "活动一" });
  expect(indexReads(calls)).toHaveLength(0);
  expect(bodyReads(calls)).toHaveLength(0);
  fireEvent.click(screen.getByRole("button", { name: "查看已记录交互" }));
  await screen.findByText("已核实的最终回复");
  expect(indexReads(calls)).toHaveLength(1);
  expect(bodyReads(calls)).toHaveLength(0);
  fireEvent.click(screen.getByRole("button", { name: "展开正文" }));
  await screen.findByRole("button", { name: "继续读取正文" });
  expect(bodyReads(calls)).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "继续读取正文" }));
  await screen.findByText("此条正文完整 · 不代表完整会话");
  expect(bodyReads(calls)).toHaveLength(2);
  fireEvent.click(screen.getByRole("button", { name: "加载更多交互" }));
  await waitFor(() => expect(indexReads(calls)).toHaveLength(2));
  expect(submit).not.toHaveBeenCalled();
});

it("keeps a terminal deep link collapsed and does not refetch loaded history pages on focus", async () => {
  const { calls } = setup(
    "/projects/project/activities?filter=history&activity=a",
    true,
  );
  await screen.findByRole("heading", { name: "活动一" });
  expect(indexReads(calls)).toHaveLength(0);
  fireEvent.click(await screen.findByRole("button", { name: "加载更多活动" }));
  await waitFor(() =>
    expect(
      calls.filter(
        (p) => p.startsWith("/activities?") && p.includes("cursor="),
      ),
    ).toHaveLength(1),
  );
  const before = calls.length;
  await act(async () => {
    focusManager.setFocused(false);
    focusManager.setFocused(true);
  });
  expect(calls).toHaveLength(before);
  expect(indexReads(calls)).toHaveLength(0);
});

it("checks only a lightweight change marker and leaves all further index pages explicit", async () => {
  const { calls, client } = setup("/projects/project/activities?activity=a");
  fireEvent.click(
    await screen.findByRole("button", { name: "查看已记录交互" }),
  );
  await screen.findByText("已核实的最终回复");
  await screen.findByText("有更新，刷新后读取新快照。");
  vi.useFakeTimers();
  await act(async () => {
    await client.invalidateQueries({
      queryKey: ["home", "project", "instance", "a", "reading-changes"],
      exact: true,
    });
  });
  const beforeChanges = calls.filter((p) => p.endsWith("/changes")).length;
  await act(async () => {
    await vi.advanceTimersByTimeAsync(16000);
  });
  expect(calls.filter((p) => p.endsWith("/changes")).length).toBeGreaterThan(
    beforeChanges,
  );
  expect(indexReads(calls)).toHaveLength(1);
  expect(bodyReads(calls)).toHaveLength(0);
  vi.useRealTimers();
  fireEvent.click(screen.getByRole("button", { name: "刷新交互" }));
  await waitFor(() => expect(indexReads(calls)).toHaveLength(2));
  expect(indexReads(calls)[1]).not.toContain("cursor=");
});

it("cancels the selected interaction request when it is collapsed", async () => {
  const { api } = setup("/projects/project/activities?activity=a");
  await screen.findByRole("button", { name: "查看已记录交互" });
  let signal: AbortSignal | undefined;
  const read = vi.mocked(api.get).getMockImplementation()!;
  vi.spyOn(api, "get").mockImplementation((path, s) =>
    path.includes("/interactions?")
      ? ((signal = s), new Promise(() => {}))
      : read(path, s),
  );
  fireEvent.click(screen.getByRole("button", { name: "查看已记录交互" }));
  await waitFor(() => expect(signal).toBeDefined());
  fireEvent.click(screen.getByRole("button", { name: "收起已记录交互" }));
  await waitFor(() => expect(signal?.aborted).toBe(true));
});

it("rejects a body segment whose identity or byte progress differs from its original reference", async () => {
  const api = new Api(identity, "csrf", "home");
  vi.spyOn(api, "get").mockResolvedValue({
    id: "other",
    sha256: longItem.sha256,
    offset: 0,
    next_offset: 1,
    text: "x",
    bytes: 300000,
    complete: false,
  });
  await expect(
    readBody(api, "a", longItem, 0, new AbortController().signal),
  ).rejects.toThrow("正文分段与原引用不符");
});

it("preserves loaded interaction pages when a refresh is cancelled by collapsing", async () => {
  const { api, client } = setup("/projects/project/activities?activity=a");
  fireEvent.click(
    await screen.findByRole("button", { name: "查看已记录交互" }),
  );
  await screen.findByText("已核实的最终回复");
  fireEvent.click(screen.getByRole("button", { name: "加载更多交互" }));
  const key = ["home", "project", "instance", "a", "reading-index"];
  await waitFor(() =>
    expect(client.getQueryData<{ pages: unknown[] }>(key)?.pages).toHaveLength(
      2,
    ),
  );
  const read = vi.mocked(api.get).getMockImplementation()!;
  let signal: AbortSignal | undefined;
  vi.spyOn(api, "get").mockImplementation((path, s) => {
    if (!path.includes("/interactions?")) return read(path, s);
    signal = s;
    return new Promise((_, reject) =>
      s?.addEventListener("abort", () =>
        reject(new DOMException("cancelled", "AbortError")),
      ),
    );
  });
  fireEvent.click(screen.getByRole("button", { name: "刷新交互" }));
  await waitFor(() => expect(signal).toBeDefined());
  fireEvent.click(screen.getByRole("button", { name: "收起已记录交互" }));
  await waitFor(() => expect(signal?.aborted).toBe(true));
  fireEvent.click(screen.getByRole("button", { name: "查看已记录交互" }));
  await screen.findByText("已核实的最终回复");
  expect(client.getQueryData<{ pages: unknown[] }>(key)?.pages).toHaveLength(2);
});

it("retries a failed next body segment explicitly while keeping the first segment", async () => {
  const { api, calls } = setup("/projects/project/activities?activity=a");
  const read = vi.mocked(api.get).getMockImplementation()!;
  let failNext = true;
  vi.spyOn(api, "get").mockImplementation((path, s) => {
    if (path.includes("/body?") && path.includes("offset=262144") && failNext) {
      failNext = false;
      return Promise.reject(new Error("segment offline"));
    }
    return read(path, s);
  });
  fireEvent.click(
    await screen.findByRole("button", { name: "查看已记录交互" }),
  );
  fireEvent.click(await screen.findByRole("button", { name: "展开正文" }));
  fireEvent.click(await screen.findByRole("button", { name: "继续读取正文" }));
  const retry = await screen.findByRole("button", { name: "重试当前分段" });
  expect(
    document.querySelector(".reading-interaction pre")?.textContent,
  ).toHaveLength(262144);
  fireEvent.click(retry);
  await screen.findByText("此条正文完整 · 不代表完整会话");
  expect(
    document.querySelector(".reading-interaction pre")?.textContent,
  ).toHaveLength(300000);
  expect(bodyReads(calls)).toHaveLength(2);
});

it("moves narrow-screen focus to details and restores the selected card and scroll on return", async () => {
  vi.stubGlobal("innerWidth", 320);
  vi.stubGlobal("scrollY", 900);
  const scroll = vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  setup();
  const card = await screen.findByRole("button", { name: /活动一/ });
  card.focus();
  fireEvent.click(card);
  const back = await screen.findByRole("button", { name: "返回活动列表" });
  expect(back).toHaveFocus();
  fireEvent.click(back);
  expect(card).toHaveFocus();
  expect(scroll).toHaveBeenLastCalledWith({ top: 900 });
});

it("opens candidate conversation on demand and prepends earlier pages in native order", async () => {
  const { calls } = setup(
    "/projects/project/activities?activity=a",
    false,
    true,
  );
  const expand = await screen.findByRole("button", { name: "查看对话" });
  expect(calls.filter((path) => path.includes("/conversation"))).toHaveLength(
    0,
  );
  fireEvent.click(expand);
  await screen.findByText("最新的助手消息");
  expect(calls.filter((path) => path.includes("/conversation"))).toHaveLength(
    1,
  );
  fireEvent.click(screen.getByRole("button", { name: "加载更早消息" }));
  await screen.findByText("更早的助手消息");
  const panel = screen.getByRole("region", { name: "原会话对话" });
  expect(
    Array.from(panel.querySelectorAll("article p"), (el) => el.textContent),
  ).toEqual(["更早的助手消息", "最新的助手消息"]);
  expect(screen.getByText("原会话仅有部分历史可读。")).toBeVisible();
  expect(calls.filter((path) => path.includes("/conversation"))).toHaveLength(
    2,
  );
  fireEvent.click(screen.getByRole("button", { name: "展开消息正文" }));
  await screen.findByText("最新的助手消息完整正文");
  expect(calls.filter((path) => path.includes("cursor=body"))).toHaveLength(1);
});
