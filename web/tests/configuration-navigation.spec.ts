import { test, expect } from "@playwright/test";
import { createServer, type Server } from "node:http";
import { readFileSync } from "node:fs";
import { resolve, extname } from "node:path";
import { configSnapshot } from "../src/test/execution-config";
import type { ProjectReadiness } from "../src/shared/api/types";

const projectReadiness: ProjectReadiness = {
  status: "not_ready",
  commit: "",
  total: 0,
  ready: 0,
  blockers: ["fixture project not prepared"],
};

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

test("continuation suggestion settings preserve drafts and unrelated configuration", async ({
  page,
}, info) => {
  let snapshot = structuredClone(configSnapshot);
  let loaded = snapshot.saved_digest;
  let rejectSave = true;
  const writes: unknown[] = [];
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(String(error)));
  await page.route("**/auth/status", (route) =>
    route.fulfill({
      json: {
        csrf_token: "csrf",
        identity: {
          home_id: "suggestion-browser-home",
          instance_id: "web",
          ready: true,
        },
      },
    }),
  );
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let data: unknown = {};
    if (path === "/api/web/execution-config") {
      if (request.method() === "PUT") {
        const edit = request.postDataJSON();
        writes.push(edit);
        expect(edit.saved_digest).toBe(snapshot.saved_digest);
        expect(request.headers()["if-match"]).toBe(
          '"' + snapshot.saved_digest + '"',
        );
        if (rejectSave) {
          await route.fulfill({
            status: 409,
            json: { code: "stale_saved_config", reason: "fixture conflict" },
          });
          return;
        }
        expect(edit.config).not.toHaveProperty("main_agent");
        expect(edit.config.execution_profiles).toEqual(
          configSnapshot.config.execution_profiles,
        );
        snapshot = {
          ...snapshot,
          config: edit.config,
          saved_digest: "saved-suggestion",
        };
      }
      data = snapshot;
    } else if (path.endsWith("/projects")) {
      data = { projects: [], loaded_execution_digest: loaded };
    } else if (path.endsWith("/status")) {
      data = { saved_digest: snapshot.saved_digest };
    } else if (path.endsWith("/discover")) {
      expect(request.postDataJSON()).toMatchObject({
        kind: "models",
        adapter: "codex-app-server",
        host: "local",
        agent: "codex",
      });
      data = {
        state: "ready",
        models: [
          {
            id: "browser-chosen",
            label: "Browser model",
            default_effort: "high",
            efforts: [
              { value: "high", label: "High" },
              { value: "medium", label: "Medium" },
            ],
          },
        ],
      };
    }
    await route.fulfill({ json: data });
  });
  await page.goto(origin + "/settings");
  await expect(
    page.getByRole("heading", { name: "对话续作 · 提示词建议", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("当前设置：gpt-6.1-sol / high")).toBeVisible();
  await page.getByRole("button", { name: "读取本地 Codex 模型" }).click();
  await page
    .getByLabel("对话续作 · 提示词建议 模型")
    .selectOption("browser-chosen");
  await page
    .getByLabel("对话续作 · 提示词建议 思考强度")
    .selectOption("medium");
  page.once("dialog", (dialog) => dialog.dismiss());
  await page.getByRole("link", { name: "执行方案配置", exact: true }).click();
  await expect(page.getByLabel("对话续作 · 提示词建议 模型")).toHaveValue(
    "browser-chosen",
  );
  const save = page.getByRole("button", {
    name: "保存对话续作 · 提示词建议 设置",
  });
  await save.click();
  await expect(page.getByText(/fixture conflict/)).toBeVisible();
  await expect(page.getByLabel("对话续作 · 提示词建议 思考强度")).toHaveValue(
    "medium",
  );
  expect(writes).toHaveLength(1);
  expect(snapshot.config.continuation_suggestion).toBeUndefined();
  rejectSave = false;
  await save.click();
  await expect(
    page.getByText("对话续作 · 提示词建议 设置已保存，重启 SteerLine 后生效。"),
  ).toBeVisible();
  expect(writes).toHaveLength(2);
  expect(snapshot.config.continuation_suggestion).toEqual({
    model: "browser-chosen",
    reasoning_effort: "medium",
  });
  for (const width of [900, 375]) {
    await page.setViewportSize({ width, height: 1000 });
    const panel = page.getByRole("region", {
      name: "对话续作 · 提示词建议 设置",
    });
    await expect(panel).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: info.outputPath(`suggestion-${width}.png`),
      fullPage: true,
    });
  }
  loaded = snapshot.saved_digest;
  await page.reload();
  await expect(
    page.getByText("当前设置：browser-chosen / medium"),
  ).toBeVisible();
  expect(writes).toHaveLength(2);
  expect(errors).toEqual([]);
});

