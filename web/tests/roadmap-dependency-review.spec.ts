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

test("saved roadmap preview preserves topology, comparison, task details and confirmation scope", async ({
  page,
}, info) => {
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
  tasks[3].reason = "分支 B 直接消费工具交付，移除起点样本前置";
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
      expect(envelope).toMatchObject({
        action: "roadmap_confirm",
        target_id: record.id,
        expected_version: 3,
        payload: {},
      });
      record.state = "confirmed";
      data = {
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
                  dispatch_paused: true,
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
    }
    await route.fulfill({ json: data });
  });
  await page.goto(`${origin}/projects/${project}/roadmaps`);
  const review = page.locator("#roadmap-dependencies");
  await expect(review.locator(".react-flow__node")).toHaveCount(7);
  await expect(review.locator(".react-flow__edge")).toHaveCount(7);
  await expect(
    review.locator(".react-flow__edge-path").first(),
  ).toHaveAttribute("marker-end", /.+/);
  await expect(review.getByText(/未引用任务/)).toHaveCount(0);
  await expect(review.locator(".roadmaps-review-summary")).toContainText(
    "6 项规划",
  );
  const positions = await review
    .locator(".react-flow__node")
    .evaluateAll((nodes) =>
      nodes.map((node) => ({
        id: node.getAttribute("data-id"),
        transform: (node as HTMLElement).style.transform,
      })),
    );
  await review.getByRole("button", { name: "差异", exact: true }).click();
  await expect(review.locator(".react-flow__edge")).toHaveCount(8);
  await expect(review.locator(".react-flow__edge.added")).toHaveCount(1);
  await expect(review.locator(".react-flow__edge.removed")).toHaveCount(1);
  await review.getByRole("button", { name: "查看变更 · right" }).click();
  const detail = review.getByRole("region", { name: "所选任务详情" });
  await expect(detail.getByText("草稿原文 right")).toBeVisible();
  const viewport = review.locator(".react-flow__viewport");
  const beforeZoom = await viewport.getAttribute("style");
  await review.locator(".react-flow__controls-zoomin").click();
  await expect.poll(() => viewport.getAttribute("style")).not.toBe(beforeZoom);
  await review.locator(".react-flow__controls-fitview").click();
  const canvas = review.locator(".roadmaps-review-canvas");
  await expect
    .poll(() =>
      review.locator(".react-flow__node").evaluateAll((nodes) => {
        const bounds = document
          .querySelector(".roadmaps-review-canvas")!
          .getBoundingClientRect();
        return nodes.every((node) => {
          const rect = node.getBoundingClientRect();
          return (
            rect.left >= bounds.left &&
            rect.right <= bounds.right &&
            rect.top >= bounds.top &&
            rect.bottom <= bounds.bottom
          );
        });
      }),
    )
    .toBe(true);
  await canvas.scrollIntoViewIfNeeded();
  expect(
    await canvas.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      const progress = document
        .querySelector(".roadmaps-progress")!
        .getBoundingClientRect();
      return progress.bottom <= bounds.top || progress.top >= bounds.bottom;
    }),
  ).toBe(true);
  await canvas.screenshot({ path: info.outputPath("dependency-graph.png") });
  await expect(detail.getByText("seed → right · 移除")).toBeVisible();
  await review.locator('.react-flow__node[data-id="local:left"]').click();
  await expect(detail.getByText("草稿原文 left")).toBeVisible();
  await review.locator(".react-flow__edge.removed").dispatchEvent("click");
  await expect(detail.getByText("草稿原文 right")).toBeVisible();
  await expect(
    review.locator(".react-flow__node.review-dimmed"),
  ).not.toHaveCount(0);
  await review.getByRole("button", { name: "草稿原始", exact: true }).click();
  await expect(review.locator(".react-flow__edge")).toHaveCount(7);
  expect(
    await review.locator(".react-flow__node").evaluateAll((nodes) =>
      nodes.map((node) => ({
        id: node.getAttribute("data-id"),
        transform: (node as HTMLElement).style.transform,
      })),
    ),
  ).toEqual(positions);
  await page.getByRole("button", { name: "刷新 Markdown 与观察" }).click();
  await expect(
    review.getByRole("button", { name: "草稿原始", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(detail.getByText("草稿原文 right")).toBeVisible();
  for (const width of [1440, 768, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    const taskButton = review.getByRole("button", {
      name: `查看 independent · ${tasks[5].title}`,
      exact: true,
    });
    await taskButton.focus();
    await page.keyboard.press("Enter");
    await expect(detail.getByText("草稿原文 independent")).toBeVisible();
    await page.screenshot({
      path: info.outputPath(`roadmap-review-${width}.png`),
      fullPage: true,
    });
    const layout = await review.evaluate((element) => {
      const area = element as HTMLElement;
      const documentWidth = document.documentElement.scrollWidth;
      const bounds = area.getBoundingClientRect();
      const previous = area.style.display;
      area.style.display = "none";
      const baselineWidth = document.documentElement.scrollWidth;
      area.style.display = previous;
      return {
        width: innerWidth,
        documentWidth,
        baselineWidth,
        left: bounds.left,
        right: bounds.right,
        contentWidth: area.scrollWidth,
        clientWidth: area.clientWidth,
      };
    });
    writeFileSync(
      info.outputPath(`layout-${width}.json`),
      JSON.stringify(layout, null, 2),
    );
    expect(layout.documentWidth).toBeLessThanOrEqual(
      Math.max(width, layout.baselineWidth),
    );
    expect(layout.left).toBeGreaterThanOrEqual(0);
    expect(layout.right).toBeLessThanOrEqual(width);
    expect(layout.contentWidth).toBeLessThanOrEqual(layout.clientWidth);
  }
  expect(writes).toEqual([]);
  expect(reads.some((path) => path.endsWith("/graph"))).toBe(false);
  await page.getByText("历史记录（2）", { exact: true }).click();
  await page.getByLabel("查看历史记录").selectOption(historical.id);
  await expect(review.locator(".react-flow__node")).toHaveCount(1);
  await expect(
    review.getByRole("button", { name: "查看 historical · historical 任务" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /确认本次任务与依赖/ }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "返回当前流程", exact: true }).click();
  await expect(review.locator(".react-flow__node")).toHaveCount(7);
  await expect(
    review.getByRole("button", { name: "Runner 建议", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(
    detail.getByText("选择图中节点或下方任务，查看范围、依赖与原文依据。"),
  ).toBeVisible();
  await review.getByRole("button", { name: "草稿原始", exact: true }).click();
  await page.getByRole("button", { name: /确认本次任务与依赖/ }).click();
  await expect.poll(() => writes.length).toBe(1);
  await expect(
    page.getByRole("status").filter({ hasText: "任务与依赖已确认" }),
  ).toBeVisible();
  expect(errors).toEqual([]);
  writeFileSync(
    info.outputPath("browser-facts.json"),
    JSON.stringify({ writes, reads, errors, positions }, null, 2),
  );
});
