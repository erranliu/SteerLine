import { afterEach, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { QueryClientProvider } from "@tanstack/react-query";
import { WorkspaceContext, type Workspace } from "../../app/context";
import { useWorkspace } from "../../app/context";
import { useOriginalRequests } from "../../app/use-original-requests";
import {
  contentPending,
  currentRoadmapRecord,
  roadmapProgress,
} from "./workflow";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, testClient } from "../../test/workspace";
import type { Envelope, StoredRequest } from "../../shared/api/types";
import {
  Roadmaps,
  roadmapQueryKey,
  type RoadmapRecord,
  type RoadmapView,
} from "./roadmaps";

// Layout and real canvas interactions are covered by the production-build browser test.
vi.mock("@xyflow/react", () => ({
  ReactFlow: () => <div>依赖图画布</div>,
  Controls: () => null,
  Handle: () => null,
  Position: { Left: "left", Right: "right" },
  MarkerType: { ArrowClosed: "arrowclosed" },
}));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
function LocationProbe() {
  return <div data-testid="location">{useLocation().search}</div>;
}
function setup(
  state = "review",
  online = true,
  requests: StoredRequest[] = [],
  configure?: (view: RoadmapView, record: RoadmapRecord) => void,
  observed = false,
  pendingRequests?: () => Envelope[],
) {
  const source = {
    id: "b".repeat(32),
    path: "SteerlineFlow/Roadmap/drafts/control.md",
    body: "# 控制\nT1 候选控制\n前置：已有正式 #8",
    digest: "c".repeat(64),
    head: "d".repeat(40),
    modified: true,
  };
  const record: RoadmapRecord = {
    id: "a".repeat(32),
    source,
    revision: 3,
    created_at: "2026-10-26T00:00:00Z",
    state,
    existing: [{ id: "8", title: "固定调度", status: "open" }],
    documents: [],
    formal_mapping: {},
    cancelled: false,
    confirmed_digest: state === "confirmed" ? "confirmed" : undefined,
    analysis: {
      tasks: [
        {
          local_key: "T1",
          key_origin: "markdown",
          title: "候选控制",
          goal: "控制候选",
          scope: "当前任务",
          constraints: "保持归属",
          acceptance: "停止核实",
          evidence: "T1 候选控制",
          original_dependencies: { local: [], formal: [] },
          proposed_dependencies: { local: [], formal: ["8"] },
          reason: "建议补充固定调度前置",
          existing_task_id: "",
        },
      ],
      questions: [],
      overlap: "无重复覆盖",
    },
  };
  const view: RoadmapView = {
    preparation: {
      commit: source.head,
      ready: true,
      local_ready: true,
      branch: "main",
      files: [],
    },
    sources: [{ ...source, body: "" }],
    records: [record],
    profiles: [{ id: "local", name: "本机 Codex" }],
    documents_available: true,
    documents_reason: "",
  };
  const api = new Api(identity, "csrf");
  const get = vi
    .spyOn(api, "get")
    .mockImplementation(async (route) =>
      route === "/roadmaps"
        ? structuredClone(view)
        : route.includes("/sources/")
          ? route.endsWith(source.id)
            ? source
            : view.sources.find((s) => route.endsWith(s.id))
          : (view.records.find((r) => route.endsWith("/" + r.id)) as never),
    );
  const submit = vi.spyOn(api, "submit").mockImplementation(
    async (envelope) =>
      ({
        request_id: envelope.request_id,
        status: "applied",
        roadmap: {
          ...record,
          id:
            envelope.action === "roadmap_parse"
              ? envelope.request_id
              : record.id,
        },
      }) as never,
  );
  configure?.(view, record);
  const client = testClient();
  function ObservedRoadmaps() {
    const workspace = useWorkspace();
    const original = useOriginalRequests(api, online);
    return (
      <WorkspaceContext.Provider
        value={{
          ...workspace,
          submit: original.submit,
          requests: original.observedRequests,
          pendingRequests: original.pendingRequests,
        }}
      >
        <Roadmaps />
      </WorkspaceContext.Provider>
    );
  }
  mountFeature(
    <>
      {observed ? <ObservedRoadmaps /> : <Roadmaps />}
      <LocationProbe />
    </>,
    api,
    client,
    online,
    { requests, pendingRequests },
  );
  return { api, get, submit, source, record, view, client };
}

it("keeps a verified empty draft list visible during refresh and labels a failed refresh", async () => {
  const { api, get, client } = setup("review", true, [], (view) => {
    view.sources = [];
    view.records = [];
  });
  const empty = await screen.findByText("drafts 目录下暂无 Markdown 草稿。");
  const read = get.getMockImplementation()!;
  let fail!: (error: Error) => void;
  const refreshing = new Promise<never>((_, reject) => {
    fail = reject;
  });
  get.mockImplementation((path, signal) =>
    path === "/roadmaps" ? refreshing : read(path, signal),
  );
  const queryKey = roadmapQueryKey(
    api.home,
    identity.project_id,
    identity.instance_id,
  );
  act(() => {
    void client.refetchQueries({
      queryKey,
      exact: true,
    });
  });
  expect(client.getQueryState(queryKey)?.fetchStatus).toBe("fetching");
  expect(empty).toBeVisible();
  fail(new Error("草稿暂时无法读取"));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Roadmap 刷新失败，保留已读取的内容。",
  );
  expect(empty).toBeVisible();
});

