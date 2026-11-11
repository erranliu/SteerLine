import { afterEach, expect, it, vi } from "vitest";
import { useState } from "react";
import { useWorkspace, WorkspaceContext } from "../../app/context";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { NavigationGuard } from "../../shared/ui/navigation-guard";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, testClient } from "../../test/workspace";
import { Activities } from "./activities";
import {
  provisionalActivities,
  activityDestination,
  type Activity,
  type ActivityPage,
} from "./model";
import type { StoredRequest } from "../../shared/api/types";
import type { CandidateStart } from "../../app/use-candidate-starts";

const clients: ReturnType<typeof testClient>[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((c) => c.clear());
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  document.body.style.overflow = "";
  document.getElementById("root")?.remove();
});
const activity: Activity = {
  activity_id: "formal",
  project_id: "project",
  source: "task_candidate",
  source_id: "start",
  kind: "task_execution",
  request_ids: ["start"],
  title: "任务 #1 · 候选 2",
  state: "running",
  needs_attention: false,
  phase: "实施中",
  observation: "current",
  details_ref: { kind: "candidate", target: "start", task_id: "1" },
};
it("shows content classification without replacing the actual business phase", async () => {
  const current = {
    ...activity,
    source: "roadmap",
    kind: "roadmap_documents",
    steer_agent: { type: "material", operation: "generate_documents" },
    phase: "等待文档 PR 合并",
  };
  setup(async () => page([current]));
  fireEvent.click(await screen.findByRole("button", { name: /^活动 ·/ }));
  expect(await screen.findByLabelText("SteerAgent 能力")).toHaveTextContent(
    "材料生成",
  );
  expect(screen.getByText("等待文档 PR 合并")).toBeVisible();
});
it.each(["future", "constructor", "__proto__", "toString"])(
  "keeps an unknown classification %s from changing activity rendering",
  async (type) => {
    setup(async () =>
      page([{ ...activity, steer_agent: { type, operation: "future" } }]),
    );
    fireEvent.click(await screen.findByRole("button", { name: /^活动 ·/ }));
    expect(await screen.findByText(activity.title)).toBeVisible();
    expect(screen.getByText("实施中")).toBeVisible();
    expect(screen.queryByLabelText("SteerAgent 能力")).toBeNull();
  },
);
it.each([
  ["roadmap_parse", "提交 Roadmap 草稿解析", "roadmap-source", "source"],
  [
    "roadmap_generate_documents",
    "提交 Spec / Plan / 验收文档生成",
    "roadmap",
    "record",
  ],
  ["roadmap_publish", "提交 Roadmap 文档发布", "roadmap", "record"],
  ["roadmap_admit", "提交 Roadmap 入图", "roadmap", "record"],
])(
  "tracks %s until its original roadmap record is linked",
  (action, title, kind, query) => {
    const request = {
      envelope: {
        request_id: "request",
        project_id: "project",
        instance_id: "instance",
        action,
        target_id: "target",
      },
    } as StoredRequest;
    const provisional = provisionalActivities(
      "project",
      "instance",
      [request],
      [],
      {},
      [],
    );
    expect(provisional).toHaveLength(1);
    expect(provisional[0].title).toBe(title);
    expect(provisional[0].details_ref.kind).toBe(kind);
    expect(activityDestination("project", provisional[0])).toBe(
      `/projects/project/roadmaps?${query}=target`,
    );
    const official = {
      ...activity,
      source: "roadmap",
      request_ids: ["request"],
      details_ref: { kind: "roadmap", target: "original-record" },
    };
    expect(
      provisionalActivities(
        "project",
        "instance",
        [request],
        [],
        { request: official },
        [],
      ),
    ).toHaveLength(0);
    expect(activityDestination("project", official)).toBe(
      "/projects/project/roadmaps?record=original-record",
    );
    expect(
      provisionalActivities("project", "new-instance", [request], [], {}, []),
    ).toHaveLength(0);
  },
);
function page(items: Activity[] = []): ActivityPage {
  return {
    project_id: "project",
    instance_id: "instance",
    sampled_at: "2026-11-06T01:00:00Z",
    items,
    request_links: {},
    sources: [{ source: "task_candidate", complete: true }],
    counts: { ongoing: items.length, attention: 0, recent: 0 },
    counts_known: true,
    has_more: false,
  };
}
function setup(data: () => Promise<ActivityPage> = async () => page()) {
  const api = new Api(identity, "csrf", "home");
  const read = vi.spyOn(api, "get").mockImplementation(data);
  const submit = vi.spyOn(api, "submit");
  const client = testClient();
  clients.push(client);
  mountFeature(<Activities />, api, client);
  return { api, read, submit, client };
}
it("merges local waiting, journal and formal IDs once, without reviving old-instance requests", () => {
  const local: CandidateStart = {
    id: "start",
    taskID: "1",
    title: "task",
    profile: "profile",
    status: "等待首次启动确认",
    phase: "waiting",
    participating: true,
  };
  const stored = {
    envelope: {
      request_id: "start",
      project_id: "project",
      instance_id: "instance",
      action: "start_candidate",
      target_id: "1",
    },
    created_at: "2026-11-06T01:00:00Z",
  } as StoredRequest;
  expect(
    provisionalActivities("project", "instance", [], [local], {}, []),
  ).toHaveLength(1);
  expect(
    provisionalActivities("project", "instance", [stored], [local], {}, []),
  ).toHaveLength(1);
  expect(
    provisionalActivities(
      "project",
      "instance",
      [stored],
      [local],
      { start: activity },
      [],
    ),
  ).toHaveLength(0);
  expect(
    provisionalActivities("project", "new-instance", [stored], [], {}, []),
  ).toHaveLength(0);
});
it("keeps a zero-activity entry, opens only on click, closes with focus restored, and sends no command", async () => {
  const { submit } = setup();
  const entry = await screen.findByRole("button", {
    name: /活动 · 0 项进行中/,
  });
  expect(screen.queryByRole("region", { name: /活动中心/ })).toBeNull();
  fireEvent.click(entry);
  const panel = await screen.findByRole("region", { name: /活动中心/ });
  expect(await screen.findByText("暂无进行中的活动")).toBeVisible();
  fireEvent.keyDown(panel, { key: "Escape" });
  await waitFor(() => expect(entry).toHaveFocus());
  expect(screen.queryByRole("region", { name: /活动中心/ })).toBeNull();
  expect(submit).not.toHaveBeenCalled();
});
it("preserves last valid work on a source failure, without changing its business state", async () => {
  let value = page([activity]);
  const { client } = setup(async () => value);
  fireEvent.click(
    await screen.findByRole("button", { name: /活动 · 1 项进行中/ }),
  );
  expect(await screen.findByText(activity.title)).toBeVisible();
  value = {
    ...page(),
    counts_known: false,
    sources: [
      {
        source: "task_candidate",
        complete: false,
        diagnostic: "read unavailable",
      },
    ],
  };
  await act(async () => {
    await client.invalidateQueries({
      queryKey: ["home", "project", "instance", "async-activities"],
    });
  });
  expect(screen.getByText(activity.title)).toBeVisible();
  expect(screen.getByText("实施中")).toBeVisible();
  expect(await screen.findByText(/部分来源不可用/)).toBeVisible();
});
it("uses a modal drawer on narrow screens and restores background interaction on close", async () => {
  vi.stubGlobal("matchMedia", () => ({
    matches: true,
    addEventListener() {},
    removeEventListener() {},
  }));
  const root = document.createElement("div");
  root.id = "root";
  document.body.append(root);
  setup();
  const entry = await screen.findByRole("button", {
    name: /活动 · 0 项进行中/,
  });
  fireEvent.click(entry);
  const panel = await screen.findByRole("dialog", { name: /活动中心/ });
  expect(panel).toHaveAttribute("aria-modal", "true");
  expect(root.inert).toBe(true);
  expect(document.body.style.overflow).toBe("hidden");
  fireEvent.click(screen.getByRole("button", { name: "关闭活动中心" }));
  await waitFor(() => expect(entry).toHaveFocus());
  expect(root.inert).toBe(false);
  expect(document.body.style.overflow).toBe("");
});

