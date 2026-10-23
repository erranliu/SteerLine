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
