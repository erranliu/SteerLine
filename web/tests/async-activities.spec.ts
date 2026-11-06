import { test, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { join, resolve } from "node:path";
import { writeFileSync } from "node:fs";

test("activity drawer observes the owned project without driving execution", async ({
  page,
}) => {
  const web = process.env.STEERLINE_ACTIVITIES_WEB,
    home = process.env.STEERLINE_ACTIVITIES_HOME;
  const project = process.env.STEERLINE_ACTIVITIES_PROJECT,
    output = process.env.STEERLINE_WEB_EVIDENCE;
  if (
    !web ||
    !home ||
    !project ||
    !output ||
    !resolve(home, "../..").endsWith("runtime.async-activities")
  )
    throw new Error("explicit isolated activities inputs required");
  const opening = execFileSync(web, ["open", "-print-url"], {
    env: { ...process.env, STEERLINE_HOME: home },
    encoding: "utf8",
    timeout: 30000,
  }).trim();
  await page.goto(opening);
  await page.goto(new URL(`/projects/${project}/runs`, page.url()).href);
  const entry = page.getByRole("button", { name: /^活动 ·/ });
  await expect(entry).toBeVisible();
  await expect(page.getByRole("region", { name: /活动中心/ })).toHaveCount(0);
  await entry.click();
  const panel = page.getByRole("region", { name: /活动中心/ });
  await expect(panel.getByText("等待合并", { exact: true })).toBeVisible();
  await page.getByRole("link", { name: "任务 DAG", exact: true }).click();
  await expect(panel).toBeVisible();
  await page.screenshot({
    path: join(output, "activities-desktop.png"),
    fullPage: true,
  });
  await panel.getByRole("button", { name: /^最近结束/ }).click();
  await expect(panel.getByText("测试准备", { exact: true })).toBeVisible();
  await expect(panel.getByText("检测", { exact: true })).toBeVisible();
  await panel.getByRole("button", { name: /^进行中/ }).click();
  await page.route("**/activities?**", (route) => route.abort());
  await panel.getByRole("button", { name: "重新读取活动" }).click();
  await expect(panel.getByText(/连接或查询中断/)).toBeVisible();
  await expect(panel.getByText("等待合并", { exact: true })).toBeVisible();
  await page.unroute("**/activities?**");
  await panel.getByRole("button", { name: "重新读取活动" }).click();
  await expect(panel.getByText(/当前项目所有已接入活动/)).toBeVisible();
  await panel.getByRole("button", { name: "查看详情", exact: true }).click();
  await expect(page).toHaveURL(/\/runs\?task=.+&run=.+/);
  await expect(page.getByRole("region", { name: /活动中心/ })).toHaveCount(0);
  await entry.click();
  // 1440x1000 at 200% gives a 720x500 CSS layout viewport. This
  // checks reflow and reachable controls, not OS/browser chrome zoom.
  await page.setViewportSize({ width: 720, height: 500 });
  await expect(panel).toBeVisible();
  const closeBounds = await panel
    .getByRole("button", { name: "关闭活动中心" })
    .boundingBox();
  expect(
    closeBounds &&
      closeBounds.x >= 0 &&
      closeBounds.y >= 0 &&
      closeBounds.x + closeBounds.width <= 720 &&
      closeBounds.y + closeBounds.height <= 500,
  ).toBe(true);
  await page.screenshot({
    path: join(output, "activities-reflow-200-equivalent.png"),
    fullPage: true,
  });
  await panel.getByRole("button", { name: /^进行中/ }).focus();
  await page.setViewportSize({ width: 641, height: 760 });
  await expect(panel).toBeVisible();
  await page.setViewportSize({ width: 640, height: 760 });
  await expect(page.getByRole("dialog", { name: /活动中心/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /^进行中/ })).toBeFocused();
  await page.setViewportSize({ width: 320, height: 760 });
  const modal = page.getByRole("dialog", { name: /活动中心/ });
  await expect(modal).toBeVisible();
  await expect(modal).toHaveAttribute("aria-modal", "true");
  await expect
    .poll(() => page.evaluate(() => document.getElementById("root")?.inert))
    .toBe(true);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: join(output, "activities-mobile.png"),
    fullPage: true,
  });
  await modal.getByRole("button", { name: "关闭活动中心" }).focus();
  await page.keyboard.press("Escape");
  await expect(modal).toHaveCount(0);
  await expect(entry).toBeFocused();
  await expect
    .poll(() => page.evaluate(() => document.getElementById("root")?.inert))
    .toBe(false);
  await page.reload();
  await expect(entry).toBeVisible();
  await entry.click();
  await expect(
    page
      .getByRole("dialog", { name: /活动中心/ })
      .getByText("等待合并", { exact: true }),
  ).toBeVisible();
  writeFileSync(
    join(output, "activity-browser-proof.json"),
    JSON.stringify(
      {
        passed: true,
        desktop: true,
        narrow_320: true,
        breakpoint_640_641: true,
        reflow_200_equivalent: true,
        observation_outage_preserved: true,
        no_auto_open: true,
        navigation: true,
        refresh: true,
        focus_restored: true,
      },
      null,
      2,
    ),
  );
});
