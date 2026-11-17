import { test, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { join } from "node:path";
import { writeFileSync } from "node:fs";
test("maintenance report, history and refresh remain observational", async ({
  page,
}) => {
  const e = process.env,
    web = e.STEERLINE_MAINTENANCE_WEB,
    home = e.STEERLINE_MAINTENANCE_HOME,
    project = e.STEERLINE_MAINTENANCE_PROJECT,
    id = e.STEERLINE_MAINTENANCE_ID,
    out = e.STEERLINE_WEB_EVIDENCE;
  if (
    !web ||
    !home ||
    !project ||
    !out ||
    !home.includes("runtime.refactoring-maintenance")
  )
    throw new Error("isolated maintenance inputs required");
  const commands: string[] = [];
  let startRequest: unknown;
  page.on("request", (r) => {
    if (r.method() === "POST" && r.url().includes("/commands")) {
      const body = r.postDataJSON();
      commands.push(body?.action ?? "unknown");
      if (body?.action === "refactoring_start") startRequest = body;
    }
  });
  const opening = execFileSync(web, ["open", "-print-url"], {
    env: { ...e, STEERLINE_HOME: home },
    encoding: "utf8",
    timeout: 30000,
  }).trim();
  await page.goto(opening);
  const target = new URL(
    `/projects/${project}/refactoring?action=${id}`,
    page.url(),
  );
  await page.goto(target.href);
  await expect(
    page.getByRole("heading", { name: "重构与维护", exact: true }),
  ).toBeVisible();
  if (!id) {
    await page.getByRole("button", { name: "管理重构方向" }).click();
    const manager = page.getByRole("dialog", { name: "管理重构方向" });
    await expect(manager.getByText("更新过期文档")).toBeVisible();
    await manager.getByLabel("方向名称").fill("更新过期文档");
    await manager.getByLabel("方向正文").fill("浏览器流程测试");
    await manager.getByRole("button", { name: "保存方向" }).click();
    await expect(manager.getByRole("alert")).toContainText(
      "重构方向名称已存在",
    );
    await manager.getByLabel("方向名称").fill("浏览器测试方向");
    await manager.getByRole("button", { name: "保存方向" }).click();
    await expect(
      manager.getByRole("button", { name: "编辑 浏览器测试方向" }),
    ).toBeVisible();
    await manager.getByLabel("方向名称").fill("已改名测试方向");
    await manager.getByRole("button", { name: "保存方向" }).click();
    await expect(
      manager.getByRole("button", { name: "编辑 已改名测试方向" }),
    ).toBeVisible();
    await manager.getByRole("button", { name: "删除方向" }).click();
    await manager.getByRole("button", { name: "确认删除" }).click();
    await expect(manager.getByText("已删除方向")).toBeVisible();
    await expect(
      manager.getByRole("button", { name: "编辑 已改名测试方向" }),
    ).toHaveCount(0);
    await manager.getByRole("button", { name: "关闭" }).click();
    const opener = page.getByRole("button", { name: "新建重构任务" });
    await opener.focus();
    await page.keyboard.press("Enter");
    const dialog = page.getByRole("dialog", { name: "新建重构任务" });
    await expect(dialog).toBeVisible();
    const start = dialog.getByRole("button", {
      name: "启动重构任务",
      exact: true,
    });
    await expect(start).toBeEnabled();
    await expect(
      dialog.getByRole("heading", { name: "维护方向", exact: true }),
    ).toBeVisible();
    await expect(
      dialog.getByRole("heading", { name: "执行指令", exact: true }),
    ).toBeVisible();
    await expect(dialog.getByLabel("决策策略")).toHaveValue("cautious");
    await expect(dialog.getByLabel("重构轮次")).toHaveValue("1");
    const preview = dialog.getByRole("textbox", {
      name: "方向正文",
      exact: true,
    });
    await expect(preview).toHaveValue(/找出与当前实现/);
    await dialog.getByLabel("重构方向").selectOption({ label: "收拢共享夹具" });
    await expect(preview).toHaveValue("优先处理共享测试夹具。  \n");
    await preview.fill("优先处理共享测试夹具。  \n\nask-once");
    await dialog.getByLabel("重构轮次").fill("2");
    await page.screenshot({
      path: join(out, "maintenance-direction-desktop.png"),
      fullPage: true,
    });
    await page.setViewportSize({ width: 375, height: 900 });
    await expect(preview).toBeVisible();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(375);
    await page.screenshot({
      path: join(out, "maintenance-direction-mobile.png"),
      fullPage: true,
    });
    await page.setViewportSize({ width: 1440, height: 1000 });
    const [startResponse] = await Promise.all([
      page.waitForResponse(
        (response) =>
          response.url().endsWith("/commands") &&
          response.request().method() === "POST" &&
          response.request().postDataJSON()?.action === "refactoring_start",
        { timeout: 120_000 },
      ),
      start.click(),
    ]);
    expect(startResponse.ok()).toBe(true);
    await expect(
      page
        .getByRole("region", { name: "重构任务详情" })
        .getByText("待答复", { exact: true }),
    ).toBeVisible();
    expect(commands).toEqual(["refactoring_start"]);
    expect(startRequest).toBeTruthy();
    writeFileSync(join(out, "start.json"), JSON.stringify(startRequest));
    await page.screenshot({
      path: join(out, "maintenance-start.png"),
      fullPage: true,
    });
    return;
  }
  if (e.STEERLINE_MAINTENANCE_MODE === "answer") {
    await expect(
      page.getByText("是否保留公共接口？", { exact: true }),
    ).toBeVisible();
    await expect(page.getByText("维持兼容", { exact: true })).toBeVisible();
    const submit = page.getByRole("button", { name: "答复并继续" });
    await expect(submit).toBeDisabled();
    await page.getByRole("radio", { name: "保留公共接口" }).check();
    await page.getByLabel("补充说明（可选）").fill("保留原接口");
    await submit.click();
    await expect(submit).toHaveCount(0);
    expect(commands).toEqual(["refactoring_answer"]);
    return;
  }
  await expect(
    page
      .getByRole("region", { name: "重构任务详情" })
      .getByText("已交付", { exact: true }),
  ).toBeVisible();
  const detail = page.getByRole("region", { name: "重构任务详情" });
  const copy = detail.getByRole("button", { name: "复制调试信息" });
  await expect(copy).toBeVisible();
  const dialogue = detail.getByRole("region", { name: "执行对话" });
  const implementation = dialogue
    .getByRole("listitem")
    .filter({
      has: page.getByRole("heading", {
        name: "SteerAgent · 实施",
        exact: true,
      }),
    })
    .first();
  const reviews = dialogue.getByRole("listitem").filter({
    has: page.getByRole("heading", {
      name: "SteerAgent · PR 合入评审",
      exact: true,
    }),
  });
  await expect(reviews.getByText("展开结论", { exact: true })).toHaveCount(2);
  await expect(
    dialogue.getByRole("heading", { name: "SteerLine · 交付", exact: true }),
  ).toBeVisible();
  const latestReview = reviews.last();
  await latestReview.getByText("展开结论", { exact: true }).click();
  await expect(latestReview.getByText(/head.*base/)).toBeVisible();
  const direction = dialogue.getByText("优先处理共享测试夹具。", {
    exact: false,
  });
  const instruction = implementation.getByText("实施同方向的有限切片并验证。", {
    exact: true,
  });
  const decision = implementation.getByText("决策指令", { exact: true });
  const decisionBody = implementation.getByText(
    "根据 direction_prompt 中的重构方向判断",
    {
      exact: false,
    },
  );
  await expect(direction).toBeVisible();
  await expect(instruction).toBeVisible();
  await expect(
    dialogue.getByText("受控执行", { exact: true }).first(),
  ).toBeVisible();
  await expect(decisionBody).toBeHidden();
  await decision.click();
  await expect(decisionBody).toBeVisible();
  await expect(
    page.getByRole("button", { name: "处理冲突并重新评审" }),
  ).toBeVisible();
  const billingCoverage = detail.getByText(/不覆盖子\s*Agent/);
  await expect(billingCoverage).toBeHidden();
  await detail.getByText("运行信息", { exact: true }).click();
  await expect(billingCoverage).toBeVisible();
  await page.reload();
  await expect(latestReview).toBeVisible();
  await expect(latestReview.getByText(/head.*base/)).toBeHidden();
  await expect(decisionBody).toBeHidden();
  expect(commands).toEqual([]);
  await page.setViewportSize({ width: 375, height: 900 });
  await page.getByRole("button", { name: "返回重构任务列表" }).click();
  const list = page.getByRole("region", { name: "重构任务列表" });
  const row = list.getByRole("listitem").filter({
    has: page.locator(`[data-action="${id}"]`),
  });
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await row.getByRole("button", { name: "复制调试信息" }).click();
  await expect(row.getByRole("status")).toHaveText("已复制调试信息");
  const clipboard = await page.evaluate(() => navigator.clipboard.readText());
  expect(clipboard).toContain(`重构任务 ID：${id}`);
  expect(clipboard).toMatch(/Runner Run ID：run-/);
  await expect(list).toBeVisible();
  expect(commands).toEqual([]);
  await page.goBack();
  await expect(copy).toBeVisible();
  await expect(latestReview).toBeVisible();
  await expect(direction).toBeVisible();
  await expect(instruction).toBeVisible();
  await expect(decision).toBeVisible();
  await page.screenshot({
    path: join(out, "maintenance-mobile.png"),
    fullPage: true,
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  writeFileSync(
    join(out, "proof.json"),
    JSON.stringify({
      report: true,
      original_action: id,
      refresh_read_only: true,
      commands,
    }),
  );
});