it("keeps static initialization blocked until the acknowledged material state is read", async () => {
  const { submit, view } = setup(
    "review",
    true,
    [],
    (view) => {
      view.preparation.ready = false;
      view.preparation.local_ready = false;
    },
    true,
  );
  const initialize = await screen.findByRole("button", {
    name: "初始化静态材料",
  });
  await waitFor(() => expect(initialize).toBeEnabled());
  fireEvent.click(initialize);
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  await screen.findAllByText("已受理，正在读取工作信息");
  expect(screen.getByRole("button", { name: "初始化静态材料" })).toBeDisabled();
  view.preparation.local_ready = true;
  fireEvent.click(screen.getByRole("button", { name: "刷新 Markdown 与观察" }));
  await waitFor(() =>
    expect(screen.queryByText("已受理，正在读取工作信息")).toBeNull(),
  );
  expect(submit).toHaveBeenCalledTimes(1);
});

it("does not block another draft while the first draft is submitting", async () => {
  const { submit } = setup(
    "review",
    true,
    [],
    (view, record) => {
      view.sources.push({
        ...record.source,
        id: "other",
        path: "SteerlineFlow/Roadmap/drafts/other.md",
      });
    },
    true,
  );
  submit.mockImplementation(() => new Promise(() => {}) as never);
  const first = await screen.findByRole("button", { name: "重新解析草稿" });
  await waitFor(() => expect(first).toBeEnabled());
  fireEvent.click(first);
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  fireEvent.click(screen.getByRole("button", { name: /other\.md/ }));
  const second = await screen.findByRole("button", {
    name: "Runner generate · 解析任务与依赖",
  });
  await waitFor(() => expect(second).toBeEnabled());
  fireEvent.click(second);
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(2));
  expect(submit.mock.calls[1][0].target_id).toBe("other");
});
it("blocks duplicate parses synchronously and keeps the accepted handoff locked until its own record is read", async () => {
  const { submit, view, source, record, get } = setup(
    "review",
    true,
    [],
    undefined,
    true,
  );
  let finish!: (value: unknown) => void;
  submit.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }) as never,
  );
  const button = await screen.findByRole("button", { name: "重新解析草稿" });
  await waitFor(() => expect(button).toBeEnabled());
  fireEvent.click(button);
  fireEvent.click(button);
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  await screen.findAllByText("正在提交解析…");
  expect(button).toBeDisabled();
  const envelope = submit.mock.calls[0][0];
  const next = {
    ...record,
    id: envelope.request_id,
    state: "queued",
    revision: 1,
    created_at: new Date().toISOString(),
  };
  finish({ ...envelope, status: "applied", roadmap: next });
  await screen.findAllByText("已受理，正在读取工作信息");
  expect(screen.getByRole("button", { name: "重新解析草稿" })).toBeDisabled();
  view.records.push(next);
  fireEvent.click(screen.getByRole("button", { name: "刷新 Markdown 与观察" }));
  await screen.findByText("解析草稿中", { selector: "strong" });
  expect(screen.queryByText("已受理，正在读取工作信息")).toBeNull();
  expect(submit).toHaveBeenCalledTimes(1);
  expect(get).toHaveBeenCalledWith("/roadmaps/sources/" + source.id);
});

