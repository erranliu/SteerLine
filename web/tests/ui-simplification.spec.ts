import { expect, test, type Page } from "@playwright/test";
import { createServer, type Server } from "node:http";
import { readFileSync } from "node:fs";
import { extname, resolve } from "node:path";
import type { ProjectBlocker } from "../src/shared/api/types";

test.setTimeout(120000);
test.use({ actionTimeout: 10000 });
let server: Server, origin: string;
test.beforeAll(async () => {
  const root = resolve(process.env.STEERLINE_UI_ASSETS_ROOT || "server/dist");
  server = createServer((request, response) => {
    const path = new URL(request.url!, "http://localhost").pathname;
    if (path.startsWith("/api/") || path.startsWith("/auth/")) {
      response.writeHead(500).end("Unmocked API");
      return;
    }
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
  server.closeAllConnections();
  await new Promise<void>((done, reject) =>
    server.close((e) => (e ? reject(e) : done())),
  );
});

const project = "a".repeat(64);
const now = () => new Date().toISOString();
function candidate(id = "candidate-one") {
  return {
    run_id: id,
    execution_id: "execution-" + id,
    attempt_id: "attempt-" + id,
    task_id: "1",
    domain_task_id: "1",
    task_status: "in_progress",
    participation: "active",
    submission_status: "accepted",
    stop_requested: false,
    workflow_status: "running",
    workflow_stage: "implement.execute",
    execution_status: "running",
    candidate_number: id.endsWith("one") ? 1 : 2,
    control_revision: 1,
    title: "实现登录功能",
    agent: "codex",
    host: "local",
    worktree_path: "C:/fixture/private-worktree",
    worktree_branch: "private-branch",
    execution_profile: {
      id: "default",
      name: "默认方案",
      host: "local",
      agent: "codex",
      details: [],
    },
    pr: { state: "none", url: "" },
    runner_delivery: {
      execution_wait: null,
      available: true,
      run_id: "execution-" + id,
      revision: 1,
      submission: "accepted",
      step: "implement.execute",
      status: "running",
      phase: "running",
      native_outcome: "running",
      final_verified: false,
      closed: false,
      resume_required: false,
      new_inputs_allowed: true,
      commands: [],
      controls: {
        can_reply: false,
        can_resume: false,
        can_stop: true,
        can_message: false,
        can_interrupt: false,
        can_close: false,
        reason: "",
      },
    },
  };
}
async function setup(page: Page) {
  const state = {
    runs: [candidate(), candidate("candidate-two")] as any[],
    failed: "",
    requests: [] as string[],
    errors: [] as string[],
    delivery: undefined as unknown,
    blockers: [] as ProjectBlocker[],
  };
  page.on("pageerror", (e) => state.errors.push(e.message));
  const task = () => ({
    task_id: "1",
    title: "实现登录功能",
    status: "in_progress",
    version: 1,
    spec_path: "spec.md",
    dependency_ready: true,
    startup_blockers: [],
    predecessors: [],
    successors: [],
    workflow_runs: [],
    delivery: state.delivery,
  });
  const taskPage = () => ({
    tasks: [task()],
    graph_revision: 1,
    database_revision: 1,
    database_id: "db",
    sampled_at: now(),
    has_more: false,
  });
  const readiness = () => ({
    status: state.blockers.length ? "not_ready" : "ready",
    blockers: state.blockers,
    total: 1,
    ready: 1,
  });
  await page.addInitScript(() =>
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async (text: string) => {
          (window as any).copied = text;
        },
      },
    }),
  );
  await page.route("**/auth/status", (r) =>
    r.fulfill({
      json: {
        csrf_token: "csrf",
        identity: { home_id: "ui-fixture", instance_id: "web", ready: true },
      },
    }),
  );
  await page.route("**/api/**", async (route) => {
    const request = route.request(),
      path = new URL(request.url()).pathname;
    state.requests.push(request.method() + " " + path);
    if (request.method() !== "GET")
      throw new Error("Unexpected write: " + path);
    if (state.failed && path.endsWith(state.failed)) {
      await route.abort();
      return;
    }
    let data: unknown;
    if (path.endsWith("/projects"))
      data = {
        projects: [
          {
            project_id: project,
            config_id: "fixture",
            name: "SteerlineTest",
            repository: "C:/fixture",
            online: true,
          },
        ],
        loaded_execution_digest: "same",
      };
    else if (path.endsWith("/identity"))
      data = {
        project_id: project,
        instance_id: "instance",
        repository_identity: "C:/fixture/.git",
        ready: true,
      };
    else if (path.endsWith("/snapshot"))
      data = {
        connection: {
          state: "connected",
          can_control: true,
          identity_matched: true,
          config_matched: true,
          snapshot: {
            sampled_at: now(),
            page: {
              service: { instance_id: "instance", capabilities: [] },
              runs: state.runs,
              control: {
                revision: 1,
                dispatch_mode: "manual",
                blockers: [],
              },
            },
          },
        },
      };
    else if (path.endsWith("/tasks") || path.endsWith("/tasks/1"))
      data = taskPage();
    else if (path.endsWith("/graph"))
      data = {
        ...taskPage(),
        nodes: [
          {
            task_id: "1",
            title: "实现登录功能",
            status: "in_progress",
            dependency_ready: true,
            blockers: [],
            startup_blockers: [],
          },
        ],
        edges: [],
        boundary: { truncated: false, omitted_node_count: 0, reasons: [] },
      };
    else if (path.endsWith("/main-agent"))
      data = { record: null, records: [], has_more: false };
    else if (path.endsWith("/presentation"))
      data = { presentation: { profiles: [], default_profile_id: "" } };
    else if (path.endsWith("/environment/options"))
      data = {
        environments: [],
        profiles: [],
        default_profile_id: "",
        environment_readiness_required: true,
      };
    else if (path.endsWith("/environment/history"))
      data = { records: [], has_more: false };
    else if (path.endsWith("/tests/catalogue"))
      data = { commit: "base", tests: [] };
    else if (path.endsWith("/maintenance/readiness")) data = readiness();
    else if (path.endsWith("/maintenance"))
      data = {
        readiness: readiness(),
        observation: {},
        remote_name: "main",
        remote: { sha: "", title: "", prs: [] },
        local: { status: "synced", ahead: 0, behind: 0, can_update: false },
        tasks: [],
      };
    else if (path.endsWith("/activities"))
      data = {
        project_id: project,
        instance_id: "instance",
        sampled_at: now(),
        items: [],
        request_links: {},
        sources: [],
        counts: { ongoing: 0, attention: 0, recent: 0 },
        counts_known: true,
        has_more: false,
      };
    else if (path.endsWith("/roadmaps"))
      data = {
        sources: [],
        records: [],
        profiles: [],
        documents_available: false,
        documents_reason: "选择草稿后生成文档",
        preparation: { ready: true, local_ready: true, files: [] },
      };
    else if (path.endsWith("/activity"))
      data = {
        page: {
          activity: Array.from({ length: 7 }, (_, i) => ({
            seq: i + 1,
            ts: "2026-11-10T00:00:00Z",
            kind: "worker_state",
            title: `阶段变化 ${i + 1}`,
            detail: "本次状态的业务说明",
          })),
          diagnostics: [],
        },
      };
    else if (path.endsWith("/last-message")) {
      const run = state.runs.find((r) => path.includes(r.run_id))!;
      data = {
        run_id: run.execution_id,
        attempt_id: run.attempt_id,
        message: {
          run_id: run.execution_id,
          attempt_id: run.attempt_id,
          observed_at: "2026-11-10T00:00:00Z",
          text: Array.from(
            { length: 10 },
            (_, i) => `正在验证登录流程，第 ${i + 1} 项处理。`,
          ).join("\n"),
        },
      };
    } else if (path.endsWith("/tests"))
      data = {
        status: "ready",
        total: 12,
        passed: 8,
        pending: 4,
        plan_path: "plan.md",
        progress_version: 1,
        observed_at: now(),
        rows: [
          {
            id: "test-one",
            name: "登录验证",
            level: "L1",
            environment: "isolated",
            command: "npm test",
            check: "真实断言通过",
            status: "pending",
          },
        ],
      };
    else if (path.endsWith("/usage"))
      data = {
        run_id: state.runs.find((r) => path.includes(r.run_id))!.run_id,
        execution_id: state.runs.find((r) => path.includes(r.run_id))!
          .execution_id,
        reason: "USAGE_UNSUPPORTED",
      };
    else if (path.endsWith("/final")) {
      const run = state.runs.find((r) => path.includes(r.run_id))!;
      data = {
        workflow_id: run.run_id,
        execution_id: run.execution_id,
        attempt_id: run.attempt_id,
        page: { events: [], has_more: false },
        bodies: [
          {
            item_key: "old",
            revision: 1,
            text: "这是先前轮次的回复，不能证明本轮完成。",
            complete: true,
            completeness: "complete",
          },
        ],
      };
    } else if (path.endsWith("/documents"))
      data = {
        commit: "abc",
        latest_commit: "def",
        files: [
          { path: "spec.md", before: "执行文档正文", after: "最新文档正文" },
        ],
      };
    else if (path.endsWith("/maintenance/tasks/1"))
      data = {
        commit: "abc",
        status: "ready",
        files: [{ path: "spec.md", body: "最新任务文档" }],
      };
    else if (path.endsWith("/usage-collection/status"))
      data = { usage_policy: { enabled: true }, usage_policy_revision: "1" };
    else if (path.endsWith("/status"))
      data = { saved_digest: "same", generation: 1 };
    else throw new Error("Unmapped API: " + path);
    await route.fulfill({ json: data });
  });
  return state;
}

