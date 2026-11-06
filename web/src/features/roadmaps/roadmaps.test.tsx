import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, testClient } from "../../test/workspace";
import type {StoredRequest} from "../../shared/api/types";
import { Roadmaps, type RoadmapRecord, type RoadmapView } from "./roadmaps";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
function setup(state = "review", online = true, requests:StoredRequest[] = []) {
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
    preparation: {commit: source.head, ready: true, local_ready: true, branch: "main", files: []},
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
          ? source
          : (record as never),
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
  mountFeature(<Roadmaps />, api, testClient(), online,{requests});
  return { api, get, submit, source, record, view };
}
it("reads Markdown and shows original versus proposed dependencies without allocating or calling a model on mount", async () => {
  const { submit } = setup();
  expect(await screen.findByDisplayValue(/T1 候选控制/)).toHaveAttribute(
    "readonly",
  );
  expect(await screen.findByRole("link", { name: "正式 #8 · 固定调度" })).toHaveAttribute(
    "href", "/projects/" + identity.project_id + "/dag?task=8",
  );
  expect(screen.getByText("建议补充固定调度前置")).toBeVisible();
  expect(
    screen.getByRole("button", { name: "生成 Spec / Plan" }),
  ).toBeDisabled();
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
it("shows mixed original references separately from proposed formal dependencies", async () => {
  const { record } = setup();
  const task = record.analysis!.tasks[0];
  task.original_dependencies = { local: ["T0"], formal: ["8"] };
  task.proposed_dependencies = { local: [], formal: ["8"] };
  task.reason = "T0 已关联正式 #8，合并重复依赖";
  await screen.findByText(task.reason);
  const row = screen.getByRole("row", { name: /候选控制.*合并重复依赖/ });
  const cells = within(row).getAllByRole("cell");
  expect(within(cells[1]).getByText("本 Roadmap T0")).toBeVisible();
  expect(within(cells[1]).getByRole("link", { name: "正式 #8 · 固定调度" })).toHaveAttribute(
    "href", "/projects/" + identity.project_id + "/dag?task=8",
  );
  expect(within(cells[2]).queryByText("本 Roadmap T0")).toBeNull();
  expect(within(cells[2]).getByRole("link", { name: "正式 #8 · 固定调度" })).toBeVisible();
});
it("submits an explicit Runner parse with original request identity and no formal IDs", async () => {
  const { submit, source } = setup();
  await screen.findByDisplayValue(/T1 候选控制/);
  fireEvent.click(
    screen.getByRole("button", { name: "Runner generate · 解析任务与依赖" }),
  );
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
    await screen.findByRole("button", { name: "正式入 DAG · 分配编号" }),
  ).toBeDisabled();
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
  expect(
    screen.getByRole("button", { name: "正式入 DAG · 分配编号" }),
  ).toBeDisabled();
});

it("shows fixed preparation failures and submits explicit initialization without parsing", async () => {
 const {view,submit}=setup();
 view.preparation={commit:"d".repeat(40),ready:false,local_ready:false,branch:"main",files:[{path:"SteerlineFlow/Roadmap/parse-rules.md",ready:false}]};
 expect(await screen.findByRole("heading",{name:"先准备项目静态材料"})).toBeVisible();
 expect(screen.getByRole("button",{name:"Runner generate · 解析任务与依赖"})).toBeDisabled();
 fireEvent.click(screen.getByRole("button",{name:"初始化静态材料"}));
 await waitFor(()=>expect(submit).toHaveBeenCalledWith(expect.objectContaining({action:"roadmap_prepare_static",target_id:identity.project_id,expected_version:1,payload:{commit:"d".repeat(40)}})));
 expect(submit.mock.calls.some(([e])=>e.action==="roadmap_parse")).toBe(false);
});
it("keeps local-only preparation blocked until published and prevents needless regeneration",async()=>{
 const {view,submit}=setup();
 view.preparation={commit:"d".repeat(40),ready:false,local_ready:true,branch:"main",files:[]};
 expect(await screen.findByText(/材料已在工作树中，请提交并合入 main/)).toBeVisible();
 expect(screen.getByRole("button",{name:"初始化静态材料"})).toBeDisabled();
 expect(screen.getByRole("button",{name:"Runner generate · 解析任务与依赖"})).toBeDisabled();
 expect(submit).not.toHaveBeenCalled();
});

it.each([["other",false],[identity.project_id,true]])("scopes pending initialization requests to project %s",async(owner,blocked)=>{
 const request:StoredRequest={envelope:new Api({...identity,project_id:owner},"csrf").envelope("roadmap_prepare_static",owner,1,{}),created_at:new Date().toISOString()};
 const {view}=setup("review",true,[request]);
 view.preparation={commit:"d".repeat(40),ready:false,local_ready:false,branch:"main",files:[]};
 const button=await screen.findByRole("button",{name:"初始化静态材料"});
 if(blocked)expect(button).toBeDisabled();else expect(button).toBeEnabled();
});