it("keeps observe and stop available for an unknown parse, without trusting activity queries", async () => {
  const { api, record, submit } = setup("unknown");
  cleanup();
  const envelope = api.envelope("roadmap_parse", record.source.id, 1, {});
  mountFeature(<Roadmaps />, api, testClient(), true, {
    busy: true,
    requests: [
      {
        envelope,
        created_at: new Date().toISOString(),
        response: {
          ...envelope,
          status: "outcome_unknown",
          sampled_at: new Date().toISOString(),
        },
      },
    ],
  });
  expect(
    await screen.findByRole("button", { name: "查询原操作结果" }),
  ).toBeEnabled();
  expect(
    screen.getByRole("button", { name: "停止当前内容请求" }),
  ).toBeEnabled();
  expect(screen.getByRole("button", { name: "重新解析草稿" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "查询原操作结果" }));
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0].action).toBe("roadmap_observe");
});
it.each([false, true])(
  "opens an activity record in its own draft and preserves current/history actions (historical=%s)",
  async (historical) => {
    const { api, get, view, record } = setup();
    cleanup();
    get.mockClear();
    view.sources.unshift({
      ...record.source,
      id: "another",
      path: "SteerlineFlow/Roadmap/drafts/another.md",
      body: "# other draft",
    });
    if (historical)
      view.records.push({
        ...record,
        id: "e".repeat(32),
        parent_id: record.id,
        purpose: "documents",
        state: "documents-review",
        created_at: "2026-11-06T00:00:00Z",
      });
    const client = testClient();
    render(
      <QueryClientProvider client={client}>
        <WorkspaceContext.Provider
          value={
            {
              api,
              identity,
              online: true,
              busy: false,
              requests: [],
            } as unknown as Workspace
          }
        >
          <MemoryRouter
            initialEntries={[`/projects/project/roadmaps?record=${record.id}`]}
          >
            <Roadmaps />
          </MemoryRouter>
        </WorkspaceContext.Provider>
      </QueryClientProvider>,
    );
    await waitFor(() =>
      expect(get).toHaveBeenCalledWith("/roadmaps/" + record.id),
    );
    expect(get).toHaveBeenCalledWith("/roadmaps/sources/" + record.source.id);
    expect(screen.getByLabelText("查看历史记录")).toHaveValue(record.id);
    if (historical) {
      expect(
        await screen.findByRole("button", { name: "返回当前流程" }),
      ).toBeVisible();
      expect(
        screen.getByRole("button", { name: "重新解析草稿" }),
      ).toBeDisabled();
      fireEvent.change(screen.getByLabelText("查看历史记录"), {
        target: { value: "e".repeat(32) },
      });
      await waitFor(() =>
        expect(
          screen.queryByRole("button", { name: "返回当前流程" }),
        ).toBeNull(),
      );
    } else {
      await waitFor(() =>
        expect(
          screen.getByRole("button", {
            name: /确认任务依赖并生成 Spec \/ Plan/,
          }),
        ).toBeEnabled(),
      );
      expect(screen.queryByRole("button", { name: "返回当前流程" })).toBeNull();
    }
    client.clear();
  },
);
it("follows the current document generation after acting on a parse activity link", async () => {
  const { api, view, record, submit, get } = setup();
  cleanup();
  view.documents_available = true;
  get.mockImplementation(
    async (route) =>
      structuredClone(
        route === "/roadmaps"
          ? view
          : route.includes("/sources/")
            ? record.source
            : view.records.find((r) => route.endsWith("/" + r.id)),
      ) as never,
  );
  submit.mockImplementation(async (envelope) => {
    if (envelope.action === "roadmap_generate_documents") {
      record.state = "confirmed";
      record.confirmed_digest = "confirmed";
      view.records.push({
        ...record,
        id: "documents-from-activity",
        parent_id: record.id,
        purpose: "documents",
        state: "queued",
        created_at: "2026-11-06T00:00:00Z",
      });
    }
    return { request_id: envelope.request_id, status: "applied" } as never;
  });
  const client = testClient();
  render(
    <QueryClientProvider client={client}>
      <WorkspaceContext.Provider
        value={
          {
            api,
            identity,
            online: true,
            busy: false,
            requests: [],
            submit: api.submit.bind(api),
            refresh: vi.fn(),
          } as unknown as Workspace
        }
      >
        <MemoryRouter
          initialEntries={[`/projects/project/roadmaps?record=${record.id}`]}
        >
          <Roadmaps />
          <LocationProbe />
        </MemoryRouter>
      </WorkspaceContext.Provider>
    </QueryClientProvider>,
  );
  const confirm = await screen.findByRole("button", {
    name: /确认任务依赖并生成 Spec \/ Plan/,
  });
  await waitFor(() => expect(confirm).toBeEnabled());
  fireEvent.click(confirm);
  await waitFor(() =>
    expect(screen.getByTestId("location")).not.toHaveTextContent("record="),
  );
  expect(screen.getByTestId("location")).toHaveTextContent(
    "source=" + record.source.id,
  );
  await waitFor(() =>
    expect(screen.getByRole("status")).toHaveTextContent("生成文档中"),
  );
  expect(screen.queryByRole("button", { name: "返回当前流程" })).toBeNull();
  expect(submit).toHaveBeenCalledTimes(1);
  client.clear();
});
it("reads Markdown and shows original versus proposed dependencies without allocating or calling a model on mount", async () => {
  const { submit } = setup();
  expect(await screen.findByDisplayValue(/T1 候选控制/)).toHaveAttribute(
    "readonly",
  );
  expect(
    await screen.findByRole("link", { name: "正式 #8 · 固定调度" }),
  ).toHaveAttribute("href", "/projects/" + identity.project_id + "/dag?task=8");
  expect(screen.getByText("建议补充固定调度前置")).toBeVisible();
  expect(
    screen.queryByRole("button", { name: "重试生成 Spec / Plan" }),
  ).not.toBeInTheDocument();
  expect(screen.getByText("入 DAG 时分配")).toBeVisible();
  expect(submit).not.toHaveBeenCalled();
  fireEvent.click(
    screen.getByRole("button", { name: /确认任务依赖并生成 Spec \/ Plan/ }),
  );
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "roadmap_generate_documents",
        target_id: "a".repeat(32),
        expected_version: 3,
        payload: {},
      }),
    ),
  );
  expect(screen.queryByText("#91")).toBeNull();
});
it("shows mixed original references separately from proposed formal dependencies", async () => {
  const { record } = setup();
  const task = record.analysis!.tasks[0];
  task.original_dependencies = { local: ["T0"], formal: ["8"] };
  task.proposed_dependencies = { local: [], formal: ["8"] };
  task.reason = "T0 已关联正式 #8，合并重复依赖";
  record.analysis!.tasks.push({
    ...task,
    local_key: "T0",
    title: "原有基础",
    existing_task_id: "8",
    original_dependencies: { local: [], formal: [] },
    proposed_dependencies: { local: [], formal: [] },
    reason: "",
  });
  fireEvent.click(
    await screen.findByRole("button", { name: "查看 T1 · 候选控制" }),
  );
  const detail = screen.getByRole("region", { name: "所选任务详情" });
  expect(within(detail).getByText("本 Roadmap T0、正式 #8")).toBeVisible();
  expect(within(detail).getByText("正式 #8", { exact: true })).toBeVisible();
  expect(within(detail).getByText(task.reason)).toBeVisible();
  expect(screen.getByText("与草稿依赖一致")).toBeVisible();
});
it("keeps dependency comparison in task details and confirms the whole proposed snapshot without view switches", async () => {
  const { submit, get } = setup();
  fireEvent.click(await screen.findByRole("button", { name: "查看变更 · T1" }));
  const detail = screen.getByRole("region", { name: "所选任务详情" });
  expect(within(detail).getByText("T1 候选控制")).toBeVisible();
  expect(within(detail).getByText("#8 → T1 · 新增")).toBeVisible();
  expect(within(detail).getByText("草稿原有依赖")).toBeVisible();
  expect(within(detail).getByText("无", { exact: true })).toBeVisible();
  expect(within(detail).getByText("正式 #8", { exact: true })).toBeVisible();
  expect(screen.queryByRole("group", { name: "依赖视图" })).toBeNull();
  for (const name of ["Runner 建议", "草稿原始", "差异"]) {
    expect(screen.queryByRole("button", { name })).toBeNull();
  }
  expect(submit).not.toHaveBeenCalled();
  expect(get.mock.calls.some(([path]) => path.startsWith("/graph"))).toBe(
    false,
  );
  fireEvent.click(
    screen.getByRole("button", { name: /确认任务依赖并生成 Spec \/ Plan/ }),
  );
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0]).toMatchObject({
    action: "roadmap_generate_documents",
    target_id: "a".repeat(32),
    expected_version: 3,
    payload: {},
  });
});
it.each(["questions", "stale", "offline", "capability", "preparation"])(
  "keeps the original confirmation guard with a readable %s preview",
  async (guard) => {
    const { submit } = setup(
      "review",
      guard !== "offline",
      [],
      (view, record) => {
        if (guard === "questions")
          record.analysis!.questions = ["需要明确依赖范围"];
        if (guard === "stale")
          record.source = { ...record.source, body: "旧原文", digest: "old" };
        if (guard === "capability") view.documents_available = false;
        if (guard === "preparation") view.preparation.ready = false;
      },
    );
    // Offline has no cached server data in this fixture and therefore no preview.
    if (guard !== "offline") {
      expect(await screen.findByLabelText("依赖预览图")).toBeVisible();
      if (guard === "questions")
        expect(screen.getByText("需要明确依赖范围")).toBeVisible();
    }
    const confirm = screen.queryByRole("button", {
      name: /确认任务依赖并生成 Spec \/ Plan/,
    });
    if (confirm) expect(confirm).toBeDisabled();
    expect(submit).not.toHaveBeenCalled();
  },
);
it("retains confirmation after generation preparation fails and retries once without reconfirming", async () => {
  const { submit, record, view } = setup("review", true, [], undefined, true);
  submit.mockImplementation(async (envelope) => {
    if (record.state === "review") {
      record.state = "confirmed";
      record.confirmed_digest = "confirmed";
      record.revision++;
      return {
        request_id: envelope.request_id,
        status: "rejected",
        diagnostic: "固定生成规范不可用",
      } as never;
    }
    const document = {
      ...record,
      id: envelope.request_id,
      parent_id: record.id,
      purpose: "documents",
      state: "queued",
      revision: 1,
      created_at: new Date().toISOString(),
    };
    view.records.push(document);
    return {
      request_id: envelope.request_id,
      status: "applied",
      roadmap: document,
    } as never;
  });
  const confirm = await screen.findByRole("button", {
    name: "确认任务依赖并生成 Spec / Plan",
  });
  await waitFor(() => expect(confirm).toBeEnabled());
  fireEvent.click(confirm);
  fireEvent.click(confirm);
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "固定生成规范不可用",
  );
  const retry = await screen.findByRole("button", {
    name: "重试生成 Spec / Plan",
  });
  await waitFor(() => expect(retry).toBeEnabled());
  expect(submit).toHaveBeenCalledTimes(1);
  expect(record.confirmed_digest).toBe("confirmed");
  fireEvent.click(retry);
  fireEvent.click(retry);
  await waitFor(() =>
    expect(screen.getByRole("status")).toHaveTextContent("生成文档中"),
  );
  expect(submit).toHaveBeenCalledTimes(2);
  expect(submit.mock.calls[0][0]).toMatchObject({
    action: "roadmap_generate_documents",
    target_id: record.id,
    expected_version: 3,
  });
  expect(submit.mock.calls[1][0]).toMatchObject({
    action: "roadmap_generate_documents",
    target_id: record.id,
    expected_version: 4,
  });
});
it("submits an explicit Runner parse with original request identity and no formal IDs", async () => {
  const { submit, source } = setup();
  await screen.findByDisplayValue(/T1 候选控制/);
  fireEvent.click(screen.getByRole("button", { name: "重新解析草稿" }));
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  const envelope = submit.mock.calls[0][0];
  expect(envelope.action).toBe("roadmap_parse");
  expect(envelope.target_id).toBe(source.id);
  expect(envelope.payload).toEqual({
    path: source.path,
    source_digest: source.digest,
    profile: "local",
    request_id: envelope.request_id,
  });
  expect(JSON.stringify(envelope)).not.toContain("task_id");
});
it("keeps confirmed content at local identities while document capability is unavailable", async () => {
  const { submit } = setup("confirmed", true, [], (view) => {
    view.documents_available = false;
    view.documents_reason = "待 Runner 文档能力接入";
  });
  expect(await screen.findByText(/待 Runner 文档能力接入/)).toBeVisible();
  expect(
    screen.getByRole("button", { name: "重试生成 Spec / Plan" }),
  ).toBeDisabled();
  expect(
    screen.queryByRole("button", { name: "正式入 DAG · 分配编号" }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "SteerLine 生成的 JSON" }),
  ).toBeNull();
  expect(screen.getByLabelText("Markdown 原文 · 只读")).toBeVisible();
  expect(screen.queryByRole("link", { name: /#101/ })).toBeNull();
  expect(submit).not.toHaveBeenCalled();
});
it("generates documents only after confirmation when the selected Runner publishes repository support", async () => {
  const { view, submit } = setup("confirmed");
  view.documents_available = true;
  view.documents_reason = "";
  const button = await screen.findByRole("button", {
    name: "重试生成 Spec / Plan",
  });
  await waitFor(() => expect(button).toBeEnabled());
  fireEvent.click(button);
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "roadmap_generate_documents",
        target_id: "a".repeat(32),
        expected_version: 3,
        payload: {},
      }),
    ),
  );
});
it("shows code evidence, acceptance material and the complete publication target before publishing", async () => {
  const { record, submit } = setup("documents-review");
  record.purpose = "documents";
  record.code_commit = "d".repeat(40);
  record.design = "复用已有 Task owner";
  record.investigation = [
    {
      source: "repository",
      path: "logic.go",
      sha256: "e".repeat(64),
      reason: "已有纯函数",
    },
  ];
  record.documents = [
    {
      local_key: "T1",
      tests: ["arithmetic.unit"],
      provides_tests: ["N201-CONTENT"],
      spec: { path: "spec/control.md", body: "# 完整 Spec" },
      plan: { path: "plans/control.plan.md", body: "# 完整 Plan" },
      acceptance: [{ path: "acceptance/control.md", body: "# 验收待执行" }],
    },
  ];
  record.publication_preview = {
    repository: "https://example.invalid/owner/repo",
    branch: "main",
    files: [
      {
        path: "SteerlineFlow/Roadmap/execution-map.md",
        before: "# 原清单",
        after: "# 新清单",
      },
    ],
  };
  await screen.findByText(/logic.go/);
  fireEvent.click(screen.getByText("设计与调查依据"));
  expect(screen.getByText(/logic.go/)).toBeVisible();
  expect(screen.getByText(/本任务将新增测试：N201-CONTENT/)).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "测试安排 JSON" }));
  expect(screen.getByText("plans/control.tests.json")).toBeVisible();
  expect(screen.getByText(/"arithmetic.unit"/)).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "验收材料" }));
  expect(screen.getByText("# 验收待执行")).toBeVisible();
  fireEvent.click(screen.getByText("完整发布内容与目标"));
  expect(
    screen.getByText("https://example.invalid/owner/repo · main"),
  ).toBeVisible();
  expect(
    screen.getByText("SteerlineFlow/Roadmap/execution-map.md"),
  ).toBeVisible();
  expect(submit).not.toHaveBeenCalled();
});
it("shows the specific generation failure and original question", async () => {
  setup("failed", true, [], (_view, record) => {
    record.purpose = "documents";
    record.diagnostic =
      "未取得经核实的完整生成结果；human_input_required；测试 N201-CONTENT 尚未登记，无法满足当前约束";
  });
  expect(
    await screen.findByText(/human_input_required.*N201-CONTENT 尚未登记/),
  ).toBeVisible();
});
it("distinguishes repository and input citations with the same logical path", async () => {
  const { record } = setup("documents-review");
  record.investigation = [
    {
      source: "repository",
      path: "roadmap-inputs/source.md",
      sha256: "e".repeat(64),
      reason: "仓库中的历史文件",
    },
    {
      source: "input_file",
      path: "roadmap-inputs/source.md",
      sha256: "f".repeat(64),
      reason: "本次冻结的原草稿",
    },
  ];
  await screen.findByText(/固定仓库.*仓库中的历史文件/);
  fireEvent.click(screen.getByText("设计与调查依据"));
  expect(screen.getByText(/固定仓库.*仓库中的历史文件/)).toBeVisible();
  expect(screen.getByText(/本次输入.*本次冻结的原草稿/)).toBeVisible();
});
it("disables mutation while offline and does not fall back to file reads or submits", () => {
  const { get, submit } = setup("review", false);
  expect(screen.getByText(/连接待核实/)).toBeVisible();
  expect(
    screen.getByRole("button", { name: "Runner generate · 解析任务与依赖" }),
  ).toBeDisabled();
  expect(get).not.toHaveBeenCalled();
  expect(submit).not.toHaveBeenCalled();
});
it("shows official IDs only from the admitted mapping and keeps local origin", async () => {
  const { record } = setup("admitted");
  record.formal_mapping = { T1: "101" };
  expect(await screen.findByRole("link", { name: "#101" })).toHaveAttribute(
    "href",
    "/projects/" + identity.project_id + "/dag?task=101",
  );
  expect(screen.getByRole("cell", { name: "control.md / T1" })).toBeVisible();
  expect(screen.getByRole("link", { name: "查看正式任务 DAG" })).toBeVisible();
});