test("global developer switches preserve drafts, require restart and disappear in release builds", async ({
  page,
}, info) => {
  let snapshot = structuredClone({
    ...configSnapshot,
    developer_available: true,
  });
  snapshot.config.developer = { skip_environment_readiness: false };
  let loaded = "loaded";
  let available = true;
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(String(error)));
  await page.route("**/auth/status", (route) =>
    route.fulfill({
      json: {
        csrf_token: "csrf",
        identity: {
          home_id: "developer-browser-home",
          instance_id: "web",
          ready: true,
        },
      },
    }),
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    let data: unknown = {};
    if (path === "/api/web/execution-config") {
      if (route.request().method() === "PUT") {
        expect(route.request().headers()["if-match"]).toBe(
          '"' + snapshot.saved_digest + '"',
        );
        const edit = route.request().postDataJSON();
        expect(edit.saved_digest).toBe(snapshot.saved_digest);
        expect(edit.profile_edits).toEqual([]);
        snapshot = {
          ...snapshot,
          config: edit.config,
          saved_digest: "saved",
        };
      }
      data = { ...snapshot, developer_available: available };
    } else if (path.endsWith("/status"))
      data = { saved_digest: snapshot.saved_digest };
    else if (path.endsWith("/projects"))
      data = { projects: [], loaded_execution_digest: loaded };
    else if (path.endsWith("/discover"))
      data = {
        state: "ready",
        models: [],
        environments: [],
        context_unverified: true,
      };
    await route.fulfill({ json: data });
  });
  await page.goto(origin + "/settings");
  const toggle = page.getByRole("switch", { name: "环境未就绪时阻止执行" });
  await expect(toggle).toBeChecked();
  await toggle.focus();
  await page.keyboard.press("Space");
  await expect(toggle).not.toBeChecked();
  await expect(
    page.getByText("当前生效状态：环境就绪阻塞已开启"),
  ).toBeVisible();
  page.once("dialog", (dialog) => dialog.dismiss());
  await page.getByRole("link", { name: "执行方案配置", exact: true }).click();
  await expect(toggle).not.toBeChecked();
  await page.getByRole("button", { name: "保存开发者配置" }).click();
  await expect(
    page.getByText("全局配置已保存，待重启生效", { exact: true }),
  ).toBeVisible();
  expect(snapshot.config.developer).toEqual({
    skip_environment_readiness: true,
  });
  expect(snapshot.config.execution_profiles).toEqual(
    configSnapshot.config.execution_profiles,
  );
  for (const width of [1440, 375]) {
    await page.setViewportSize({ width, height: 1000 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: info.outputPath(`developer-${width}.png`),
      fullPage: true,
    });
  }
  loaded = snapshot.saved_digest;
  await page.reload();
  await expect(
    page.getByText("当前生效状态：环境就绪阻塞已关闭"),
  ).toBeVisible();
  // The other editor must retain its unsaved protection with both panels mounted.
  await page.getByLabel("生成最大并发数").fill("7");
  page.once("dialog", (dialog) => dialog.dismiss());
  await page.getByRole("link", { name: "执行方案配置", exact: true }).click();
  await expect(page.getByLabel("生成最大并发数")).toHaveValue("7");
  await page.getByRole("button", { name: "取消修改", exact: true }).click();
  available = false;
  await page.reload();
  await expect(page.getByLabel("生成最大并发数")).toBeVisible();
  await expect(page.getByRole("region", { name: "开发者配置" })).toHaveCount(0);
  expect(errors).toEqual([]);
});

