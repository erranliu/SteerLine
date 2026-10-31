import { test, expect } from "@playwright/test";
import { createServer, type Server } from "node:http";
import { readFileSync } from "node:fs";
import { resolve, extname } from "node:path";
import { configSnapshot } from "../src/test/execution-config";

test.setTimeout(120000);
test.use({ actionTimeout: 10000 });

// UI-only browser verification: serve the built client and intercept all APIs.
let server: Server;
let origin: string;
test.beforeAll(async () => {
  const root = resolve("server/dist");
  server = createServer((request, response) => {
    const pathname = new URL(request.url!, "http://localhost").pathname;
    const file = pathname.startsWith("/assets/")
      ? resolve(root, "." + pathname)
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
    throw new Error("missing local server address");
  origin = `http://127.0.0.1:${address.port}`;
});
test.afterAll(async () => {
  await new Promise<void>((done, reject) =>
    server.close((error) => (error ? reject(error) : done())),
  );
});

test("runtime environments keep card actions, failed observations and task return scoped", async ({
  page,
}, info) => {
  const projects = ["a".repeat(64), "b".repeat(64)];
  let failOptions = false;
  const writes: unknown[] = [];
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(String(e)));
  const profiles = ["default", "alternate"].map((id) => ({
    id,
    name: id,
    host: "本机",
    agent: "codex",
    details: [],
  }));
  const stages = [
    "ready",
    "awaiting_user",
    "verifying",
    "blocked",
    "stale",
    "unprepared",
  ];
  const environments = stages.map((stage, i) => ({
    environment_key: String(i + 1).repeat(64),
    name: "本机",
    target: { kind: "local" },
    adapter: i === 0 ? "codex-app-server" : "orca-terminal",
    agent: "Codex",
    status: stage === "ready" ? "ready" : "not_ready",
    stage,
    reason: `原原因 ${stage}`,
    current_revision: i + 1,
    activity_settled: true,
    profiles:
      i === 0 ? profiles : [{ id: `profile-${stage}`, name: `方案-${stage}` }],
    document_path: "SteerlineFlow/environment.md",
    document: "共享的准备清单",
    script_path: "scripts/verify-environment.ps1",
    source_commit: "c".repeat(40),
  }));
  await page.route("**/auth/status", (route) =>
    route.fulfill({
      json: {
        csrf_token: "csrf",
        identity: {
          home_id: "runtime-ui-home",
          instance_id: "web",
          ready: true,
        },
      },
    }),
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const project =
      path.match(/\/projects\/([a-f0-9]{64})/)?.[1] || projects[0];
    const identity = {
      project_id: project,
      instance_id: "instance-" + project[0],
      repository_identity: "C:/demo/" + project[0] + "/.git",
      ready: true,
    };
    const task = {
      task_id: "38",
      title: "环境返回任务",
      version: 1,
      status: "open",
      dependency_ready: true,
      startup_blockers: [],
      predecessors: [],
      successors: [],
      workflow_runs: [],
    };
    let data: unknown = {};
    if (route.request().method() !== "GET") {
      writes.push(route.request().postDataJSON());
      await route.fulfill({
        status: 409,
        json: { code: "fixture_read_only", message: "UI fixture" },
      });
      return;
    }
    if (path.endsWith("/environment/options")) {
      if (failOptions) {
        await route.fulfill({
          status: 503,
          json: { message: "fixture unavailable" },
        });
        return;
      }
      const visible = project === projects[0] ? environments : [];
      data = {
        environments: visible,
        profiles: visible.flatMap((e) =>
          e.profiles.map((p) => ({
            ...p,
            environment_key: e.environment_key,
            status: e.status,
            stage: e.stage,
            reason: e.reason,
            current_revision: e.current_revision,
          })),
        ),
        default_profile_id: "default",
        generators: [],
      };
    } else if (path.endsWith("/environment")) data = { records: [] };
    else if (path.endsWith("/projects"))
      data = {
        projects: projects.map((id, i) => ({
          project_id: id,
          config_id: id[0],
          name: `项目${i + 1}`,
          repository: `C:/demo/${i}`,
          online: true,
        })),
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
                capabilities: [
                  "start_task",
                  "start_candidate",
                  "set_dispatch_mode",
                  "pause_dispatch",
                ],
              },
              runs: [],
              control: {
                revision: 1,
                dispatch_mode: "manual",
                dispatch_paused: false,
                blockers: [],
              },
            },
          },
        },
      };
    else if (path.endsWith("/status")) data = { saved_digest: "same" };
    else if (path.endsWith("/maintenance"))
      data = { observation: { commit: "" }, tasks: [], profiles: [] };
    else if (path.endsWith("/presentation"))
      data = { presentation: { profiles, default_profile_id: "default" } };
    else if (/\/tasks(?:\/38)?$/.test(path))
      data = { tasks: [task], graph_revision: 1, has_more: false };
    else if (path.endsWith("/roadmaps")) data = { records: [] };
    await route.fulfill({ json: data });
  });
  await page.goto(
    `${origin}/projects/${projects[0]}/runs?task=38&start_profile=alternate`,
  );
  await expect(page.getByLabel("本次执行方案")).toHaveValue("alternate");
  // Make the selected environment unavailable; after returning from this page,
  // exercise the task's own preparation link as part of the round trip.
  environments[0].status = "not_ready";
  environments[0].stage = "stale";
  await page.getByRole("button", { name: "刷新查询" }).click();
  // Explicit navigation also covers a refresh with serialized task/profile.
  await page.goto(
    `${origin}/projects/${projects[0]}/environment?profile=alternate&task=38`,
  );
  await expect(
    page.getByRole("heading", { name: "运行环境", exact: true }),
  ).toBeVisible();
  const first = page.locator(".environment-card").first();
  await expect(first.getByLabel("验收执行方案")).toHaveValue("alternate");
  await expect(first).toBeFocused();
  await expect(page.getByText("阅读项目准备清单", { exact: true })).toHaveCount(
    1,
  );
  await page.reload();
  await expect(first.getByLabel("验收执行方案")).toHaveValue("alternate");
  await page.getByRole("link", { name: "返回任务", exact: true }).click();
  await expect(page.getByLabel("本次执行方案")).toHaveValue("alternate");
  await page.getByRole("link", { name: "前往环境准备", exact: true }).click();
  await expect(first.getByLabel("验收执行方案")).toHaveValue("alternate");
  environments[0].stage = "ready";
  environments[0].status = "ready";
  await page.getByRole("button", { name: "刷新环境" }).click();
  await expect(first.getByRole("heading")).toHaveText("Codex 直连 · 已就绪");
  for (const label of [
    "待准备与验收",
    "验收中",
    "验收阻断",
    "已失效",
    "未准备",
  ]) {
    await expect(
      page.locator(".environment-card h3").filter({ hasText: label }),
    ).toHaveCount(1);
  }
  failOptions = true;
  await page.getByRole("button", { name: "刷新环境" }).click();
  await expect(page.getByText(/上次已知状态，当前待核实/)).toBeVisible();
  await expect(first.getByRole("button", { name: "提交验收" })).toBeDisabled();
  failOptions = false;
  await page.getByRole("button", { name: "刷新环境" }).click();
  await expect(first.getByRole("button", { name: "提交验收" })).toBeEnabled();
  const invalidation = first.getByText("标记失效", { exact: true });
  await invalidation.focus();
  await page.keyboard.press("Enter");
  await expect(first.getByLabel("环境失效原因")).toBeVisible();
  await invalidation.focus();
  await page.keyboard.press("Enter");
  for (const width of [1440, 375]) {
    await page.setViewportSize({ width, height: 1000 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: info.outputPath(`runtime-environments-${width}.png`),
      fullPage: true,
    });
  }
  await page.goto(`${origin}/projects/${projects[1]}/environment`);
  await expect(
    page.getByRole("link", { name: "配置执行方案", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".environment-card")).toHaveCount(0);
  await expect(
    page.getByRole("link", { name: "返回任务", exact: true }),
  ).toHaveCount(0);
  expect(writes).toEqual([]);
  expect(errors).toEqual([]);
});

