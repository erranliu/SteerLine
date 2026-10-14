import { test, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, resolve, join } from "node:path";

const config = process.env.STEERLINE_WEB_CONFIG;
test("web.smoke: original browser, task, feedback draft, final, delivery and offline", async ({
  page,
  context,
}, testInfo) => {
  if (!config)
    throw new Error(
      "STEERLINE_WEB_CONFIG must name a freshly generated simulate package configuration",
    );
  const absolute = resolve(config);
  const root = resolve(dirname(absolute), "../..");
  if (
    !root.toLowerCase().startsWith("c:\\steerlinedebug\\") ||
    absolute !== join(root, "home", "etc", "SteerlineTest.json")
  )
    throw new Error(
      "browser test must use only this C:\\SteerlineDebug package",
    );
  const manifest = JSON.parse(
    readFileSync(join(root, "manifest.json"), "utf8"),
  );
  const scheme = JSON.parse(readFileSync(join(root, "scheme.json"), "utf8"));
  if (
    manifest.status !== "ready" ||
    !manifest.web_address ||
    scheme.scheme !== "simulate"
  )
    throw new Error("ready Web-enabled simulate package required");
  const env = {
    ...process.env,
    STEERLINE_HOME: join(root, "home"),
    STEERLINE_E2E_CONTROLLED_STATE: join(root, "provider", "state.json"),
  };
  const binary = join(root, "bin", "steerline.exe");
  const helper = join(root, "bin", "debug-helper.exe");
  let opening = "";
  let openingError = "";
  await expect
    .poll(
      () => {
        try {
          opening = execFileSync(
            binary,
            ["web", "-config", absolute, "-print-url"],
            { env, encoding: "utf8", stdio: "pipe", timeout: 30000 },
          ).trim();
          return !!opening;
        } catch (error) {
          openingError = String(error);
          return false;
        }
      },
      { timeout: 180000, message: "original Web listener must become ready" },
    )
    .toBe(true)
    .catch(async (error) => {
      await testInfo.attach("opening-error", {
        body: openingError,
        contentType: "text/plain",
      });
      throw error;
    });
  const consoleErrors: string[] = [];
  page.on("pageerror", (error) => consoleErrors.push(error.message));
  await page.goto(opening);
  await expect(page.getByText("服务已连接", { exact: true })).toBeVisible();
  await expect(page).toHaveURL(/\/dag$/);
  expect(page.url()).not.toContain("ticket=");
  await expect(page.locator(".graph-task")).toHaveCount(2);
  await page.screenshot({
    path: testInfo.outputPath("dag-desktop.png"),
    fullPage: true,
  });
  const identity = await (
    await page.request.get(
      "http://" + manifest.web_address + "/api/v1/identity",
    )
  ).json();
  const prefix =
    "http://" +
    manifest.web_address +
    "/api/v1/projects/" +
    identity.project_id;
  const get = async (path: string) =>
    (await page.request.get(prefix + path)).json();
  const continuing = process.env.STEERLINE_WEB_CONTINUE === "1";
  if (!continuing) {
    let startPosts = 0;
    let originalStartID = "";
    await page.route("**/api/v1/projects/**/commands", async (route) => {
      const envelope = route.request().postDataJSON();
      if (envelope.action === "start_task") {
        startPosts++;
        originalStartID = envelope.request_id;
        if (startPosts === 1) {
          await route.fetch();
          await route.abort("failed");
          return;
        }
      }
      await route.continue();
    });
    if (
      await page.getByRole("button", { name: "恢复派发", exact: true }).count()
    )
      await page.getByRole("button", { name: "恢复派发", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "暂停派发", exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "任务列表", exact: true }).click();
    await page
      .getByRole("button")
      .filter({ hasText: "SteerlineTest 独立调试任务 A" })
      .click();
    await expect(
      page.getByRole("button", { name: "手动启动", exact: true }),
    ).toBeEnabled();
    await page.getByRole("button", { name: "手动启动", exact: true }).click();
    await page.getByRole("button", { name: "确认启动", exact: true }).click();
    await expect
      .poll(
        async () => {
          const snap = await get("/snapshot");
          return snap.connection.snapshot.page?.runs?.[0]?.pr?.state;
        },
        {
          timeout: 300000,
          message:
            "cold native preparation and original three-stage publication",
        },
      )
      .toBe("open");
    await page.reload();
    await expect(page.getByText("服务已连接", { exact: true })).toBeVisible();
    await expect
      .poll(async () => (await get("/commands/" + originalStartID)).status)
      .toBe("applied");
    expect(startPosts).toBe(1);
  }
  const snap = await get("/snapshot");
  const run = snap.connection.snapshot.page.runs[0];
  const tasksBefore = await get("/tasks");
  expect(
    tasksBefore.tasks.find((t: { task_id: string }) => t.task_id === "2")
      .dependency_ready,
  ).toBe(false);
  await page.getByRole("link", { name: /执行与待处理/ }).click();
  await expect(page.getByText("候选 C1", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "最终回复", exact: true }).click();
  await expect(
    page.getByText(/Controlled candidate pushed and PR created/),
  ).toBeVisible({ timeout: 90000 });
  await expect(
    page.getByRole("button", { name: "打开回答", exact: true }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "打开回答", exact: true }).click();
  await page
    .getByRole("textbox", { name: "回答内容" })
    .fill("中文验收反馈\n保持当前候选，核对 PR 元数据。");
  await page.reload();
  await expect(
    page.getByRole("button", { name: "打开回答", exact: true }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "打开回答", exact: true }).click();
  await expect(page.getByRole("textbox", { name: "回答内容" })).toHaveValue(
    "中文验收反馈\n保持当前候选，核对 PR 元数据。",
  );
  // The optional publish feedback exercises a second input in the original
  // controlled native thread when the Web fixture explicitly enables it.
  expect(
    JSON.parse(readFileSync(join(root, "provider", "state.json"), "utf8"))
      .web_feedback,
  ).toBe(true);
  {
    await page.getByLabel("修复发布或 PR 元数据", { exact: false }).check();
    await page.getByRole("button", { name: "提交回答", exact: true }).click();
    await expect
      .poll(
        async () =>
          JSON.parse(readFileSync(join(root, "provider", "state.json"), "utf8"))
            .turns.length,
        { timeout: 150000 },
      )
      .toBe(4);
    await expect
      .poll(
        async () => {
          const snapshot = await get("/snapshot");
          const current = snapshot.connection.snapshot.page.runs.find(
            (candidate: { run_id: string }) => candidate.run_id === run.run_id,
          );
          return [current.workflow_stage, current.workflow_status];
        },
        { timeout: 90000 },
      )
      .toEqual(["await_merge", "waiting_external"]);
    const after = await get("/snapshot");
    expect(after.connection.snapshot.page.runs[0].session_id).toBe(
      run.session_id,
    );
    expect(after.connection.snapshot.page.runs[0].execution_id).toBe(
      run.execution_id,
    );
    expect(after.connection.snapshot.page.runs[0].attempt_id).toBe(
      run.attempt_id,
    );
    expect(after.connection.snapshot.page.runs[0].current_turn_id).toBe(
      JSON.parse(readFileSync(join(root, "provider", "state.json"), "utf8"))
        .turns[3].id,
    );
  }
  await page.screenshot({
    path: testInfo.outputPath("execution-desktop.png"),
    fullPage: true,
  });
  await page.setViewportSize({ width: 420, height: 900 });
  await page.screenshot({
    path: testInfo.outputPath("execution-narrow.png"),
    fullPage: true,
  });
  await expect(page.locator("body")).toHaveJSProperty("scrollWidth", 420);
  await page.setViewportSize({ width: 1440, height: 1000 });
  execFileSync(helper, ["debug", "merge", "--simulate"], {
    env,
    encoding: "utf8",
  });
  await expect
    .poll(
      async () => {
        const tasks = await get("/tasks");
        return [
          !!tasks.tasks.find((t: { task_id: string }) => t.task_id === "1")
            .delivery,
          tasks.tasks.find((t: { task_id: string }) => t.task_id === "2")
            .dependency_ready,
        ];
      },
      { timeout: 30000 },
    )
    .toEqual([true, true]);
  await context.setOffline(true);
  await expect(
    page.getByText("当前不可提交业务操作", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /在 Codex 中查看/ }),
  ).toBeDisabled();
  await context.setOffline(false);
  await expect(page.getByText("服务已连接", { exact: true })).toBeVisible();
  const second = await context.newPage();
  await second.goto("http://" + manifest.web_address + "/dag");
  await expect(second.getByText("服务已连接", { exact: true })).toBeVisible();
  await second.close();
  expect((await get("/snapshot")).connection.snapshot.page.service.ready).toBe(
    true,
  );
  expect(consoleErrors).toEqual([]);
  await testInfo.attach("real-project-summary", {
    body: JSON.stringify({
      package: root,
      coverage: continuing ? "diagnostic-continuation" : "fresh-complete",
      source_commit: manifest.source_commit,
      source_dirty: manifest.source_dirty,
      project_id: identity.project_id,
      instance_id: identity.instance_id,
      run_id: run.run_id,
      execution_id: run.execution_id,
      session_id: run.session_id,
    }),
    contentType: "application/json",
  });
});