test("runtime environments keep row actions, failed observations and task return scoped", async ({
  page,
  context,
}, info) => {
  const projects = ["a".repeat(64), "b".repeat(64)];
  const remoteMachineID = "e68f4edf-8a83-42c2-a9da-6d404d3aa3d8";
  let failOptions = false;
  let holdQualification = false;
  let releaseQualification = () => {};
  const writes: unknown[] = [];
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(String(e)));
  const profiles = ["default", "alternate"].map((id) => ({
    id,
    name: id,
    model: id === "alternate" ? "model-high" : "model-default",
    reasoning_effort: "high",
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
    target:
      i === 1
        ? { kind: "orca_environment", environment: remoteMachineID }
        : { kind: "local" },
    adapter: i === 0 ? "codex-app-server" : "orca-terminal",
    agent: "Codex",
    status: stage === "ready" ? "ready" : "not_ready",
    stage,
    reason: `原原因 ${stage}`,
    current_revision: i + 1,
    activity_settled: true,
    activity_id: i === 0 ? "d".repeat(32) : undefined,
    profiles:
      i === 0 ? profiles : [{ id: `profile-${stage}`, name: `方案-${stage}` }],
    document_path: "SteerlineFlow/environment.md",
    document: "共享的准备清单",
    script_path: "SteerlineFlow/scripts/verify-environment.ps1",
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
    if (
      path === "/api/web/execution-config/discover" &&
      route.request().postDataJSON()?.kind === "environments"
    ) {
      await route.fulfill({
        json: { environments: [{ id: remoteMachineID, name: "远程工作站" }] },
      });
      return;
    }
    if (route.request().method() !== "GET") {
      writes.push(route.request().postDataJSON());
      await route.fulfill({
        status: 409,
        json: { code: "fixture_read_only", message: "UI fixture" },
      });
      return;
    }
    if (path.endsWith("/environment/options")) {
      const configuration =
        new URL(route.request().url()).searchParams.get("configuration") ===
        "true";
      if (!configuration && holdQualification)
        await new Promise<void>((resolve) => {
          releaseQualification = resolve;
        });
      if (!configuration && failOptions) {
        await route.fulfill({
          status: 503,
          json: { message: "fixture unavailable" },
        });
        return;
      }
      const visible =
        project === projects[0]
          ? environments.map((e) =>
              configuration
                ? {
                    ...e,
                    status: "not_ready",
                    stage: "unverified",
                    reason: "项目运行环境状态待核实",
                    current_revision: 0,
                  }
                : e,
            )
          : [];
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
    } else if (path.endsWith("/environment/materials"))
      data = {
        document_path: "SteerlineFlow/environment.md",
        document: "Target requirements",
        generation_ready: false,
        generation_diagnostic: "生成输入待维护",
      };
    else if (path.endsWith("/environment")) data = { records: [] };
    else if (path.endsWith("/environment/" + "d".repeat(32)))
      data = {
        id: "d".repeat(32),
        revision: 1,
        settled: true,
        state: "ready",
        status: "ready",
        created_at: "2026-11-02T06:30:00Z",
        rounds: [
          {
            run_id: "opaque-run-id",
            result: { content_base64: "opaque-file" },
          },
        ],
      };
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
                ],
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
    else if (path.endsWith("/maintenance/readiness")) data = projectReadiness;
    else if (path.endsWith("/maintenance"))
      data = {
        readiness: projectReadiness,
        observation: { commit: "" },
        tasks: [],
        profiles: [],
      };
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
  holdQualification = true;
  await page.goto(
    `${origin}/projects/${projects[0]}/environment?profile=alternate&task=38`,
  );
  await expect(
    page.getByRole("heading", { name: "项目运行环境", exact: true }),
  ).toBeVisible();
  const overview = page.getByRole("region", { name: "全部运行环境" });
  const environmentRow = (index: number) =>
    overview.locator(
      `[id="environment-${environments[index].environment_key}"]`,
    );
  const first = environmentRow(0);
  await expect(
    overview.getByRole("row", { name: /远程工作站 · Orca → Codex/ }),
  ).toHaveCount(1);
  await expect(overview.getByRole("row")).toHaveCount(7);
  await expect(first.locator(".environment-status")).toHaveText("待核实");
  await expect(
    first.getByRole("button", { name: /开始检测|重新检测/ }),
  ).toBeDisabled();
  await page.screenshot({
    path: info.outputPath("configuration-visible-qualification-pending.png"),
    fullPage: true,
  });
  holdQualification = false;
  releaseQualification();
  await expect(first.locator(".environment-status")).toHaveText("已失效");
  await expect(first.getByLabel("检测所用方案")).toHaveValue("alternate");
  await expect(
    first.getByLabel("检测所用方案").locator("option:checked"),
  ).toHaveText("alternate · model-high · high");
  await expect(first).toBeFocused();
  await expect(
    page.getByRole("heading", { name: "准备要求", exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(first.getByLabel("检测所用方案")).toHaveValue("alternate");
  await page.getByRole("link", { name: "返回任务", exact: true }).click();
  await expect(page.getByLabel("本次执行方案")).toHaveValue("alternate");
  await page.getByRole("link", { name: "前往环境准备", exact: true }).click();
  await expect(first.getByLabel("检测所用方案")).toHaveValue("alternate");
  environments[0].stage = "ready";
  environments[0].status = "ready";
  await page.getByRole("button", { name: "刷新环境" }).click();
  await expect(first.locator(".environment-status")).toHaveText("已就绪");
  const labels = [
    "已就绪",
    "待准备与检测",
    "检测中",
    "检测阻断",
    "已失效",
    "未准备",
  ];
  for (const [index, label] of labels.entries()) {
    await expect(
      environmentRow(index).locator(".environment-status"),
    ).toHaveText(label);
  }
  failOptions = true;
  await page.getByRole("button", { name: "刷新环境" }).click();
  await expect(page.getByText(/上次已知状态，当前待核实/)).toBeVisible();
  await expect(
    first.getByRole("button", { name: /开始检测|重新检测/ }),
  ).toBeDisabled();
  failOptions = false;
  await page.getByRole("button", { name: "刷新环境" }).click();
  await expect(
    first.getByRole("button", { name: /开始检测|重新检测/ }),
  ).toBeEnabled();
  await expect(page.getByText("更多设置", { exact: true })).toHaveCount(0);
  await expect(first.getByRole("button", { name: "标记失效" })).toHaveCount(0);
  await page.evaluate(
    ({ project }) =>
      new Promise<void>((resolve, reject) => {
        const request = indexedDB.open("steerline-web:runtime-ui-home", 1);
        request.onerror = () => reject(request.error);
        request.onsuccess = () => {
          const db = request.result;
          const tx = db.transaction("requests", "readwrite");
          const envelope = {
            request_id: "opaque-request-id",
            project_id: project,
            instance_id: "instance-a",
            action: "environment_start",
            target_id: project,
            expected_version: 1,
            payload: {},
          };
          tx.objectStore("requests").put({
            envelope,
            created_at: "2026-11-02T06:30:00Z",
            response: {
              ...envelope,
              status: "applied",
              diagnostic: "opaque-receipt-detail",
            },
          });
          tx.oncomplete = () => {
            db.close();
            resolve();
          };
          tx.onerror = () => {
            db.close();
            reject(tx.error);
          };
        };
      }),
    { project: projects[0] },
  );
  const operationResult = page.getByRole("region", { name: "操作结果" });
  await expect(
    operationResult.getByText("开始检测：操作成功", { exact: true }),
  ).toBeVisible();
  await expect(
    operationResult.getByRole("button", { name: "复制执行信息", exact: true }),
  ).toBeEnabled();
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await first
    .getByRole("button", { name: "复制执行信息", exact: true })
    .click();
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toContain("d".repeat(32));
  const bodyText = await page.locator("body").innerText();
  for (const hidden of [
    remoteMachineID,
    "d".repeat(32),
    "1".repeat(64),
    "c".repeat(40),
    "instance-a",
    "opaque-run-id",
    "opaque-request-id",
    "opaque-receipt-detail",
    "操作回执",
  ]) {
    expect(bodyText).not.toContain(hidden);
  }
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
  await expect(overview).toHaveCount(0);
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
      if (route.request().method() === "PUT") {
        const edit = route.request().postDataJSON();
        expect(edit.saved_digest).toBe(snapshot.saved_digest);
        expect(route.request().headers()["if-match"]).toBe(
          '"' + snapshot.saved_digest + '"',
        );
        expect(edit.profile_edits).toEqual([]);
        snapshot = {
          ...snapshot,
          config: edit.config,
          saved_digest: "saved",
        };
      }
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
                blockers: [],
              },
            },
          },
        },
      };
    else if (path.endsWith("/maintenance/readiness")) data = projectReadiness;
    else if (path.endsWith("/maintenance"))
      data = {
        readiness: projectReadiness,
        observation: { commit: "" },
        tasks: [],
        profiles: [],
      };
    else if (path.endsWith("/environment/options"))
      data = { profiles: [], environments: [], default_profile_id: "" };
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
  await page.locator("summary").filter({ hasText: "更多设置" }).click();
  await expect(page.getByText("开发执行权限固定为完全访问。")).toBeVisible();
  await expect(page.getByRole("combobox", { name: "权限模式" })).toHaveCount(0);
  await expect(
    page.getByRole("option", { name: /Runner 默认|沙箱/ }),
  ).toHaveCount(0);
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
