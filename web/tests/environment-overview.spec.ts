import { expect, test } from "@playwright/test";
import { createServer, type Server } from "node:http";
import { readFileSync } from "node:fs";
import { extname, resolve } from "node:path";

let server: Server;
let origin: string;
test.beforeAll(async () => {
  const root = resolve("server/dist");
  server = createServer((request, response) => {
    const path = new URL(request.url!, "http://localhost").pathname;
    const file = resolve(
      root,
      path.startsWith("/assets/") ? "." + path : "index.html",
    );
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
    throw new Error("missing owned server");
  origin = `http://127.0.0.1:${address.port}`;
});
test.afterAll(async () => {
  await new Promise<void>((done, reject) =>
    server.close((error) => (error ? reject(error) : done())),
  );
});

test("all environment readiness remains visible with inline actions across refresh failures and responsive layouts", async ({
  page,
}, info) => {
  const project = "a".repeat(64);
  const identity = {
    project_id: project,
    instance_id: "overview-instance",
    ready: true,
    repository_identity: "C:/fixture/.git",
  };
  const entries = [
    {
      key: "direct",
      agent: "Codex",
      adapter: "codex-app-server",
      stage: "ready",
      reason: "",
      names: ["日常开发", "深度审查"],
    },
    {
      key: "orca",
      agent: "Codex",
      adapter: "orca-terminal",
      stage: "blocked",
      reason: "Agent 登录不可用",
      names: ["Orca 开发"],
    },
    {
      key: "dsh",
      agent: "DSH",
      adapter: "orca-terminal",
      stage: "verifying",
      reason: "正在检测依赖与登录",
      names: ["DSH 开发"],
    },
    {
      key: "pi",
      agent: "Pi",
      adapter: "orca-terminal",
      stage: "stale",
      reason: "准备材料已更新",
      names: ["Pi 开发"],
    },
  ];
  const profiles = entries.flatMap((entry) =>
    entry.names.map((name, i) => ({
      id: `${entry.key}-${i}`,
      name,
      environment_key: entry.key,
    })),
  );
  const environments = entries.map((entry) => ({
    environment_key: entry.key,
    target: { kind: "local" },
    adapter: entry.adapter,
    agent: entry.agent,
    stage: entry.stage,
    status: entry.stage === "ready" ? "ready" : "not_ready",
    reason: entry.reason,
    profiles: profiles.filter(
      (profile) => profile.environment_key === entry.key,
    ),
    current_revision: 1,
    activity_settled: true,
    document_path: "SteerlineFlow/environment.md",
  }));
  let failure = false;
  const writes: string[] = [];
  const reads: string[] = [];
  const errors: string[] = [];
  const repairPrompt = readFileSync(
    resolve("../internal/prompts/testdata/environment-repair.golden.md"),
    "utf8",
  );
  await page.addInitScript(() => {
    Object.defineProperty(navigator, "clipboard", {
      value: {
        writeText: async (text: string) => {
          (window as unknown as { copiedPrompt: string }).copiedPrompt = text;
        },
      },
    });
  });
  page.on("pageerror", (error) => errors.push(error.message));
  await page.route("**/auth/status", (route) =>
    route.fulfill({
      json: {
        csrf_token: "csrf",
        identity: { home_id: "environment-overview" },
      },
    }),
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (route.request().method() !== "GET") writes.push(path);
    reads.push(path);
    let data: unknown = {};
    if (path.endsWith("/projects"))
      data = {
        projects: [
          {
            project_id: project,
            config_id: "fixture",
            name: "运行环境验收",
            repository: "C:/fixture",
            online: true,
          },
        ],
      };
    else if (path.endsWith("/identity")) data = identity;
    else if (path.endsWith("/maintenance/readiness"))
      data = { status: "ready", commit: "fixture", total: 0, ready: 0, blockers: [] };
    else if (path.endsWith("/snapshot"))
      data = {
        connection: {
          state: "connected",
          can_control: true,
          identity_matched: true,
          config_matched: true,
          snapshot: {
            page: {
              service: { instance_id: identity.instance_id, capabilities: [] },
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
    else if (path.endsWith("/environment/options")) {
      if (failure) {
        await route.fulfill({
          status: 503,
          json: { code: "unavailable", reason: "fixture observation failure" },
        });
        return;
      }
      data = {
        repair_prompt: repairPrompt,
        environments,
        profiles,
        default_profile_id: "orca-0",
        environment_readiness_required: false,
      };
    } else if (path.endsWith("/environment/materials"))
      data = { document: "准备 Git 和 Agent 登录，再开始检测。" };
    else if (path.endsWith("/environment")) data = { records: [] };
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
    await route.fulfill({ json: data });
  });
  await page.goto(`${origin}/projects/${project}/environment`);
  const overview = page.getByRole("region", { name: "全部运行环境" });
  const rows = overview.locator("tbody tr");
  await expect(rows).toHaveCount(4);
  const copyPrompt = page.getByRole("button", { name: "复制修正环境提示词" });
  await expect(copyPrompt).toBeEnabled();
  await copyPrompt.click();
  await expect(
    page.getByText("已复制修正环境提示词", { exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => (window as unknown as { copiedPrompt: string }).copiedPrompt,
    ),
  ).toBe(repairPrompt);
  await expect(overview.getByRole("status")).toContainText("1/4 已就绪");
  await expect(rows.locator("th[scope=row]")).toHaveText([
    "本机 · Orca → Codex",
    "本机 · Orca → Pi",
    "本机 · Orca → DSH",
    "本机 · Codex 直连",
  ]);
  await expect(page.getByLabel("运行环境", { exact: true })).toHaveCount(0);
  const materialsSummary = page.locator("summary").filter({
    hasText: /^准备要求$/,
  });
  const materialsDocument = page.getByText(
    "准备 Git 和 Agent 登录，再开始检测。",
  );
  await expect(materialsSummary).toBeVisible();
  await expect(materialsDocument).toBeHidden();
  expect(
    reads.some((path) => path.endsWith("/environment/materials")),
  ).toBe(false);
  await materialsSummary.click();
  await expect(materialsDocument).toBeVisible();
  expect(
    reads.filter((path) => path.endsWith("/environment/materials")),
  ).toHaveLength(1);
  await materialsSummary.click();
  await expect(materialsDocument).toBeHidden();
  await expect(
    page.getByRole("button", { name: /查看详情|收起详情|标记失效/ }),
  ).toHaveCount(0);
  await expect(
    rows.nth(1).getByRole("button", { name: "开始检测" }),
  ).toBeEnabled();
  for (const width of [1440, 800, 375, 960, 390, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    await expect(rows).toHaveCount(4);
    await expect(copyPrompt).toBeVisible();
    const bounds = await page
      .locator(".environment-page")
      .evaluate((element) => {
        const box = element.getBoundingClientRect();
        return {
          left: box.left,
          right: box.right,
          scroll: element.scrollWidth,
          width: element.clientWidth,
        };
      });
    expect(bounds.left).toBeGreaterThanOrEqual(0);
    expect(bounds.right).toBeLessThanOrEqual(width + 1);
    expect(bounds.scroll).toBeLessThanOrEqual(bounds.width + 1);
    await expect(
      rows.first().getByText("检测阻断", { exact: true }),
    ).toBeVisible();
    await page
      .locator(".environment-page")
      .screenshot({ path: info.outputPath(`overview-${width}.png`) });
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
  await expect(rows).toHaveCount(4);
  environments[0].stage = "blocked";
  environments[0].status = "not_ready";
  environments[0].reason = "认证已失效";
  await page.getByRole("button", { name: "刷新环境", exact: true }).click();
  await expect(rows.last()).toContainText("认证已失效");
  await expect(overview.getByRole("status")).toContainText("0/4 已就绪");
  await expect(rows.first()).toContainText("本机 · Orca → Codex");
  failure = true;
  await page.getByRole("button", { name: "刷新环境", exact: true }).click();
  await expect(overview.getByRole("status")).toContainText(
    "共 4 个环境 · 当前状态待核实",
  );
  await expect(overview.getByText("待核实", { exact: true })).toHaveCount(4);
  expect(writes).toEqual([]);

  failure = false;
  await page.setViewportSize({ width: 320, height: 760 });
  await page.getByRole("button", { name: "刷新环境", exact: true }).click();
  const pi = rows.nth(1);
  const start = pi.getByRole("button", { name: "开始检测", exact: true });
  await expect(start).toBeEnabled();
  let finishSubmission!: () => void;
  const responseAllowed = new Promise<void>((resolve) => {
    finishSubmission = resolve;
  });
  const commands: unknown[] = [];
  await page.route("**/commands", async (route) => {
    const envelope = route.request().postDataJSON();
    commands.push(envelope);
    await responseAllowed;
    await route.fulfill({
      json: {
        ...envelope,
        original: envelope,
        status: "rejected",
        code: "fixture_rejected",
      },
    });
  });
  try {
    await start.click();
    await expect(pi.locator(".submission-status")).toContainText(
      "正在提交检测…",
    );
    await expect(
      pi.getByRole("button", { name: "正在提交检测…" }),
    ).toBeDisabled();
    await expect(
      rows.first().getByRole("button", { name: "开始检测" }),
    ).toBeEnabled();
    await expect
      .poll(() => commands)
      .toEqual([
        expect.objectContaining({
          action: "environment_start",
          target_id: project,
          payload: {
            environment_key: "pi",
            profile_id: "pi-0",
            expected_current_revision: 1,
            round_timeout_seconds: 3600,
          },
        }),
      ]);
  } finally {
    finishSubmission();
  }
  await expect(start).toBeEnabled();
  expect(errors).toEqual([]);
});
