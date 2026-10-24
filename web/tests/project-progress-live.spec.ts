import { test, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, writeFileSync, renameSync } from "node:fs";
import { dirname, resolve, join } from "node:path";

test("remote Runner progress updates one live SteerLine page", async ({ page }, testInfo) => {
  const web = process.env.STEERLINE_PROGRESS_WEB!;
  const home = process.env.STEERLINE_PROGRESS_HOME!;
  const targetFile = process.env.STEERLINE_PROGRESS_TARGET!;
  const output = process.env.STEERLINE_WEB_EVIDENCE!;
  if (!web || !home || !targetFile || !output) throw new Error("explicit isolated progress browser inputs required");
  const root = resolve(dirname(web), "..");
  if (!root.toLowerCase().startsWith("c:\\steerlinedebug\\") || !resolve(home).toLowerCase().startsWith(root.toLowerCase() + "\\")) {
    throw new Error("browser must use the test-owned home and executable");
  }
  const save = (name: string, value: unknown) => {
    const path = join(output, name);
    writeFileSync(path + ".tmp", JSON.stringify(value, null, 2));
    renameSync(path + ".tmp", path);
  };
  const opening = execFileSync(web, ["open", "-print-url"], {
    env: { ...process.env, STEERLINE_HOME: home }, encoding: "utf8", timeout: 30000,
  }).trim();
  await page.goto(opening);
  await expect(page.getByRole("combobox", { name: "当前项目" })).toBeVisible();
  save("ready.json", { ready: true, at: new Date().toISOString() });
  await expect.poll(() => existsSync(targetFile), { timeout: 300000 }).toBe(true);
  const target = JSON.parse(readFileSync(targetFile, "utf8"));
  const path = `/projects/${target.project}/runs?task=${target.task}&run=${target.candidate}`;
  await page.goto(new URL(path, page.url()).href);
  const section = page.getByRole("region", { name: "计划测试" });
  await expect(section).toBeVisible();
  let documentRequests = 0;
  page.on("request", request => { if (request.resourceType() === "document") documentRequests++; });
  const expected = [
    [false, false, false], [true, false, false], [true, true, false],
    [true, false, false], [true, true, false], [true, true, true],
  ];
  const keys = ["demo.progress.roundtrip", "demo.progress.retry", "demo.progress.cleanup"];
  const states: Array<{ at: string; state: boolean[]; version: number; text: string }> = [];
  let matched = 0;
  let lastVersion = 0;
  const deadline = Date.now() + 600000;
  while (matched < expected.length && Date.now() < deadline) {
    // Read version and rows in one DOM turn so a refetch cannot mix versions.
    const { text, rows } = await section.evaluate(element => ({
      text: (element as HTMLElement).innerText,
      rows: [...element.querySelectorAll(".test-plan-row")].map(row => ({
        id: row.querySelector("p.muted")?.textContent?.trim(),
        summary: row.querySelector("summary")?.textContent?.trim() || "",
      })),
    }));
    const version = Number(text.match(/进度版本\s+(\d+)/)?.[1] || 0);
    if (version && rows.length === 3 && text.includes("已通过") && !text.includes("测试查询失败")) {
      expect(rows.map(row => row.id).sort()).toEqual([...keys].sort());
      const state = keys.map(key => rows.find(row => row.id === key)!.summary.endsWith("· 通过"));
      if (version < lastVersion) throw new Error("rendered progress version regressed");
      lastVersion = version;
      const passed = state.filter(Boolean).length;
      expect(text).toContain(`共 3 项 · 已通过 ${passed} · 未完成 ${3 - passed}`);
      if (state.every((value, index) => value === expected[matched][index])) {
        states.push({ at: new Date().toISOString(), state, version, text });
        matched++;
        save("observations.json", { passed: false, target, states, document_requests_during_observation: documentRequests });
        await section.scrollIntoViewIfNeeded();
        await page.screenshot({ path: join(output, `step-${matched}-version-${version}.png`), fullPage: true });
      }
    }
    if (matched < expected.length) await page.waitForTimeout(100);
  }
  expect(matched, "the same page must observe all six real progress states").toBe(6);
  expect(documentRequests, "progress must update without page reload or navigation").toBe(0);
  save("observations.json", { passed: true, target, states, document_requests_during_observation: documentRequests });
  await testInfo.attach("rendered-progress", { path: join(output, "observations.json"), contentType: "application/json" });
});
