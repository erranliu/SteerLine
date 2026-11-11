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
    const start = page.getByRole("button", { name: "开始维护", exact: true });
    await expect(start).toBeEnabled();
    await expect(page.getByLabel("决策策略")).toHaveValue("cautious");
    await expect(page.getByLabel("重构轮次")).toHaveValue("1");
    const preview = page.getByLabel("方向提示词", { exact: true });
    await expect(preview).toContainText("简化不必要的抽象与控制流。");
    await page.getByLabel("重构方向").selectOption("custom:shared-fixtures");
    await expect
      .poll(() => preview.textContent())
      .toBe("# 收拢共享夹具\r\n\r\n优先处理共享测试夹具。  \r\n");
    await page.getByLabel("补充要求").fill("ask-once");
    await page.getByLabel("重构轮次").fill("2");
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
    await start.click();
    await expect(
      page.getByRole("heading", { name: "等待决策", exact: true }),
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
  await expect(
    page.getByRole("heading", { name: "已交付", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("heading", { name: "审计报告 2" })).toBeVisible();
  const detail = page.getByRole("region", { name: "维护详情" });
  await detail.getByText("方向提示词", { exact: true }).click();
  await expect(
    detail.getByText("优先处理共享测试夹具。", { exact: false }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "处理冲突并更新报告" }),
  ).toBeVisible();
  await expect(page.getByText(/不覆盖子 Agent/)).toBeVisible();
  await page.reload();
  await expect(page.getByRole("heading", { name: "审计报告 2" })).toBeVisible();
  expect(commands).toEqual([]);
  await page.setViewportSize({ width: 375, height: 900 });
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