it("shows fixed preparation failures and submits explicit initialization without parsing", async () => {
  const { submit } = setup("review", true, [], (view) => {
    Object.assign(view.preparation, {
      ready: false,
      local_ready: false,
      files: [{ path: "SteerlineFlow/Roadmap/parse-rules.md", ready: false }],
    });
  });
  expect(
    await screen.findByRole("heading", { name: "先准备项目静态材料" }),
  ).toBeVisible();
  expect(screen.getByRole("button", { name: "重新解析草稿" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "初始化静态材料" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "roadmap_prepare_static",
        target_id: identity.project_id,
        expected_version: 1,
        payload: { commit: "d".repeat(40) },
      }),
    ),
  );
  expect(submit.mock.calls.some(([e]) => e.action === "roadmap_parse")).toBe(
    false,
  );
});
it("keeps local-only preparation blocked until published and prevents needless regeneration", async () => {
  const { submit } = setup("review", true, [], (view) => {
    view.preparation.ready = false;
  });
  expect(
    await screen.findByText(/材料已在工作树中，请提交并合入 main/),
  ).toBeVisible();
  expect(screen.getByRole("button", { name: "初始化静态材料" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "重新解析草稿" })).toBeDisabled();
  expect(submit).not.toHaveBeenCalled();
});