test("configuration navigation, unsaved history, shared concurrency and responsive layout", async ({
  page,
}, info) => {
  const project = "a".repeat(64);
  const identity = {
    project_id: project,
    instance_id: "project-instance",
    repository_identity: "C:/demo/.git",
    ready: true,
  };
  let snapshot = structuredClone(configSnapshot);
  let hasProjects = true;
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(String(error)));
  await page.route("**/auth/status", (route) =>
    route.fulfill({
      json: {
        csrf_token: "csrf",
        identity: {
          home_id: "browser-config-home",
          instance_id: "web",
          ready: true,
        },
      },
    }),
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    let data: unknown;
    if (path === "/api/web/execution-config") {
      if (route.request().method() === "PUT")
        snapshot = {
          ...snapshot,
          config: route.request().postDataJSON(),
          saved_digest: "saved",
        };
      data = snapshot;
    } else if (path.endsWith("/discover"))
      data = {
        state: "ready",
        origin: "fixture",
        models: [],
        environments: [],
        context_unverified: true,
      };
    else if (path.endsWith("/status"))
      data = { saved_digest: snapshot.saved_digest };
    else if (path.endsWith("/projects"))
      data = {
        projects: hasProjects
          ? [
              {
                project_id: project,
                config_id: "demo",
                name: "示例项目",
                repository: "C:/demo",
                online: true,
              },
            ]
          : [],
        loaded_execution_digest: "loaded",
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
              service: { instance_id: identity.instance_id },
              runs: [],
              control: {
                revision: 1,
                dispatch_mode: "manual",
                dispatch_paused: false,
              },
            },
          },
        },
      };
    else if (path.endsWith("/maintenance"))
      data = { observation: { commit: "" }, tasks: [], profiles: [] };
    else if (path.endsWith("/presentation"))
      data = { presentation: { profiles: [], default_profile_id: "" } };
    else data = { tasks: [], runs: [] };
    await route.fulfill({ json: data });
  });
  const target = `/projects/${project}/settings?task=selected#details`;
  await page.goto(origin + target);
  await expect(
    page.getByRole("heading", { name: "配置", exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("生成最大并发数")).toHaveValue("5");
  await page.screenshot({
    path: info.outputPath("configuration-desktop.png"),
    fullPage: true,
  });
  await page.getByRole("link", { name: "执行方案配置", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "执行方案配置", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".sidebar")).toHaveCount(1);
  await expect(page.locator(".topbar")).toHaveCount(1);
  await expect(page.getByRole("link", { name: "返回项目" })).toHaveAttribute(
    "href",
    target,
  );
  await page.screenshot({
    path: info.outputPath("profiles-desktop.png"),
    fullPage: true,
  });
  await page.getByLabel("配置名称").fill("未保存方案");
  page.once("dialog", (dialog) => dialog.dismiss());
  await page.goBack();
  await expect(page.getByLabel("配置名称")).toHaveValue("未保存方案");
  page.once("dialog", (dialog) => dialog.accept());
  await page.getByRole("link", { name: "返回项目" }).click();
  await expect(page).toHaveURL(origin + target);
  await page.getByLabel("生成最大并发数").fill("3");
  await page.getByRole("button", { name: "保存生成并发" }).click();
  await expect(
    page.getByText("生成并发已保存。重启 SteerLine 后生效。"),
  ).toBeVisible();
  expect(snapshot.config.max_generate_concurrency).toBe(3);
  expect(snapshot.config.execution_profiles).toEqual(
    configSnapshot.config.execution_profiles,
  );
  for (const width of [900, 375]) {
    await page.setViewportSize({ width, height: 1000 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: info.outputPath(`configuration-${width}.png`),
      fullPage: true,
    });
    await page.getByRole("link", { name: "执行方案配置", exact: true }).click();
    await expect(page.getByLabel("配置名称")).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: info.outputPath(`profiles-${width}.png`),
      fullPage: true,
    });
    await page.getByRole("link", { name: "返回项目" }).click();
  }
  hasProjects = false;
  await page.goto(origin + "/settings/execution-configs");
  await expect(page.getByRole("link", { name: "返回项目" })).toHaveCount(0);
  await page.getByRole("link", { name: "配置", exact: true }).click();
  await expect(page.getByLabel("生成最大并发数")).toHaveValue("3");
  expect(errors).toEqual([]);
});
