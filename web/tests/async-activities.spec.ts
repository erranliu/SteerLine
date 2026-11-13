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
  await expect(panel.getByText("测试准备", { exact: true })).toHaveCount(0);
  await expect(panel.getByText(/^检测开发环境 ·/)).toBeVisible();
  const recent = await page.request.get(
    new URL(
      `/api/v2/application/projects/${project}/activities?filter=recent`,
      page.url(),
    ).href,
  );
  expect(recent.ok()).toBe(true);
  expect(
    (await recent.json()).items.every(
      (item: { kind: string }) => item.kind !== "test_preparation",
    ),
  ).toBe(true);
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

test("independent activity page reads conversation and bodies only on demand", async ({
  page,
}) => {
  const web = process.env.STEERLINE_ACTIVITIES_WEB,
    home = process.env.STEERLINE_ACTIVITIES_HOME,
    project = process.env.STEERLINE_ACTIVITIES_PROJECT,
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
  const response = await page.request.get(
    new URL(
      `/api/v2/application/projects/${project}/activities?filter=ongoing`,
      page.url(),
    ).href,
  );
  expect(response.ok()).toBe(true);
  const activities = await response.json();
  const candidate = activities.items.find(
    (a: { source: string }) => a.source === "task_candidate",
  );
  expect(candidate).toBeTruthy();
  const requests = {
    indexes: 0,
    bodies: 0,
    changes: 0,
    history: 0,
    conversation: 0,
  };
  const turnStartedAt = "2026-11-13T08:10:11Z";
  await page.route(
    `**/activities/${encodeURIComponent(candidate.activity_id)}/conversation*`,
    async (route) => {
      const url = new URL(route.request().url());
      if (!url.searchParams.has("session")) {
        await route.fulfill({
          json: {
            activity_id: candidate.activity_id,
            targets: [
              {
                id: "original",
                label: "原会话",
                run_id: "run",
                attempt_id: "attempt",
                state: "available",
              },
            ],
          },
        });
        return;
      }
      requests.conversation++;
      await route.fulfill({
        json: {
          activity_id: candidate.activity_id,
          target_id: "original",
          run_id: "run",
          attempt_id: "attempt",
          session_id: "session",
          generation: "generation",
          partial: false,
          messages: [
            {
              id: "current",
              role: "assistant",
              text: "有时间的历史消息",
              turn_started_at: turnStartedAt,
            },
            { id: "undated", role: "assistant", text: "没有时间的历史消息" },
          ],
        },
      });
    },
  );
  page.on("request", (request) => {
    const url = request.url();
    if (url.includes("/activities/") && url.includes("/interactions?"))
      requests.indexes++;
    if (url.includes("/activities/") && url.includes("/body?"))
      requests.bodies++;
    if (url.includes("/activities/") && url.endsWith("/changes"))
      requests.changes++;
    if (url.includes("/activities?filter=history")) requests.history++;
  });
  await page.goto(
    new URL(
      `/projects/${project}/activities?activity=${encodeURIComponent(candidate.activity_id)}`,
      page.url(),
    ).href,
  );
  const reader = page.getByRole("region", {
    name: "活动中心独立页",
    exact: true,
  });
  await expect(
    reader.getByRole("heading", { name: candidate.title, exact: true }),
  ).toBeVisible();
  expect(requests.indexes).toBe(0);
  expect(requests.bodies).toBe(0);
  expect(requests.conversation).toBe(0);
  await reader.getByRole("button", { name: "查看对话", exact: true }).click();
  const conversation = reader.getByRole("region", {
    name: "原会话对话",
    exact: true,
  });
  const turnTime = conversation.locator("time");
  await expect(turnTime).toHaveAttribute("datetime", turnStartedAt);
  await expect(conversation.getByText("时间未记录")).toBeVisible();
  expect(requests.conversation).toBe(1);
  const firstIndex = page.waitForResponse(
    (r) => r.url().includes("/interactions?") && r.request().method() === "GET",
  );
  await reader
    .getByRole("button", { name: "查看已记录交互", exact: true })
    .click();
  const indexResponse = await firstIndex;
  expect(indexResponse.ok()).toBe(true);
  const index = await indexResponse.json();
  await expect(
    reader.getByRole("region", { name: "已记录的交互", exact: true }),
  ).toBeVisible();
  expect(requests.indexes).toBe(1);
  expect(requests.bodies).toBe(0);
  await expect
    .poll(() => requests.changes, { timeout: 15000 })
    .toBeGreaterThanOrEqual(2);
  expect(requests.indexes).toBe(1);
  expect(requests.bodies).toBe(0);
  const expand = reader.getByRole("button", { name: "展开正文", exact: true });
  if (await expand.count()) {
    const bodyResponse = page.waitForResponse(
      (r) => r.url().includes("/body?") && r.request().method() === "GET",
    );
    await expand.first().click();
    expect((await bodyResponse).ok()).toBe(true);
    expect(requests.bodies).toBe(1);
  } else {
    expect(
      index.items.every(
        (item: { preview: string; bytes: number }) =>
          Buffer.byteLength(item.preview, "utf8") === item.bytes,
      ),
    ).toBe(true);
  }
  for (const width of [1440, 768, 320]) {
    await page.setViewportSize({ width, height: 900 });
    await expect(turnTime).toBeVisible();
    await expect(
      reader.getByRole("button", { name: "收起已记录交互", exact: true }),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: join(output, `activity-reading-${width}.png`),
      fullPage: true,
    });
  }
  await reader
    .getByRole("button", { name: "返回活动列表", exact: true })
    .click();
  const card = reader
    .locator(".reading-select")
    .filter({ hasText: candidate.title })
    .first();
  await card.focus();
  await page.keyboard.press("Enter");
  const back = reader.getByRole("button", {
    name: "返回活动列表",
    exact: true,
  });
  await expect(back).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(card).toBeFocused();
  await reader.getByRole("button", { name: /^历史/ }).click();
  await expect(reader.getByRole("button", { name: /^历史/ })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect.poll(() => requests.history).toBeGreaterThan(0);
  const historyReads = requests.history;
  await page.evaluate(() => {
    window.dispatchEvent(new Event("blur"));
    window.dispatchEvent(new Event("focus"));
  });
  await expect(
    reader.getByRole("heading", { name: "活动中心", exact: true }),
  ).toBeVisible();
  expect(requests.history).toBe(historyReads);
  expect(requests.indexes).toBe(1);
  writeFileSync(
    join(output, "activity-reading-proof.json"),
    JSON.stringify(
      {
        passed: true,
        summary_only_default: true,
        deep_link_collapsed: true,
        explicit_index_page: true,
        no_automatic_index_paging: true,
        explicit_body_or_complete_preview: true,
        history_no_focus_refetch: true,
        narrow_keyboard_navigation: true,
        viewport_widths: [1440, 768, 320],
        requests,
      },
      null,
      2,
    ),
  );
});