it.each([
  ["other", false],
  [identity.project_id, true],
])(
  "scopes pending initialization requests to project %s",
  async (owner, blocked) => {
    const request: StoredRequest = {
      envelope: new Api({ ...identity, project_id: owner }, "csrf").envelope(
        "roadmap_prepare_static",
        owner,
        1,
        {},
      ),
      created_at: new Date().toISOString(),
    };
    setup("review", true, [request], (view) => {
      view.preparation.ready = false;
      view.preparation.local_ready = false;
    });
    const button = await screen.findByRole("button", {
      name: "初始化静态材料",
    });
    if (blocked) expect(button).toBeDisabled();
    else expect(button).toBeEnabled();
  },
);

it("keeps current progress and source badge while inspecting read-only history, then restores current actions", async () => {
  const { record, submit } = setup(
    "documents-review",
    true,
    [],
    (view, current) => {
      current.purpose = "documents";
      current.parent_id = "parent";
      current.confirmed_digest = "confirmed";
      const parent = {
        ...current,
        id: "parent",
        purpose: "parse",
        state: "confirmed",
        created_at: "2026-10-25T00:00:00Z",
      };
      view.records.push(parent);
    },
  );
  const progress = await screen.findByRole("region", {
    name: "当前草稿转正进度",
  });
  await waitFor(() =>
    expect(
      within(progress).getByRole("button", { name: "确认发布并合并文档" }),
    ).toBeEnabled(),
  );
  fireEvent.click(screen.getByText("历史记录（2）"));
  fireEvent.change(screen.getByLabelText("查看历史记录"), {
    target: { value: "parent" },
  });
  expect(await screen.findByText("历史状态：任务与依赖已确认")).toBeVisible();
  expect(within(progress).getByRole("status")).toHaveTextContent(
    "待审阅正式文档",
  );
  expect(
    screen.getByRole("button", { name: /control.md.*待审阅正式文档/ }),
  ).toBeVisible();
  expect(screen.getByTestId("location")).toHaveTextContent("record=parent");
  expect(
    screen.queryByRole("button", { name: "重试生成 Spec / Plan" }),
  ).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "重新解析草稿" }));
  expect(submit).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "返回当前流程" }));
  expect(screen.getByTestId("location")).not.toHaveTextContent("record=");
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "确认发布并合并文档" }),
    ).toBeEnabled(),
  );
  fireEvent.click(screen.getByRole("button", { name: "确认发布并合并文档" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "roadmap_publish",
        target_id: record.id,
        expected_version: record.revision,
        payload: { review_digest: "", merge: true },
      }),
    ),
  );
});

