import { expect, test } from "@playwright/test";
import { createServer, type Server } from "node:http";
import { readFileSync, writeFileSync } from "node:fs";
import { extname, resolve } from "node:path";
import type {
  RoadmapRecord,
  RoadmapTask,
  RoadmapView,
} from "../src/features/roadmaps/roadmaps";
import { configSnapshot } from "../src/test/execution-config";

test.setTimeout(120000);
test.use({ actionTimeout: 10000 });
let server: Server;
let origin: string;
test.beforeAll(async () => {
  const root = resolve("server/dist");
  server = createServer((request, response) => {
    const path = new URL(request.url!, "http://localhost").pathname;
    const file = path.startsWith("/assets/")
      ? resolve(root, "." + path)
      : resolve(root, "index.html");
    if (!file.startsWith(root + "/") && !file.startsWith(root + "\\")) {
      response.writeHead(404).end();
      return;
    }
    try {
      response.setHeader(
        "Content-Type",
        { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" }[
          extname(file)
        ] ?? "application/octet-stream",
      );
      response.end(readFileSync(file));
    } catch {
      response.writeHead(404).end();
    }
  });
  await new Promise<void>((done) => server.listen(0, "127.0.0.1", done));
  const address = server.address();
  if (!address || typeof address === "string")
    throw new Error("missing owned test server");
  origin = `http://127.0.0.1:${address.port}`;
});
test.afterAll(async () => {
  await new Promise<void>((done, reject) =>
    server.close((error) => (error ? reject(error) : done())),
  );
});