it("keeps loaded pages when a new journal request requires an association lookup", async () => {
  const api = new Api(identity, "csrf", "home");
  const reads = vi.spyOn(api, "get").mockImplementation(async (path) => {
    const url = new URL(String(path), "http://test");
    if (url.searchParams.has("request_ids")) {
      expect(url.searchParams.get("mode")).toBe("links");
      expect(url.searchParams.has("filter")).toBe(false);
      expect(url.searchParams.has("limit")).toBe(false);
      expect(url.searchParams.has("cursor")).toBe(false);
      return page();
    }
    const second = url.searchParams.has("cursor");
    return {
      ...page(
        Array.from({ length: 50 }, (_, n) => ({
          ...activity,
          activity_id: `row-${n + (second ? 50 : 0)}`,
          title: `loaded item ${n + (second ? 50 : 0)}`,
        })),
      ),
      has_more: !second,
      next_cursor: second ? undefined : "page2",
    };
  });
  const submit = vi.spyOn(api, "submit");
  const client = testClient();
  clients.push(client);
  function Harness() {
    const workspace = useWorkspace();
    const [requests, setRequests] = useState<StoredRequest[]>([]);
    return (
      <WorkspaceContext.Provider value={{ ...workspace, requests }}>
        <button
          onClick={() =>
            setRequests([
              {
                envelope: {
                  request_id: "new",
                  project_id: "project",
                  instance_id: "instance",
                  action: "start_candidate",
                  target_id: "1",
                },
                created_at: "2026-11-06T01:00:00Z",
              } as StoredRequest,
            ])
          }
        >
          append request
        </button>
        <Activities />
      </WorkspaceContext.Provider>
    );
  }
  mountFeature(<Harness />, api, client);
  fireEvent.click(
    await screen.findByRole("button", { name: /活动 · 50 项进行中/ }),
  );
  fireEvent.click(await screen.findByRole("button", { name: "加载更多" }));
  expect(await screen.findByText("loaded item 99")).toBeVisible();
  const pageReads = reads.mock.calls.filter(
    ([path]) => !String(path).includes("request_ids="),
  ).length;
  fireEvent.click(screen.getByText("append request"));
  await waitFor(() =>
    expect(
      reads.mock.calls.some(([path]) =>
        String(path).includes("request_ids=new"),
      ),
    ).toBe(true),
  );
  expect(screen.getByText("loaded item 0")).toBeVisible();
  expect(screen.getByText("loaded item 99")).toBeVisible();
  expect(
    reads.mock.calls.filter(([path]) => !String(path).includes("request_ids="))
      .length,
  ).toBe(pageReads);
  expect(submit).not.toHaveBeenCalled();
});

