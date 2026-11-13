import { expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { join } from "node:path";

test("starts and records the draft PR head through the real verification page", async ({
  page,
}, info) => {
  const {
    STEERLINE_MANUAL_WEB: web,
    STEERLINE_MANUAL_PROJECT: project,
    STEERLINE_MANUAL_CANDIDATE: candidate,
    STEERLINE_MANUAL_HEAD: head,
  } = process.env;
  if (!web || !project || !candidate || !head || !process.env.STEERLINE_HOME)
    throw new Error("isolated verification fixture required");
  const opening = execFileSync(web, ["open", "-print-url"], {
    encoding: "utf8",
    timeout: 30000,
    windowsHide: true,
  }).trim();
  await page.goto(opening);
  const projects = page.getByRole("combobox", { name: "当前项目" });
  await expect(projects).toBeVisible();
  await projects.selectOption(project);
  const link = page.getByRole("link", { name: "人工核验", exact: true });
  await expect(link).toHaveAttribute(
    "href",
    `/projects/${project}/manual-verification`,
  );
  await link.click();
  const row = page.getByRole("button", { name: /PR #.*0\/1/ });
  await row.click();
  await expect(page).toHaveURL(new RegExp(`candidate=${candidate}$`));
  await expect(page.getByText(head, { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "创建 worktree 并启动" }).click();
  const preview = page.getByRole("link", { name: "打开预览" });
  await expect(preview).toBeVisible({ timeout: 120000 });
  const response = await page.request.get(
    (await preview.getAttribute("href"))!,
  );
  expect((await response.text()).trim()).toBe("real model delivery");
  await page.getByLabel("核验备注").fill("Exercise failure history");
  await page.getByRole("button", { name: "记录失败" }).click();
  await expect(
    page.getByRole("heading", { name: /M01 Preview · 失败/ }),
  ).toBeVisible();
  await page.screenshot({
    path: join(info.outputDir, "manual-verification.png"),
    fullPage: true,
  });
});
