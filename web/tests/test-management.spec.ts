import { test, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, resolve, join } from "node:path";

test("testing.management-local-codex: original API results appear in executions", async ({ page }, testInfo) => {
  const config = process.env.STEERLINE_WEB_CONFIG;
  if (!config) throw new Error("fresh debug configuration required");
  const root = resolve(dirname(config), "../..");
  const manifest = JSON.parse(readFileSync(join(root, "manifest.json"), "utf8"));
  const scheme = JSON.parse(readFileSync(join(root, "scheme.json"), "utf8"));
  if (!root.toLowerCase().startsWith("c:\\steerlinedebug\\") || manifest.status !== "ready" || scheme.scheme !== "live") throw new Error("fresh live SteerlineTest package required");
  const opening = execFileSync(join(root,"bin","steerline-web.exe"), ["open", "-print-url"], { env: {...process.env,STEERLINE_HOME:join(root,"home")}, encoding:"utf8",timeout:30000 }).trim();
  await page.goto(opening);
  await expect(page.getByRole("combobox", {name:"当前项目"})).toBeVisible();
  const project = process.env.STEERLINE_TM_PROJECT!;
  await page.goto(new URL(`/projects/${project}/runs?task=${process.env.STEERLINE_TM_TASK}&run=${process.env.STEERLINE_TM_CANDIDATE}`,page.url()).href);
  const section = page.getByRole("region",{name:"计划测试"});
  const failed = process.env.STEERLINE_TM_PHASE === "fail";
  await expect(section).toContainText(failed ? "共 1 项 · 通过 0 · 失败 1 · 待验证 0" : "共 1 项 · 通过 1 · 失败 0 · 待验证 0");
  await section.getByText(`Fixture marker · L1 / isolated · ${failed ? "失败" : "通过"}`, {exact:true}).click();
  await expect(section).toContainText("fixture.marker");
  await expect(section).toContainText("证据引用：");
  await expect(section).toContainText("SHA256");
  await expect(section).toContainText(failed ? "历史结果（2）" : "历史结果（1）");
  await page.screenshot({path:testInfo.outputPath("execution-tests.png"),fullPage:true});
});