it("keeps the drawer open when navigation protection cancels a details jump", async () => {
  const api = new Api(identity, "csrf", "home");
  vi.spyOn(api, "get").mockResolvedValue(page([activity]));
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
  const client = testClient();
  clients.push(client);
  function Layout() {
    return (
      <WorkspaceContext.Provider
        value={
          { api, identity, online: true } as ReturnType<typeof useWorkspace>
        }
      >
        <NavigationGuard dirty />
        <Activities />
      </WorkspaceContext.Provider>
    );
  }
  const router = createMemoryRouter([{ path: "*", element: <Layout /> }], {
    initialEntries: ["/original"],
  });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  fireEvent.click(
    await screen.findByRole("button", { name: /活动 · 1 项进行中/ }),
  );
  fireEvent.click(await screen.findByRole("button", { name: "查看详情" }));
  await waitFor(() => expect(confirm).toHaveBeenCalledOnce());
  expect(router.state.location.pathname).toBe("/original");
  expect(screen.getByRole("region", { name: /活动中心/ })).toBeVisible();
  confirm.mockReturnValue(true);
  fireEvent.click(screen.getByRole("button", { name: "查看详情" }));
  await waitFor(() =>
    expect(router.state.location.pathname).toBe("/projects/project/runs"),
  );
  expect(screen.queryByRole("region", { name: /活动中心/ })).toBeNull();
  router.dispose();
});

it("copies a concise roadmap identity reference without source paths or diagnostics", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal("navigator", { clipboard: { writeText } });
  setup(async () =>
    page([
      {
        ...activity,
        source: "roadmap",
        kind: "roadmap_parse",
        source_id: "original-record",
        title: "解析 Roadmap 草稿 · SteerlineFlow/Roadmap/drafts/private.md",
        diagnostic: "long diagnostic",
        details_ref: { kind: "roadmap", target: "original-record" },
        execution_ref: {
          request_id: "original-record",
          run_id: "original-run",
          attempt_id: "original-attempt",
        },
      },
    ]),
  );
  fireEvent.click(
    await screen.findByRole("button", { name: /^活动 · 1 项进行中/ }),
  );
  fireEvent.click(await screen.findByRole("button", { name: "复制执行信息" }));
  await screen.findByText("已复制执行信息");
  expect(writeText.mock.calls[0][0]).toContain("Roadmap 解析");
  expect(writeText.mock.calls[0][0]).toContain("Runner Run ID：original-run");
  expect(writeText.mock.calls[0][0]).not.toContain("SteerlineFlow");
  expect(writeText.mock.calls[0][0]).not.toContain("long diagnostic");
});

it("passes the original activity identity from a drawer card to the independent reading page", async () => {
  const api = new Api(identity, "csrf", "home");
  const reads = vi.spyOn(api, "get").mockResolvedValue(page([activity]));
  const client = testClient();
  clients.push(client);
  const router = createMemoryRouter(
    [
      {
        path: "*",
        element: (
          <WorkspaceContext.Provider
            value={
              { api, identity, online: true } as ReturnType<typeof useWorkspace>
            }
          >
            <Activities />
            <main>
              <h1>destination</h1>
            </main>
          </WorkspaceContext.Provider>
        ),
      },
    ],
    { initialEntries: ["/original"] },
  );
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  fireEvent.click(
    await screen.findByRole("button", { name: /活动 · 1 项进行中/ }),
  );
  fireEvent.click(await screen.findByRole("button", { name: "在独立页阅读" }));
  await waitFor(() =>
    expect(router.state.location.pathname + router.state.location.search).toBe(
      "/projects/project/activities?activity=formal",
    ),
  );
  expect(screen.queryByRole("region", { name: /活动中心/ })).toBeNull();
  expect(
    reads.mock.calls.some(
      ([path]) => path.includes("/interactions") || path.includes("/body"),
    ),
  ).toBe(false);
  router.dispose();
});
