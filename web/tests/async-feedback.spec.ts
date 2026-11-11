import { test, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";

test("explicit submissions give immediate local feedback without a second coordinator", async ({
  page,
}) => {
  const web = process.env.STEERLINE_ACTIVITIES_WEB!,
    home = process.env.STEERLINE_ACTIVITIES_HOME!;
  const project = process.env.STEERLINE_ACTIVITIES_PROJECT!,
    output = process.env.STEERLINE_WEB_EVIDENCE!;
  const draft = process.env.STEERLINE_FEEDBACK_DRAFT!;
  if (
    !web ||
    !home ||
    !project ||
    !output ||
    !draft ||
    !resolve(home, "../..").endsWith("runtime.async-activities") ||
    !resolve(draft).startsWith(resolve(home, "..") + "\\")
  )
    throw new Error("owned feedback inputs required");
  const opening = execFileSync(web, ["open", "-print-url"], {
    env: { ...process.env, STEERLINE_HOME: home },
    encoding: "utf8",
    timeout: 30000,
  }).trim();
  const commands: { action: string; request_id: string }[] = [];
  const failures: string[] = [];
  page.on("pageerror", (error) => failures.push(error.message));
  page.on("request", (request) => {
    if (request.method() === "POST" && request.url().endsWith("/commands"))
      commands.push(request.postDataJSON());
  });
  // Delay delivery of the real response only, leaving the actual owner and
  // command unchanged. It gives the browser a stable submitting-state window.
  let delivered = 0;
  await page.route("**/commands", async (route) => {
    const response = await route.fetch();
    await new Promise((resolve) => setTimeout(resolve, 650));
    await route.fulfill({ response });
    delivered++;
  });
  await page.goto(opening);
  const root = new URL(`/projects/${project}`, page.url()).href;
  await page.goto(root + "/roadmaps");
  await expect(page.getByRole("switch", { name: "自动派发" })).toHaveAttribute(
    "aria-checked",
    "false",
  );
  const parse = page.getByRole("button", {
    name: "Runner generate · 解析任务与依赖",
    exact: true,
  });
  await expect(parse).toBeEnabled();
  // An intentionally stale source produces a real rejection before any native
  // generation; this peer deliberately supplies no generation results.
  const originalDraft = readFileSync(draft, "utf8");
  writeFileSync(
    draft,
    originalDraft + "\nChanged after the source was read.\n",
  );
  await parse.click();
  await expect(page.locator(".roadmaps-progress")).toHaveAttribute(
    "aria-busy",
    "true",
  );
  await expect(
    page.getByRole("button", { name: "正在提交解析…", exact: true }).first(),
  ).toBeDisabled();
  await expect(page.locator(".activity-feedback-flight")).toHaveCount(1);
  await expect(page.getByRole("button", { name: /^活动 ·/ })).toHaveAttribute(
    "aria-expanded",
    "false",
  );
  await page.screenshot({
    path: join(output, "roadmap-submitting.png"),
    fullPage: true,
  });
  await expect(page.getByText(/Markdown 已变化/).first()).toBeVisible();
  // Restore the owned input so the next, independent candidate test meets the
  // original clean-worktree readiness requirement. Never bypass admission.
  writeFileSync(draft, originalDraft);

  const maintenance = page.getByRole("region", { name: "项目维护" });
  await maintenance.getByRole("button", { name: /^项目维护/ }).click();
  await maintenance
    .getByRole("button", { name: "检查更新", exact: true })
    .click();
  await expect(
    maintenance.getByRole("button", { name: "正在检查更新…", exact: true }),
  ).toBeDisabled();
  await expect(
    maintenance.getByText("本次检查完成", { exact: true }),
  ).toBeVisible();

  await page.goto(root + "/runs?task=1");
  const start = page.getByRole("button", { name: "手动启动", exact: true });
  await expect(start).toBeEnabled();
  await start.click();
  await page.getByRole("button", { name: "确认启动", exact: true }).click();
  await expect(page).toHaveURL(/run=[a-f0-9]+/);
  await expect(
    page.getByRole("button", { name: "新增并行候选", exact: true }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "新增并行候选", exact: true }).click();
  await page.getByRole("button", { name: "确认启动", exact: true }).click();
  await expect
    .poll(
      () =>
        commands.filter((c) =>
          ["start_task", "start_candidate"].includes(c.action),
        ).length,
    )
    .toBe(2);
  await expect.poll(() => delivered).toBe(4);

  await page.goto(root + "/environment");
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.setViewportSize({ width: 320, height: 760 });
  const environment = page.getByRole("region", { name: "全部运行环境" });
  await expect(
    environment.getByRole("button", { name: "重新检测", exact: true }),
  ).toBeEnabled();
  const beforeWidth = await page.evaluate(
    () => document.documentElement.scrollWidth,
  );
  await environment
    .getByRole("button", { name: "重新检测", exact: true })
    .click();
  await expect(
    environment.getByText("正在提交检测…", { exact: true }).first(),
  ).toBeVisible();
  await expect(page.locator(".activity-feedback-flight")).toHaveCount(0);
  await page.screenshot({
    path: join(output, "environment-narrow-reduced.png"),
    fullPage: true,
  });
  const afterWidth = await page.evaluate(
    () => document.documentElement.scrollWidth,
  );
  expect(afterWidth).toBeLessThanOrEqual(Math.max(320, beforeWidth));
  const feedbackBounds = await environment
    .locator(".submission-status")
    .boundingBox();
  expect(
    feedbackBounds &&
      feedbackBounds.x >= 0 &&
      feedbackBounds.x + feedbackBounds.width <= 320,
  ).toBe(true);
  await expect
    .poll(() => commands.filter((c) => c.action === "environment_start").length)
    .toBe(1);
  await expect(environment.getByText("已就绪", { exact: true })).toBeVisible();
  await page.setViewportSize({ width: 720, height: 500 });
  await environment
    .getByRole("button", { name: "查看活动", exact: true })
    .click();
  await expect(page.getByRole("region", { name: /活动中心/ })).toBeVisible();
  await page.getByRole("button", { name: "关闭活动中心" }).focus();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: /^活动 ·/ })).toBeFocused();
  expect(failures).toEqual([]);
  for (const action of [
    "roadmap_parse",
    "maintenance_check",
    "start_task",
    "start_candidate",
    "environment_start",
  ])
    expect(
      commands.filter((command) => command.action === action),
      action,
    ).toHaveLength(1);
  expect(new Set(commands.map((command) => command.request_id)).size).toBe(
    commands.length,
  );
  writeFileSync(
    join(output, "feedback-browser-proof.json"),
    JSON.stringify(
      {
        passed: true,
        commands,
        real_roadmap_rejection: true,
        immediate_feedback: true,
        legal_parallel_candidate: true,
        narrow_reduced: true,
        narrow_layout: {
          viewport: 320,
          before: beforeWidth,
          after: afterWidth,
          feedback_inside_viewport: true,
        },
        no_page_errors: true,
      },
      null,
      2,
    ),
  );
});