test("per-task generation displays identities, retries only one task and stops the whole batch", async ({
  page,
  context,
}, info) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  const project = "a".repeat(64);
  const identity = {
    project_id: project,
    instance_id: "review-instance",
    repository_identity: "C:/fixture/.git",
    ready: true,
  };
  const makeTask = (local_key: string, local: string[] = []): RoadmapTask => ({
    local_key,
    title: `${local_key} 任务`,
    key_origin: "markdown",
    goal: "交付当前能力",
    scope: "本次范围",
    constraints: "仅按所列直接依赖",
    acceptance: "结果可复核",
    evidence: `草稿原文 ${local_key}`,
    reason: "",
    existing_task_id: "",
    original_dependencies: { local, formal: [] },
    proposed_dependencies: { local, formal: [] },
  });
  const tasks = [
    makeTask("tools"),
    makeTask("seed", ["tools"]),
    makeTask("left", ["seed"]),
    makeTask("right", ["seed"]),
    makeTask("join", ["left", "right"]),
    makeTask("independent", ["tools"]),
  ];
  tasks[0].original_dependencies.formal = ["101"];
  tasks[0].proposed_dependencies.formal = ["101"];
  tasks[3].proposed_dependencies.local = ["tools"];
  tasks[3].original_dependencies.formal = ["102"];
  tasks[3].reason = "分支 B 直接消费工具交付，移除起点样本和旧正式任务前置";
  tasks[5].title = "长标题验证".repeat(16);
  const source = {
    id: "b".repeat(32),
    path: "SteerlineFlow/Roadmap/drafts/review.md",
    body: "# 原始草稿",
    digest: "c".repeat(64),
    head: "d".repeat(40),
    modified: false,
  };
  const record: RoadmapRecord = {
    id: "e".repeat(32),
    source,
    revision: 3,
    created_at: "2026-11-09T00:00:00Z",
    state: "review",
    documents: [],
    formal_mapping: {},
    cancelled: false,
    existing: [
      { id: "101", title: "已有工具基础" },
      { id: "102", title: "仅原始引用的前置" },
      { id: "105", title: "未引用任务" },
    ],
    analysis: { tasks, questions: [], overlap: "无重复；固定解析来源供复核" },
  };
  const view: RoadmapView = {
    preparation: {
      commit: source.head,
      ready: true,
      local_ready: true,
      branch: "main",
      files: [],
    },
    sources: [source],
    records: [record],
    profiles: [{ id: "local", name: "本机" }],
    documents_available: true,
    documents_reason: "",
  };
  const historical: RoadmapRecord = {
    ...structuredClone(record),
    id: "f".repeat(32),
    created_at: "2026-11-08T00:00:00Z",
    state: "confirmed",
    analysis: {
      tasks: [makeTask("historical")],
      questions: [],
      overlap: "历史范围",
    },
  };
  view.records.push(historical);
  record.purpose = "documents";
  record.state = "accepted";
  record.analysis!.coordination =
    "tools 固定工具接口，其它任务按确认依赖消费，不修改工具所有权";
  tasks[0].provides_tests = ["demo.tools"];
  record.generations = tasks.map((task, i) => ({
    generation_id: "generation-" + task.local_key,
    local_key: task.local_key,
    created_at: record.created_at,
    state: [
      "documents-review",
      "accepted",
      "invalid",
      "accepted",
      "unknown",
      "queued",
    ][i],
    settled: i === 0 || i === 2,
    runner_status: i === 1 ? "running" : i === 3 ? "completed" : undefined,
    run_id: i === 5 ? undefined : "run-" + task.local_key,
    attempt_id: "attempt-" + task.local_key,
    session_id: "session-" + task.local_key,
    diagnostic:
      i === 2
        ? "invalid_json: invalid character after top-level value"
        : undefined,
  }));
  record.generations![0].document = {
    local_key: "tools",
    spec: { path: "spec.md", body: "# tools 完整 Spec" },
    plan: { path: "plan.md", body: "# tools 完整 Plan" },
    tests: ["fixture.unit"],
    provides_tests: ["demo.tools"],
    acceptance: [
      {
        path: "SteerlineFlow/testing/acceptance/roadmaps/tools.md",
        body: "# tools 验收待执行",
      },
    ],
  };
  historical.purpose = "documents";
  historical.state = "failed";
  historical.generations = [
    {
      generation_id: "history",
      local_key: "historical",
      created_at: historical.created_at,
      state: "failed",
      settled: true,
    },
  ];
  const errors: string[] = [];
  const writes: unknown[] = [];
  const reads: string[] = [];
  page.on("pageerror", (error) => errors.push(String(error)));
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  await page.route("**/auth/status", (route) =>
    route.fulfill({
      json: {
        csrf_token: "csrf",
        identity: {
          home_id: "review-owned-home",
          instance_id: "web",
          ready: true,
        },
      },
    }),
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    let data: unknown = {};
    if (route.request().method() !== "GET") {
      const envelope = route.request().postDataJSON();
      writes.push(envelope);
      expect(envelope.target_id).toBe(record.id);
      expect(envelope.expected_version).toBe(
        envelope.action === "roadmap_stop" ? 1 : record.revision,
      );
      expect(["roadmap_retry_documents", "roadmap_stop"]).toContain(
        envelope.action,
      );
      if (envelope.action === "roadmap_retry_documents") {
        expect(envelope.payload).toEqual({ local_key: "left" });
        record.generations!.push({
          generation_id: "retry-left",
          local_key: "left",
          created_at: new Date().toISOString(),
          state: "queued",
          settled: false,
        });
      } else {
        expect(envelope.payload).toEqual({});
        record.cancelled = true;
        record.state = "cancelled";
      }
      record.revision++;
      const document = record;
      data = {
        roadmap: document,
        ...envelope,
        original: envelope,
        status: "applied",
        sampled_at: new Date().toISOString(),
      };
    } else {
      reads.push(path);
      if (path.endsWith("/projects"))
        data = {
          projects: [
            {
              project_id: project,
              config_id: "review",
              name: "审阅样本",
              repository: "C:/fixture",
              online: true,
            },
          ],
          loaded_execution_digest: "same",
        };
      else if (path.endsWith("/identity")) data = identity;
      else if (path.endsWith("/snapshot"))
        data = {
          connection: {
            state: "connected",
            can_control: true,
            identity_matched: true,
            config_matched: true,
            snapshot: {
              page: {
                service: {
                  instance_id: identity.instance_id,
                  capabilities: [],
                },
                runs: [],
                control: {
                  revision: 1,
                  dispatch_mode: "manual",
                  blockers: [],
                },
              },
            },
          },
        };
      else if (path.endsWith("/status")) data = { saved_digest: "same" };
      else if (path.endsWith("/execution-config")) data = configSnapshot;
      else if (path.endsWith("/maintenance/readiness"))
        data = {
          status: "ready",
          blockers: [],
          commit: source.head,
          total: 0,
          ready: 0,
        };
      else if (path.endsWith("/maintenance"))
        data = {
          readiness: {
            status: "ready",
            blockers: [],
            commit: source.head,
            total: 0,
            ready: 0,
          },
          observation: { commit: "" },
          remote_name: "main",
          remote: {
            sha: "",
            title: "",
            author: "",
            time: "",
            url: "",
            prs: [],
          },
          local: {
            commit: source.head,
            branch: "main",
            status: "synced",
            ahead: 0,
            behind: 0,
            can_update: false,
          },
          tasks: [],
        };
      else if (path.endsWith("/activities"))
        data = {
          project_id: project,
          instance_id: identity.instance_id,
          items: [],
          sources: [],
          request_links: {},
          counts: { ongoing: 0, attention: 0, recent: 0 },
          counts_known: true,
          has_more: false,
        };
      else if (path.endsWith("/presentation"))
        data = { presentation: { profiles: [] } };
      else if (path.endsWith("/tasks")) data = { tasks: [], has_more: false };
      else if (path.endsWith("/environment/options"))
        data = { environments: [], profiles: [], generators: [] };
      else if (path.endsWith("/roadmaps")) data = view;
      else if (path.endsWith("/roadmaps/sources/" + source.id)) data = source;
      else if (path.endsWith("/roadmaps/" + record.id)) data = record;
      else if (path.endsWith("/roadmaps/" + historical.id)) data = historical;
      else if (path.includes("/roadmaps/"))
        data = view.records.find((r) => path.endsWith("/" + r.id));
    }
    await route.fulfill({ json: data });
  });
  await page.goto(`${origin}/projects/${project}/roadmaps`);
  const section = page.getByRole("region", { name: "逐任务文档生成" });
  await expect(section.locator(".roadmaps-generation")).toHaveCount(6);
  await expect(page.getByText(/协作安排：tools 固定工具接口/)).toBeVisible();
  for (const label of [
    "生成完成",
    "生成中",
    "结果校验失败",
    "结果收尾中",
    "提交待核实",
    "排队中",
  ])
    await expect(section.getByText(label, { exact: true })).toBeVisible();
  await page
    .getByRole("button", { name: "查看 tools · tools 任务", exact: true })
    .click();
  await expect(page.getByText("demo.tools", { exact: true })).toBeVisible();
  const copy = section
    .locator(".roadmaps-generation")
    .filter({ has: page.getByRole("heading", { name: "left", exact: true }) });
  await copy.getByRole("button", { name: "复制执行信息" }).click();
  await expect(copy.getByText("已复制执行信息")).toBeVisible();
  const copied = await page.evaluate(() => navigator.clipboard.readText());
  expect(copied).toContain("Generation ID：generation-left");
  expect(copied).toContain("Runner Run ID：run-left");
  expect(copied).toContain("Session ID：session-left");
  await section.getByText("预览已生成正文").click();
  await expect(section.getByText("# tools 验收待执行")).toBeVisible();
  for (const width of [1440, 768, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    await section.scrollIntoViewIfNeeded();
    await page.screenshot({
      path: info.outputPath(`generation-${width}.png`),
      fullPage: true,
    });
    expect(
      await section.evaluate((el) => el.scrollWidth <= el.clientWidth),
    ).toBe(true);
  }
  const running = record.generations![1];
  for (const [status, resume, label] of [
    ["queued", false, "排队中"],
    ["blocked", false, "生成受阻，需处理"],
    ["waiting_input", false, "等待输入处理"],
    ["waiting_external", false, "等待外部结果"],
    ["completed", false, "结果收尾中"],
    ["running", true, "执行需要恢复核实"],
  ] as const) {
    running.runner_status = status;
    running.resume_required = resume;
    record.revision++;
    await page.getByRole("button", { name: "刷新 Markdown 与观察" }).click();
    await expect(
      section
        .locator(".roadmaps-generation")
        .filter({
          has: page.getByRole("heading", { name: "seed", exact: true }),
        })
        .getByText(label, { exact: true }),
    ).toBeVisible();
  }
  await page.screenshot({
    path: info.outputPath("native-statuses.png"),
    fullPage: true,
  });
  running.runner_status = "running";
  running.resume_required = false;
  record.revision++;
  await page.getByRole("button", { name: "刷新 Markdown 与观察" }).click();
  expect(writes).toHaveLength(0);
  await page.getByText("历史记录（2）", { exact: true }).click();
  await page.getByLabel("查看历史记录").selectOption(historical.id);
  await expect(
    section.getByRole("button", { name: "重新生成 historical" }),
  ).toBeDisabled();
  await page.getByRole("button", { name: "返回当前流程" }).click();
  const retry = section.getByRole("button", { name: "重新生成 left" });
  await retry.dblclick();
  await expect(section.getByText("排队中", { exact: true })).toHaveCount(2);
  expect(writes).toHaveLength(1);
  await page.getByRole("button", { name: "停止当前内容请求" }).click();
  await expect(section.getByText("停止收尾中", { exact: true })).toHaveCount(5);
  expect(writes).toHaveLength(2);
  expect(errors).toEqual([]);
  writeFileSync(
    info.outputPath("proof.json"),
    JSON.stringify(
      { writes, reads, errors, viewports: [1440, 768, 320] },
      null,
      2,
    ),
  );
});
