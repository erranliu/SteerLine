import { expect, test } from "@playwright/test";
import { createServer, type Server } from "node:http";
import { readFileSync } from "node:fs";
import { extname, resolve } from "node:path";

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

test("copies exact current, historical and local-operation IDs with visible questions", async ({
  page,
}, info) => {
  const project = "a".repeat(64),
    recordID = "b".repeat(32),
    oldID = "c".repeat(32),
    requestID = "d".repeat(32);
  const identity = {
    project_id: project,
    instance_id: "instance",
    repository_identity: "C:/Projects/SteerlineTest/.git",
    ready: true,
  };
  const source = {
    id: "e".repeat(32),
    path: "SteerlineFlow/Roadmap/drafts/sample.md",
    body: "# 演示草稿",
    digest: "f".repeat(64),
    head: "1".repeat(40),
    modified: false,
  };
  const record = {
    id: recordID,
    request_id: recordID,
    source,
    revision: 1,
    created_at: "2026-11-09T00:00:00Z",
    state: "blocked",
    documents: [],
    formal_mapping: {},
    cancelled: false,
    analysis: {
      tasks: [],
      questions: ["请确认演示使用手动调度", "请准备 Runner 登录与仓库权限"],
      overlap: "",
    },
    execution_ref: {
      request_id: recordID,
      run_id: "current-run",
      attempt_id: "current-attempt",
    },
  };
  const old = {
    ...record,
    id: oldID,
    request_id: oldID,
    created_at: "2026-11-08T00:00:00Z",
    analysis: { ...record.analysis, questions: ["历史问题"] },
    execution_ref: {
      request_id: oldID,
      run_id: "old-run",
      attempt_id: "old-attempt",
    },
  };
  const view = {
    sources: [source],
    records: [record, old],
    profiles: [{ id: "local", name: "本机" }],
    documents_available: true,
    documents_reason: "",
    preparation: {
      ready: true,
      local_ready: true,
      branch: "main",
      commit: source.head,
      files: [],
    },
  };
  const activity = {
    activity_id: "publication",
    project_id: project,
    source: "roadmap",
    source_id: recordID,
    kind: "roadmap_parse",
    request_ids: [requestID],
    title: "Roadmap 文档发布",
    state: "waiting",
    phase: "发布待核实",
    observation: "current",
    needs_attention: false,
    execution_ref: { request_id: requestID, action: "roadmap_publish" },
    details_ref: { kind: "roadmap", target: recordID },
  };
  const receipt = {
    project_id: project,
    instance_id: "old-instance",
    request_id: requestID,
    target_id: recordID,
    action: "roadmap_publish",
    status: "rejected",
    diagnostic: "发布校验未通过",
    roadmap: record,
  };
  const errors: string[] = [],
    writes: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  await page.addInitScript(() =>
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async (text: string) => {
          (window as any).copiedExecution = text;
        },
      },
    }),
  );
  await page.route("**/auth/status", (r) =>
    r.fulfill({
      json: {
        csrf_token: "csrf",
        identity: {
          home_id: "owned-execution-info-home",
          instance_id: "web",
          ready: true,
        },
      },
    }),
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (route.request().method() !== "GET") writes.push(path);
    let data: unknown = {};
    if (path.endsWith("/projects"))
      data = {
        projects: [
          {
            project_id: project,
            config_id: "test",
            name: "SteerlineTest",
            repository: "C:/Projects/SteerlineTest",
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
              service: { instance_id: "instance", capabilities: [] },
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
    else if (path.endsWith("/activities"))
      data = {
        project_id: project,
        instance_id: "instance",
        sampled_at: new Date().toISOString(),
        items: [activity],
        request_links: {},
        sources: [],
        counts: { ongoing: 1, attention: 0, recent: 0 },
        counts_known: true,
        has_more: false,
      };
    else if (path.endsWith("/roadmaps")) data = view;
    else if (path.endsWith("/roadmaps/sources/" + source.id)) data = source;
    else if (path.endsWith("/roadmaps/" + recordID)) data = record;
    else if (path.endsWith("/roadmaps/" + oldID)) data = old;
    else if (path.endsWith("/commands/" + requestID)) data = receipt;
    else if (path.endsWith("/presentation"))
      data = { presentation: { profiles: [] } };
    else if (path.endsWith("/tasks")) data = { tasks: [], has_more: false };
    else if (path.endsWith("/environment/options"))
      data = { environments: [], profiles: [] };
    else if (path.endsWith("/maintenance/readiness"))
      data = { status: "ready", blockers: [], total: 0, ready: 0 };
    else if (path.endsWith("/maintenance"))
      data = {
        readiness: { status: "ready", blockers: [], total: 0, ready: 0 },
        observation: { commit: "" },
        remote_name: "main",
        remote: { sha: "", title: "", author: "", time: "", url: "", prs: [] },
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
    else if (path.endsWith("/status")) data = { saved_digest: "same" };
    await route.fulfill({ json: data });
  });
  await page.goto(`${origin}/projects/${project}/roadmaps`);
  const selected = page.getByRole("region", { name: "所选记录结果" });
  await expect(selected.getByText("请确认演示使用手动调度")).toBeVisible();
  await selected.getByRole("button", { name: "复制执行信息" }).click();
  await expect
    .poll(() => page.evaluate(() => (window as any).copiedExecution))
    .toContain("Runner Run ID：current-run");
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    await selected.scrollIntoViewIfNeeded();
    await expect(
      selected.getByRole("button", { name: "复制执行信息" }),
    ).toBeVisible();
    await page.screenshot({
      path: info.outputPath(`execution-info-${width}.png`),
      fullPage: true,
    });
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.getByText("历史记录（2）", { exact: true }).click();
  await page.getByLabel("查看历史记录").selectOption(oldID);
  await expect(selected.getByText("历史问题", { exact: true })).toBeVisible();
  await selected.getByRole("button", { name: "复制执行信息" }).click();
  const historical = await page.evaluate(() => (window as any).copiedExecution);
  expect(historical).toContain("old-run");
  expect(historical).not.toContain("current-run");
  await page.getByRole("button", { name: /^活动 ·/ }).click();
  const drawer = page.getByRole("region", { name: /活动中心/ });
  await drawer.getByRole("button", { name: "复制执行信息" }).click();
  const publication = await page.evaluate(
    () => (window as any).copiedExecution,
  );
  expect(publication).toContain(requestID);
  expect(publication).not.toContain("Runner");
  await drawer.getByRole("button", { name: "查看详情" }).click();
  await expect(page).toHaveURL(
    new RegExp(`record=${recordID}&request_id=${requestID}`),
  );
  const operation = page.getByRole("region", { name: "指定操作结果" });
  await expect(operation.getByText("发布校验未通过")).toBeVisible();
  await operation.getByRole("button", { name: "复制执行信息" }).click();
  expect(await page.evaluate(() => (window as any).copiedExecution)).toBe(
    publication,
  );
  expect(writes).toEqual([]);
  expect(errors).toEqual([]);
});