it("keeps the confirmed milestones after a failed document attempt and retries against its parse parent", async () => {
  const { submit } = setup("confirmed", true, [], (view, parent) => {
    view.documents_available = true;
    view.records.push({
      ...parent,
      id: "failed-document",
      parent_id: parent.id,
      purpose: "documents",
      state: "failed",
      created_at: "2026-10-27T00:00:00Z",
    });
  });
  const button = await screen.findByRole("button", {
    name: "重试生成 Spec / Plan",
  });
  await waitFor(() => expect(button).toBeEnabled());
  expect(
    screen
      .getByRole("list", { name: "转正阶段" })
      .querySelectorAll(".complete"),
  ).toHaveLength(1);
  fireEvent.click(button);
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "roadmap_generate_documents",
        target_id: "a".repeat(32),
        expected_version: 3,
      }),
    ),
  );
});

it("keeps unknown results on the original operation and disables starting another parse", async () => {
  const { record, submit } = setup("unknown");
  const button = await screen.findByRole("button", { name: "查询原操作结果" });
  await waitFor(() => expect(button).toBeEnabled());
  fireEvent.click(screen.getByText("重新解析与生成方案"));
  expect(screen.getByRole("button", { name: "重新解析草稿" })).toBeDisabled();
  fireEvent.click(button);
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "roadmap_observe",
        target_id: record.id,
      }),
    ),
  );
});

