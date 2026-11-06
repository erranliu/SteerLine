import { afterEach, expect, it, vi } from "vitest";
import {
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
import { currentRoadmapRecord, roadmapProgress } from "./workflow";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, testClient } from "../../test/workspace";
import type { StoredRequest } from "../../shared/api/types";
import { Roadmaps, type RoadmapRecord, type RoadmapView } from "./roadmaps";

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
    documents_available: false,
    documents_reason:
      "待 Runner 文档能力接入：固定仓库只读调查、整批设计、多文档输出",
  };
  const api = new Api(identity, "csrf");
  const get = vi
    .spyOn(api, "get")
    .mockImplementation(async (route) =>
      route === "/roadmaps"
        ? view
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
  mountFeature(
    <>
      <Roadmaps />
      <LocationProbe />
    </>,
    api,
    testClient(),
    online,
    { requests },
  );
  return { api, get, submit, source, record, view };
}
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
          screen.getByRole("button", { name: "确认任务与依赖" }),
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
    if (envelope.action === "roadmap_confirm") {
      record.state = "confirmed";
      record.confirmed_digest = "confirmed";
    } else if (envelope.action === "roadmap_generate_documents") {
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
  const confirm = await screen.findByRole("button", { name: "确认任务与依赖" });
  await waitFor(() => expect(confirm).toBeEnabled());
  fireEvent.click(confirm);
  const generate = await screen.findByRole("button", {
    name: "生成 Spec / Plan",
  });
  await waitFor(() => expect(generate).toBeEnabled());
  expect(screen.getByTestId("location")).not.toHaveTextContent("record=");
  expect(screen.getByTestId("location")).toHaveTextContent(
    "source=" + record.source.id,
  );
  fireEvent.click(generate);
  await waitFor(() =>
    expect(screen.getByRole("status")).toHaveTextContent("生成文档中"),
  );
  expect(screen.queryByRole("button", { name: "返回当前流程" })).toBeNull();
  expect(submit).toHaveBeenCalledTimes(2);
  client.clear();
});
it("reads Markdown and shows original versus proposed dependencies without allocating or calling a model on mount", async () => {
  const { submit } = setup();
  expect(await screen.findByDisplayValue(/T1 候选控制/)).toHaveAttribute(
    "readonly",
  );
  expect(await screen.findByRole("cell", { name: "正式 #8" })).toBeVisible();
  expect(screen.getByText("建议补充固定调度前置")).toBeVisible();
  expect(
    screen.queryByRole("button", { name: "生成 Spec / Plan" }),
  ).not.toBeInTheDocument();
  expect(screen.getByText("入 DAG 时分配")).toBeVisible();
  expect(submit).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "确认任务与依赖" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "roadmap_confirm",
        target_id: "a".repeat(32),
        expected_version: 3,
        payload: {},
      }),
    ),
  );
  expect(screen.queryByText("#91")).toBeNull();
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
  const { submit } = setup("confirmed");
  expect(await screen.findByText(/待 Runner 文档能力接入/)).toBeVisible();
  expect(
    screen.getByRole("button", { name: "生成 Spec / Plan" }),
  ).toBeDisabled();
  expect(
    screen.queryByRole("button", { name: "正式入 DAG · 分配编号" }),
  ).not.toBeInTheDocument();
  fireEvent.click(
    screen.getByRole("button", { name: "SteerLine 生成的 JSON" }),
  );
  expect(screen.getByText(/"formal_mapping": \{\}/)).toBeVisible();
  expect(submit).not.toHaveBeenCalled();
});
it("generates documents only after confirmation when the selected Runner publishes repository support", async () => {
  const { view, submit } = setup("confirmed");
  view.documents_available = true;
  view.documents_reason = "";
  const button = await screen.findByRole("button", {
    name: "生成 Spec / Plan",
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
      spec: { path: "spec/control.md", body: "# 完整 Spec" },
      plan: { path: "plans/control.md", body: "# 完整 Plan" },
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
  expect(await screen.findByText(/logic.go/)).toBeVisible();
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
  expect(await screen.findByText(/固定仓库.*仓库中的历史文件/)).toBeVisible();
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
  const { view, submit } = setup();
  view.preparation = {
    commit: "d".repeat(40),
    ready: false,
    local_ready: false,
    branch: "main",
    files: [{ path: "SteerlineFlow/Roadmap/parse-rules.md", ready: false }],
  };
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
  const { view, submit } = setup();
  view.preparation = {
    commit: "d".repeat(40),
    ready: false,
    local_ready: true,
    branch: "main",
    files: [],
  };
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
    const { view } = setup("review", true, [request]);
    view.preparation = {
      commit: "d".repeat(40),
      ready: false,
      local_ready: false,
      branch: "main",
      files: [],
    };
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
    screen.queryByRole("button", { name: "生成 Spec / Plan" }),
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
    name: "生成 Spec / Plan",
  });
  await waitFor(() => expect(button).toBeEnabled());
  expect(
    screen
      .getByRole("list", { name: "转正阶段" })
      .querySelectorAll(".complete"),
  ).toHaveLength(2);
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
    screen.queryByRole("button", { name: "生成 Spec / Plan" }),
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
  expect(roadmapProgress(oldDoc).step).toBe(4);
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
    screen.queryByRole("button", { name: "生成 Spec / Plan" }),
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
    name: "生成 Spec / Plan",
  });
  await waitFor(() => expect(button).toBeEnabled());
  expect(screen.queryByText(/草稿已修改/)).not.toBeInTheDocument();
});