async function screenshotStates(page: Page, info: any, name: string) {
  for (const width of [1440, 800, 375]) {
    await page.setViewportSize({ width, height: 1000 });
    await expect
      .poll(() =>
        page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      )
      .toBe(true);
    await page.screenshot({
      path: info.outputPath(`${name}-${width}.png`),
      fullPage: true,
    });
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
}
async function openRun(page: Page, id = "candidate-one") {
  await page.goto(`${origin}/projects/${project}/runs?task=1&run=${id}`);
  await expect(page.getByRole("region", { name: "当前进度" })).toBeVisible();
}
test("B1 production progress, bounded dynamics and business details", async ({
  page,
}, info) => {
  const state = await setup(page);
  await openRun(page);
  for (const name of ["当前进度", "最近动态", "测试与审查", "交付结果"])
    await expect(page.getByRole("region", { name })).toBeVisible();
  await expect(page.getByRole("region", { name: "当前进度" })).toContainText(
    "实施中",
  );
  await expect(page.getByText(/已通过 8/)).toBeVisible();
  const dynamic = page.getByRole("region", { name: "最近动态" });
  await expect(dynamic.locator(".activity-item")).toHaveCount(5);
  await expect(dynamic.getByText(/正在验证登录流程/)).toBeVisible();
  await expect(
    page.getByText(
      /C:\/fixture\/private-worktree|private-branch|观察 revision/,
    ),
  ).toHaveCount(0);
  await screenshotStates(page, info, "running");
  await dynamic.getByRole("button", { name: "展开完整消息" }).click();
  await dynamic.getByRole("button", { name: "更多活动" }).click();
  await expect(dynamic.locator(".activity-item")).toHaveCount(7);
  await page.getByRole("button", { name: "刷新查询" }).click();
  await expect(
    dynamic.getByRole("button", { name: "收起消息" }),
  ).toHaveAttribute("aria-expanded", "true");
  await expect(dynamic.locator(".activity-item")).toHaveCount(7);
  state.runs[0].runner_delivery.step = "unrecognized-stage";
  await page.getByRole("button", { name: "刷新查询" }).click();
  await expect(page.getByRole("region", { name: "当前进度" })).toContainText(
    "阶段待核实",
  );
  state.runs[0].runner_delivery.step = "audit.execute";
  await page.getByRole("button", { name: "刷新查询" }).click();
  await expect(page.getByRole("region", { name: "当前进度" })).toContainText(
    "审查中",
  );
  state.runs[0].runner_delivery.step = "implement.execute";
  await page.getByRole("button", { name: "刷新查询" }).click();
  await expect(page.getByRole("region", { name: "当前进度" })).toContainText(
    "实施中",
  );
  const tests = page.getByRole("region", { name: "测试与审查" });
  await tests.getByText("查看测试明细").focus();
  await page.keyboard.press("Enter");
  await tests.getByText(/登录验证 · L1/).click();
  await expect(tests.getByText("npm test")).toBeVisible();
  await tests.getByRole("button", { name: "任务文档", exact: true }).click();
  await tests.getByText(/spec.md · 内容已变化/).click();
  await expect(tests.getByText("执行文档正文")).toBeVisible();
  await page
    .getByRole("region", { name: "当前进度" })
    .getByRole("button", { name: "复制执行信息" })
    .click();
  expect(await page.evaluate(() => (window as any).copied)).toContain(
    "Runner Run ID：execution-candidate-one",
  );
  await page.getByRole("button", { name: "切换导航宽度" }).click();
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
  expect(state.errors).toEqual([]);
  expect(state.requests.every((r) => r.startsWith("GET "))).toBe(true);
  await info.attach("request-facts", {
    body: JSON.stringify(state.requests),
    contentType: "application/json",
  });
});
test("B2 actual wait, exhaustion, failed observations and stale content", async ({
  page,
}, info) => {
  const state = await setup(page);
  state.runs[0].runner_delivery.execution_wait = {
    contract: "steerrunner-execution-wait/v1",
    operation_id: "private-operation",
    query: "repository_inventory",
    stage: "backoff",
    reason_code: "orca_query_transient",
    started_at: new Date(Date.now() - 30000).toISOString(),
    updated_at: now(),
    deadline_at: new Date(Date.now() + 60000).toISOString(),
    calls_started: 2,
    max_calls: 5,
    next_attempt_at: new Date(Date.now() + 15000).toISOString(),
    diagnostic: "读取暂时失败",
  };
  await openRun(page);
  await expect(page.getByText(/已尝试 2\/5 次/)).toBeVisible();
  await screenshotStates(page, info, "waiting");
  for (const [stage, reason, title] of [
    ["queue", "orca_cli_busy", "等待 Orca 调用通道"],
    ["calling", "orca_query_running", "正在查询 Orca 仓库"],
  ]) {
    Object.assign(state.runs[0].runner_delivery.execution_wait, {
      stage,
      reason_code: reason,
      next_attempt_at: null,
    });
    await page.getByRole("button", { name: "刷新查询" }).click();
    await expect(
      page.getByRole("region", { name: "准备查询等待" }).getByText(title),
    ).toBeVisible();
  }
  state.runs[0].runner_delivery.execution_wait.calls_started = 5;
  state.runs[0].runner_delivery.execution_wait.stage = "exhausted";
  state.runs[0].runner_delivery.execution_wait.reason_code =
    "query_attempts_exhausted";
  state.runs[0].runner_delivery.status = "blocked";
  await page.getByRole("button", { name: "刷新查询" }).click();
  await expect(
    page
      .getByRole("region", { name: "准备查询等待" })
      .getByText("查询重试已耗尽"),
  ).toBeVisible();
  await screenshotStates(page, info, "failed");
  state.runs[0].runner_delivery.execution_wait = null;
  state.runs[0].runner_delivery.status = "running";
  state.runs[0].decision = {
    id: "question-one",
    version: 1,
    status: "pending",
    request: {
      question: "是否保留现有登录方式？",
      context: "需要你的决定才能继续。",
      options: [],
      interaction_kind: "blocking",
      blocks_progress: true,
    },
  };
  await page.getByRole("button", { name: "刷新查询" }).click();
  await expect(
    page.getByRole("heading", { name: "需要你的决定" }),
  ).toBeVisible();
  await expect(page.getByRole("region", { name: "准备查询等待" })).toHaveCount(
    0,
  );
  await screenshotStates(page, info, "waiting-answer");
  state.runs[0].decision.request.interaction_kind = "async";
  state.runs[0].decision.request.blocks_progress = false;
  await page.getByRole("button", { name: "刷新查询" }).click();
  await expect(page.getByRole("heading", { name: "可稍后回答" })).toBeVisible();
  state.runs[0].material_status = "outdated";
  state.failed = "/tests";
  await page.getByRole("button", { name: "刷新查询" }).click();
  await expect(page.getByText(/已过期 · 保留候选/)).toBeVisible();
  await expect(page.getByRole("progressbar")).toHaveCount(0);
  await expect(page.getByText(/测试查询失败/)).toBeVisible();
  state.failed = "/last-message";
  await expect(page.getByText(/消息读取失败/)).toBeVisible();
  await expect(page.getByText(/正在验证登录流程/)).toBeVisible();
  state.failed = "/snapshot";
  await page.getByRole("button", { name: "刷新查询" }).click();
  await expect(page.getByText("当前不可提交业务操作")).toBeVisible();
  await screenshotStates(page, info, "offline");
  expect(state.requests.every((r) => r.startsWith("GET "))).toBe(true);
});
test("B3 ended cold entry, reload, switching and explicit collapse retain history semantics", async ({
  page,
}, info) => {
  const state = await setup(page);
  for (const run of state.runs) {
    run.execution_status = "succeeded";
    run.runner_delivery.native_outcome = "succeeded";
    run.pr = { state: "merged", url: "https://example.com/pr" };
    run.runner_delivery.pr_state = "merged";
    run.runner_delivery.pr_checked_at = now();
  }
  await openRun(page);
  const result = page.getByRole("region", { name: "交付结果" });
  await expect(result.getByText("已合并，交付待核实")).toBeVisible();
  await expect(
    result.getByText("这是先前轮次的回复，不能证明本轮完成。"),
  ).toBeVisible();
  await expect(result.getByText("本轮尚未取得完整最终回复。")).toBeVisible();
  await screenshotStates(page, info, "ended-history");
  await result.getByRole("button", { name: "最终回复" }).click();
  await page.getByRole("button", { name: "刷新查询" }).click();
  await expect(
    result.getByRole("button", { name: "最终回复" }),
  ).toHaveAttribute("aria-expanded", "false");
  await page.reload();
  await expect(
    result.getByRole("button", { name: "最终回复" }),
  ).toHaveAttribute("aria-expanded", "true");
  await page.getByRole("button", { name: /候选 C2/ }).click();
  await expect(
    result.getByRole("button", { name: "最终回复" }),
  ).toHaveAttribute("aria-expanded", "true");
  state.delivery = { pr_url: "https://example.com/pr", merged_at: now() };
  await page.getByRole("button", { name: "刷新查询" }).click();
  await expect(result.getByText("任务已交付")).toBeVisible();
  const start = state.requests.length;
  await page.waitForTimeout(5200);
  const polls = state.requests.slice(start);
  expect(polls.filter((r) => r.endsWith("/final")).length).toBeLessThanOrEqual(
    2,
  );
  expect(
    polls.filter((r) => r.endsWith("/activity")).length,
  ).toBeLessThanOrEqual(3);
  expect(polls.every((r) => r.startsWith("GET "))).toBe(true);
  await info.attach("enabled-window-requests", {
    body: JSON.stringify(polls),
    contentType: "application/json",
  });
});
test("B4 routes, activity and project status remain reachable", async ({
  page,
}, info) => {
  const state = await setup(page);
  await openRun(page);
  for (const route of ["dag", "roadmaps", "environment"]) {
    await page.goto(`${origin}/projects/${project}/${route}`);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await screenshotStates(page, info, route);
  }
  await page.getByRole("button", { name: /^活动 ·/ }).click();
  await expect(page.getByRole("region", { name: /活动中心/ })).toBeVisible();
  for (const name of ["进行中 0", "需处理 0", "最近结束 0"])
    await page.getByRole("button", { name, exact: true }).click();
  await page.getByRole("button", { name: "关闭活动中心" }).click();
  state.blockers = [
    { category: "worktree", code: "worktree_dirty", diagnostic: "有本地修改" },
    {
      category: "worktree",
      code: "local_behind_remote",
      diagnostic: "落后 1 个提交",
    },
    {
      category: "test_plan",
      code: "test_plan_invalid",
      task_id: "1",
      diagnostic: "测试安排 1 无效",
    },
    {
      category: "test_plan",
      code: "test_plan_invalid",
      task_id: "2",
      diagnostic: "测试安排 2 无效",
    },
  ];
  await page.setViewportSize({ width: 375, height: 850 });
  await page.goto(`${origin}/projects/${project}/dag`);
  const status = page.locator(".project-readiness-status");
  const summary = status.locator("summary");
  await expect(summary).toHaveText("项目未就绪 · 未提交修改、未同步远端 +1");
  await expect(page.getByRole("region", { name: "项目状态" })).toHaveCount(0);
  await summary.click();
  await status.getByRole("link", { name: "项目状态" }).click();
  await expect(page).toHaveURL(/\/status$/);
  await expect(
    page.getByRole("button", { name: "检查更新", exact: true }),
  ).toBeVisible();
  const reasons = page.getByRole("region", { name: "未就绪原因" });
  await expect(reasons.getByRole("listitem")).toHaveCount(4);
  await expect(reasons.getByText("测试安排 2 无效")).toBeVisible();
  await screenshotStates(page, info, "project-status");
  await summary.click();
  await status.getByRole("link", { name: "测试管理" }).click();
  await expect(page).toHaveURL(/\/tests$/);
  await expect(page.getByRole("heading", { name: "测试管理" })).toBeVisible();
  expect(state.requests.every((r) => r.startsWith("GET "))).toBe(true);
  expect(state.errors).toEqual([]);
});

if (process.env.STEERLINE_UI_BASELINE === "1") {
  test("baseline corrected fixture before implementation", async ({
    page,
  }, info) => {
    const state = await setup(page);
    await page.goto(
      `${origin}/projects/${project}/runs?task=1&run=candidate-one`,
    );
    await expect(page.getByText(/正在验证登录流程/)).toBeVisible();
    await expect(page.getByText(/已通过 8/)).toBeVisible();
    for (const width of [1440, 800, 375]) {
      await page.setViewportSize({ width, height: 1000 });
      await page.screenshot({
        path: info.outputPath(`baseline-${width}.png`),
        fullPage: true,
      });
    }
    expect(state.errors).toEqual([]);
    expect(state.requests.every((r) => r.startsWith("GET "))).toBe(true);
    await info.attach("baseline-request-facts", {
      body: JSON.stringify(state.requests),
      contentType: "application/json",
    });
  });
}

test("B5 current suggestion defaults into one answer panel, preserves edits and replies once", async ({
  page,
}, info) => {
  const state = await setup(page);
  const run = state.runs[0];
  const question =
    "已补充实现与验收记录。部分验证尚未完成，需要确认后续处理方式。";
  const prompt =
    "请继续补齐当前任务缺少的验证，记录实际结果；遇到缺失环境时说明原因。";
  run.workflow_status = "waiting_input";
  run.recovery_version = 3;
  run.decision = {
    id: "question",
    attempt_id: run.attempt_id,
    version: 3,
    status: "pending",
    request: {
      question,
      context: question,
      options: [],
      interaction_kind: "blocking",
      blocks_progress: true,
      allow_free_text: true,
    },
  };
  run.error = question;
  run.runner_delivery.status = "waiting_input";
  run.runner_delivery.diagnostic = question;
  run.runner_delivery.controls.reason = "当前验证尚未完成";
  run.runner_delivery.controls.can_reply = true;
  const record = {
    id: "a".repeat(32),
    revision: 2,
    candidate_id: run.run_id,
    occurrence: run.attempt_id + ":question",
    state: "ready",
    settled: true,
    prompt,
    context: { goal: "继续当前问题", user_input: "", materials: [] },
  };
  const posts: any[] = [];
  await page.route("**/main-agent?*", async (route) => {
    const query = new URL(route.request().url()).searchParams;
    expect(query.get("candidate_id")).toBe(run.run_id);
    expect(query.get("occurrence")).toBe(record.occurrence);
    await route.fulfill({ json: { record } });
  });
  await page.route("**/decision-eligibility?*", (route) =>
    route.fulfill({ json: { eligible: true } }),
  );
  await page.route("**/commands", async (route) => {
    const envelope = route.request().postDataJSON();
    posts.push(envelope);
    await route.fulfill({
      json: {
        ...envelope,
        original: envelope,
        status: "applied",
        sampled_at: now(),
      },
    });
  });
  await openRun(page);
  const input = page.getByRole("textbox", { name: "回答内容" });
  const reply = page.getByRole("button", { name: "回复并继续", exact: true });
  await expect(input).toHaveValue(prompt);
  await expect(reply).toBeEnabled();
  await expect(page.getByText(question, { exact: true })).toHaveCount(1);
  await expect(
    page.getByText("当前验证尚未完成", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("执行观察 · 等待输入", { exact: true }),
  ).toHaveCount(0);
  await expect(page.getByRole("button", { name: "打开回答" })).toHaveCount(0);
  await expect(
    page
      .getByRole("region", { name: "测试与审查" })
      .getByText("对话续作 · 提示词建议"),
  ).toHaveCount(0);
  expect(posts).toEqual([]);
  await screenshotStates(page, info, "default-answer");
  const edited = "按当前方案继续。\n请保留实际未验项。";
  await input.fill(edited);
  // A native progress revision must not replace an edited draft.
  run.decision.version++;
  run.control_revision++;
  await page.getByRole("button", { name: "刷新查询" }).click();
  await expect(input).toHaveValue(edited);
  await page.reload();
  await expect(input).toHaveValue(edited);
  await expect(reply).toBeEnabled();
  await reply.click();
  await expect(
    page.getByText("回答已提交，等待状态更新。", { exact: true }),
  ).toBeVisible();
  expect(posts).toHaveLength(1);
  expect(posts[0]).toMatchObject({
    action: "answer_decision",
    target_id: "question",
    expected_version: 4,
    payload: {
      run_id: run.run_id,
      decision_id: "question",
      source_attempt_id: run.attempt_id,
      expected_run_revision: 2,
      expected_recovery_version: 3,
      note: edited,
    },
  });
  await page.screenshot({
    path: info.outputPath("replied.png"),
    fullPage: true,
  });
  expect(state.errors).toEqual([]);
  await info.attach("reply-envelope", {
    body: JSON.stringify(posts),
    contentType: "application/json",
  });
});