it("preserves old progress after a source change and offers an explicit new parse", async () => {
  const { submit } = setup("confirmed", true, [], (view, record) => {
    record.source = { ...record.source, digest: "old", body: "# 旧草稿" };
    view.documents_available = true;
  });
  const progress = await screen.findByRole("region", {
    name: "当前草稿转正进度",
  });
  await waitFor(() =>
    expect(within(progress).getByText(/草稿已修改/)).toBeVisible(),
  );
  expect(within(progress).getByRole("status")).toHaveTextContent(
    "任务与依赖已确认",
  );
  expect(
    screen.queryByRole("button", { name: "重试生成 Spec / Plan" }),
  ).not.toBeInTheDocument();
  expect(
    within(progress).getByRole("button", {
      name: "Runner generate · 解析任务与依赖",
    }),
  ).toBeEnabled();
  expect(submit).not.toHaveBeenCalled();
});

it("does not confuse separate flows or drop unresolved publication facts", () => {
  const { record } = setup();
  const oldParse = { ...record, id: "old", created_at: "2026-10-20T00:00:00Z" };
  const newParse = { ...record, id: "new", created_at: "2026-10-22T00:00:00Z" };
  const oldDoc = {
    ...record,
    id: "old-doc",
    purpose: "documents",
    parent_id: "old",
    state: "documents-review",
    created_at: "2026-10-23T00:00:00Z",
  };
  expect(currentRoadmapRecord([oldParse, oldDoc, newParse])?.id).toBe("new");
  oldDoc.publication = {
    url: "https://example.invalid/pr/1",
    state: "unknown",
  };
  expect(currentRoadmapRecord([oldParse, oldDoc, newParse])?.id).toBe(
    "old-doc",
  );
  expect(roadmapProgress(oldDoc).step).toBe(3);
});

it("switches the current flow and history scope with the selected draft", async () => {
  const { submit } = setup("confirmed", true, [], (view, record) => {
    view.sources.push({
      ...record.source,
      id: "another",
      path: "SteerlineFlow/Roadmap/drafts/another.md",
      body: "# 另一份草稿",
    });
  });
  fireEvent.click(
    await screen.findByRole("button", { name: /another.md.*尚未解析/ }),
  );
  expect(await screen.findByDisplayValue("# 另一份草稿")).toBeVisible();
  expect(screen.getByRole("status")).toHaveTextContent("尚未解析");
  expect(
    screen.getByRole("button", { name: "Runner generate · 解析任务与依赖" }),
  ).toBeEnabled();
  expect(
    screen.queryByRole("button", { name: "重试生成 Spec / Plan" }),
  ).not.toBeInTheDocument();
  expect(submit).not.toHaveBeenCalled();
});

it("tolerates checkout line endings when deciding whether confirmed inputs need reparsing", async () => {
  setup("confirmed", true, [], (view, record) => {
    record.source = {
      ...record.source,
      body: record.source.body.replaceAll("\n", "\r\n"),
      digest: "crlf-digest",
    };
    view.documents_available = true;
  });
  const button = await screen.findByRole("button", {
    name: "重试生成 Spec / Plan",
  });
  await waitFor(() => expect(button).toBeEnabled());
  expect(screen.queryByText(/草稿已修改/)).not.toBeInTheDocument();
});

it("places actual clarification text by the status and copies the selected historical Run", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  const { record, client } = setup("blocked", true, [], (view, record) => {
    record.request_id = record.id;
    record.execution_ref = {
      request_id: record.id,
      run_id: "old-run",
      attempt_id: "old-attempt",
    };
    record.analysis!.questions = ["确认调度模式"];
    view.records.push({
      ...record,
      id: "new-record",
      request_id: "new-record",
      created_at: "2026-11-09T00:00:00Z",
      state: "review",
      execution_ref: { request_id: "new-record", run_id: "new-run" },
      analysis: { ...record.analysis!, questions: [] },
    });
  });
  await screen.findByRole("button", {
    name: /确认任务依赖并生成 Spec \/ Plan/,
  });
  fireEvent.change(screen.getByLabelText("查看历史记录"), {
    target: { value: record.id },
  });
  const questions = await screen.findByRole("region", { name: "待澄清问题" });
  expect(within(questions).getByText("确认调度模式")).toBeVisible();
  expect(
    within(screen.getByRole("region", { name: "所选记录结果" })).getByRole(
      "region",
      { name: "待澄清问题" },
    ),
  ).toBeVisible();
  expect(screen.getAllByText("确认调度模式")).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "复制执行信息" }));
  await screen.findByText("已复制执行信息");
  expect(writeText.mock.calls[0][0]).toContain("old-run");
  expect(writeText.mock.calls[0][0]).toContain("old-attempt");
  expect(writeText.mock.calls[0][0]).not.toContain("new-run");
  client.clear();
});

it("copies the in-flight request before a journal row exists instead of a previous request", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  const pending = {
    project_id: identity.project_id,
    instance_id: identity.instance_id,
    request_id: "new-pending",
    action: "roadmap_parse",
    target_id: "b".repeat(32),
    expected_version: 1,
    payload: {},
  } as Envelope;
  const { client } = setup("review", true, [], undefined, false, () => [
    pending,
  ]);
  const progress = await screen.findByRole("region", {
    name: "当前草稿转正进度",
  });
  fireEvent.click(
    await within(progress).findByRole("button", { name: "复制执行信息" }),
  );
  await screen.findByText("已复制执行信息");
  expect(writeText.mock.calls[0][0]).toContain("请求 ID：new-pending");
  expect(writeText.mock.calls[0][0]).toContain("Runner 运行待核实");
  client.clear();
});

