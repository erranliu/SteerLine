import { test, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { join } from "node:path";
import { writeFileSync } from "node:fs";

test("owned advisory navigation, context, copy and cancellation", async ({
  page,
  context,
}) => {
  const e = process.env,
    web = e.STEERLINE_ADVISORY_WEB,
    home = e.STEERLINE_ADVISORY_HOME,
    project = e.STEERLINE_ADVISORY_PROJECT,
    candidate = e.STEERLINE_ADVISORY_CANDIDATE,
    task = e.STEERLINE_ADVISORY_TASK,
    id = e.STEERLINE_ADVISORY_ID,
    mode = e.STEERLINE_ADVISORY_MODE,
    output = e.STEERLINE_WEB_EVIDENCE;
  if (
    !web ||
    !home ||
    !project ||
    !candidate ||
    !task ||
    !id ||
    !output ||
    !["ready", "cancel"].includes(mode || "") ||
    !home.includes("runtime.main-agent-advisory")
  )
    throw new Error("isolated advisory inputs required");
  const commands: string[] = [];
  let cancelAfterProgress = false;
  page.on("request", (r) => {
    if (r.method() === "POST" && r.url().includes("/commands"))
      commands.push(r.postDataJSON()?.action || "unknown");
  });
  const opening = execFileSync(web, ["open", "-print-url"], {
    env: { ...e, STEERLINE_HOME: home },
    encoding: "utf8",
    timeout: 30000,
  }).trim();
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto(opening);
  const base = new URL(`/projects/${project}/runs`, page.url());
  if (mode === "ready") {
    await page.goto(base.href);
    await page.getByRole("button", { name: /^活动 ·/ }).click();
    await page
      .getByRole("region", { name: "活动中心 · SteerLine", exact: true })
      .getByRole("button", { name: /^需处理 / })
      .click();
    const card = page.locator(
      `[data-activity-id="${e.STEERLINE_ADVISORY_ACTIVITY}"]`,
    );
    await expect(
      card.getByText("提示词建议已生成", { exact: true }),
    ).toBeVisible();
    await card.getByRole("button", { name: "去处理", exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`advisory=${id}`));
    const url = new URL(page.url());
    expect(url.searchParams.get("task")).toBe(task);
    expect(url.searchParams.get("run")).toBe(candidate);
  } else {
    base.searchParams.set("task", task);
    base.searchParams.set("run", candidate);
    base.searchParams.set("advisory", id);
    await page.goto(base.href);
  }
  const panel = page
    .getByText("后续对话提示词建议", { exact: true })
    .locator("..");
  await expect(panel).toHaveAttribute("open", "");
  await panel.getByText("查看本次已提供的上下文", { exact: true }).click();
  await expect(
    panel.getByRole("heading", { name: "原问题", exact: true }),
  ).toBeVisible();
  await expect(
    panel.getByText("Controlled task", { exact: false }).first(),
  ).toBeVisible();
  expect(await panel.getByRole("textbox").count()).toBe(0);
  for (const name of [
    "提交此澄清回答",
    "创建材料修订 PR",
    "分析并给建议",
    "执行建议",
  ])
    await expect(panel.getByRole("button", { name, exact: true })).toHaveCount(
      0,
    );
  if (mode === "ready") {
    await expect(panel.getByLabel("提示词建议正文")).toHaveText(
      e.STEERLINE_ADVISORY_PROMPT!,
    );
    await panel
      .getByRole("button", { name: "复制提示词", exact: true })
      .click();
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(e.STEERLINE_ADVISORY_PROMPT);
    expect(commands).toEqual([]);
    await page.reload();
    await expect(page.getByLabel("提示词建议正文")).toHaveText(
      e.STEERLINE_ADVISORY_PROMPT!,
    );
  } else {
    await page.route("**/commands", async (route) => {
      const request = route.request();
      if (
        request.method() === "POST" &&
        request.postDataJSON()?.action === "main_agent_cancel"
      ) {
        const displayed = request.postDataJSON().expected_version;
        expect(displayed).toBeGreaterThan(0);
        const detailURL = request
          .url()
          .replace(/\/commands$/, "/main-agent/" + id);
        // Hold the real immutable POST until ordinary native progress makes
        // its displayed revision stale. GET remains an observation only.
        await expect
          .poll(async () => {
            const response = await page.request.get(detailURL);
            expect(response.ok()).toBe(true);
            const detail = await response.json();
            expect(detail.id).toBe(id);
            expect(detail.candidate_id).toBe(candidate);
            return detail.revision;
          })
          .toBeGreaterThan(displayed);
        cancelAfterProgress = true;
      }
      await route.continue();
    });
    await panel
      .getByRole("button", { name: "取消本次生成", exact: true })
      .click();
    await expect(
      panel.getByText("生成状态：取消并结算中", { exact: true }),
    ).toBeVisible();
    expect(commands).toEqual(["main_agent_cancel"]);
    await expect(
      panel.getByRole("button", { name: "取消本次生成", exact: true }),
    ).toHaveCount(0);
  }
  await page.screenshot({
    path: join(output, "advisory-" + mode + ".png"),
    fullPage: true,
  });
  writeFileSync(
    join(output, "proof.json"),
    JSON.stringify(
      {
        passed: true,
        mode,
        navigation: true,
        context: true,
        copy: mode === "ready",
        cancel: mode === "cancel",
        cancel_after_progress: cancelAfterProgress,
        commands,
      },
      null,
      2,
    ),
  );
});