it("retries only the settled current task and blocks duplicate clicks while siblings run", async () => {
  const { submit, record } = setup(
    "accepted",
    true,
    [],
    (_view, r) => {
      r.purpose = "documents";
      r.analysis!.coordination = "T1 固定接口，T2 按依赖消费";
      r.generations = [
        {
          generation_id: "old",
          local_key: "T1",
          created_at: r.created_at,
          state: "failed",
          settled: true,
        },
        {
          generation_id: "current",
          local_key: "T1",
          created_at: r.created_at,
          state: "invalid",
          settled: true,
          diagnostic: "invalid_json: trailing",
        },
        {
          generation_id: "sibling",
          local_key: "T2",
          created_at: r.created_at,
          state: "accepted",
          settled: false,
          runner_status: "running",
        },
        {
          generation_id: "completed",
          local_key: "T4",
          created_at: r.created_at,
          state: "documents-review",
          settled: true,
          document: {
            local_key: "T4",
            spec: { path: "spec.md", body: "# 成功项 Spec" },
            plan: { path: "plan.md", body: "# 成功项 Plan" },
            tests: ["fixture.unit"],
            provides_tests: [],
            acceptance: [{ path: "acceptance.md", body: "# 成功项验收待执行" }],
          },
        },
        {
          generation_id: "closing",
          local_key: "T3",
          created_at: r.created_at,
          state: "failed",
          settled: false,
        },
      ];
    },
    true,
  );
  submit.mockImplementation(() => new Promise(() => {}) as never);
  const section = await screen.findByRole("region", { name: "逐任务文档生成" });
  expect(within(section).getAllByRole("heading", { level: 3 })).toHaveLength(4);
  expect(within(section).getByText("生成中")).toBeVisible();
  expect(screen.getByText(/协作安排：T1 固定接口/)).toBeVisible();
  fireEvent.click(within(section).getByText("预览已生成正文"));
  expect(within(section).getByText("# 成功项验收待执行")).toBeVisible();
  expect(within(section).getByText(/fixture.unit/)).toBeVisible();
  const retry = within(section).getByRole("button", { name: "重新生成 T1" });
  expect(
    within(section).getByRole("button", { name: "重新生成 T3" }),
  ).toBeDisabled();
  await waitFor(() => expect(retry).toBeEnabled());
  fireEvent.click(retry);
  fireEvent.click(retry);
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0]).toMatchObject({
    action: "roadmap_retry_documents",
    target_id: record.id,
    expected_version: 3,
    payload: { local_key: "T1" },
  });
  expect(retry).toBeDisabled();
});

it("continues observing a cancelled batch until every original execution settles", () => {
  const r = {
    state: "cancelled",
    cancelled: true,
    generations: [{ state: "failed", settled: false }],
  } as RoadmapRecord;
  expect(contentPending(r)).toBe(true);
  r.generations![0].settled = true;
  expect(contentPending(r)).toBe(false);
});

it("shows adopted review text throughout publication without redisplaying original generation bodies", async () => {
  setup("publishing", true, [], (_view, r) => {
    r.purpose = "documents";
    const document = {
      local_key: "T1",
      spec: { path: "spec.md", body: "# 审阅后正文" },
      plan: { path: "plan.md", body: "# Plan" },
      tests: [],
      provides_tests: [],
      acceptance: [{ path: "acceptance.md", body: "# 验收" }],
    };
    r.documents = [document];
    r.generations = [
      {
        generation_id: "generation",
        local_key: "T1",
        created_at: r.created_at,
        state: "documents-review",
        settled: true,
        document: {
          ...document,
          spec: { path: "spec.md", body: "# 原始生成正文" },
        },
      },
    ];
  });
  const section = await screen.findByRole("region", { name: "逐任务文档生成" });
  expect(within(section).queryByText("预览已生成正文")).toBeNull();
  expect(await screen.findByText("# 审阅后正文")).toBeVisible();
  expect(screen.queryByText("# 原始生成正文")).toBeNull();
});

it("distinguishes accepted native queue, blocked, external, finalizing and resume states", async () => {
  setup("accepted", true, [], (_view, r) => {
    r.purpose = "documents";
    r.generations = [
      "queued",
      "blocked",
      "waiting_input",
      "waiting_external",
      "completed",
      "running",
    ].map((runner_status, i) => ({
      generation_id: `g-${i}`,
      local_key: `T${i}`,
      created_at: r.created_at,
      state: "accepted",
      settled: false,
      runner_status,
      resume_required: i === 5,
    }));
  });
  const section = await screen.findByRole("region", { name: "逐任务文档生成" });
  for (const label of [
    "排队中",
    "生成受阻，需处理",
    "等待输入处理",
    "等待外部结果",
    "结果收尾中",
    "执行需要恢复核实",
  ])
    expect(within(section).getByText(label, { exact: true })).toBeVisible();
  expect(within(section).queryByText("生成中", { exact: true })).toBeNull();
});
