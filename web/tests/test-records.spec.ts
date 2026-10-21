import { test, expect } from "@playwright/test";
import { execFileSync, spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve, join } from "node:path";
import { packageCommand } from "./helpers/package";

test("testing.records: actual script failure, committed fix, retest and read-only history", async ({
  page,
}, info) => {
  test.setTimeout(240000);
  const config = process.env.STEERLINE_WEB_CONFIG;
  if (!config) throw new Error("fresh controlled debug configuration required");
  const root = resolve(dirname(config), "../..");
  const manifest = JSON.parse(
    readFileSync(join(root, "manifest.json"), "utf8"),
  );
  const scheme = JSON.parse(readFileSync(join(root, "scheme.json"), "utf8"));
  if (
    !root.toLowerCase().startsWith("c:\\steerlinedebug\\") ||
    manifest.status !== "ready" ||
    scheme.scheme !== "simulate"
  )
    throw new Error("fresh isolated simulate package required");
  const repo = join(root, "projects", "SteerlineTest");
  const env = { ...process.env, STEERLINE_HOME: join(root, "home") };
  const command = packageCommand(env);
  const git = (...args: string[]) =>
    execFileSync("git", ["-C", repo, ...args], {
      encoding: "utf8",
      windowsHide: true,
    }).trim();
  await page.goto(
    command(join(root, "bin", "steerline-web.exe"), ["open", "-print-url"]),
  );
  const projects = await (
    await page.request.get(
      new URL("/api/v2/application/projects", page.url()).href,
    )
  ).json();
  const project = projects.projects.find(
    (p: any) => p.config_id === "SteerlineTest.json",
  )?.project_id;
  expect(project).toBeTruthy();
  const prefix = "/api/v2/application/projects/" + project;
  const get = async (path: string) => {
    const response = await page.request.get(
      new URL(prefix + path, page.url()).href,
    );
    expect(response.ok(), await response.text()).toBeTruthy();
    return response.json();
  };
  const input = join(repo, "fixture-case.txt");
  const execute = (name: string, parent = "") => {
    const out = join(root, "evidence", "test-records", name);
    const result = spawnSync(
      "powershell.exe",
      [
        "-NoProfile",
        "-ExecutionPolicy",
        "Bypass",
        "-File",
        join(repo, "test-marker.ps1"),
        "-Out",
        out,
        "-InputPath",
        input,
        ...(parent ? ["-RerunOf", parent] : []),
      ],
      { cwd: repo, env, encoding: "utf8", windowsHide: true, timeout: 30000 },
    );
    expect(result.error, result.stderr).toBeUndefined();
    const report = JSON.parse(
      readFileSync(join(out, "managed-fixture.marker.json"), "utf8"),
    );
    return { out, report, exit: result.status };
  };
  const register = (out: string) =>
    command(join(root, "bin", "steerline.exe"), [
      "tests",
      "register",
      "-project",
      project,
      "-report-dir",
      out,
    ]);
  writeFileSync(input, "actual wrong input");
  git("add", "fixture-case.txt");
  git("commit", "-qm", "Fixture actual failing input");
  const failed = execute("first-failed");
  expect(failed.exit).toBe(1);
  expect(failed.report.results[0].status).toBe("failed");
  expect(failed.report.before_clean).toBe(true);
  register(failed.out);
  writeFileSync(input, "fixture-ok");
  git("add", "fixture-case.txt");
  git("commit", "-qm", "Fix actual fixture input");
  const passed = execute("second-passed", failed.report.id);
  expect(passed.exit).toBe(0);
  expect(passed.report.before_commit).not.toBe(failed.report.before_commit);
  expect(passed.report.before_clean).toBe(true);
  register(passed.out);
  register(failed.out); // same report, new request, no duplicate or test execution
  const records = await get("/tests/records?test_id=fixture.marker");
  expect(records.summary.total).toBe(2);
  expect(records.summary.pending).toBe(0);
  const original = records.records.find((r: any) => r.id === failed.report.id);
  expect(original).toMatchObject({
    status: "failed",
    retest: "passed",
    retest_id: passed.report.id,
    retest_source_fixed: true,
    source: { kind: "script" },
  });
  const detail = await get("/tests/records/" + failed.report.id);
  expect(detail.chain.map((r: any) => r.id)).toEqual([
    failed.report.id,
    passed.report.id,
  ]);
  const summary = await get("/tests/records/" + failed.report.id + "/evidence");
  expect(summary.status).toBe("available");
  expect(JSON.parse(summary.text).actual).toBe("actual wrong input");
  await page.goto(
    new URL(`/projects/${project}/tests?test=fixture.marker`, page.url()).href,
  );
  await expect(
    page.getByRole("heading", { name: "测试管理", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /Fixture marker/ }),
  ).toBeVisible();
  await expect(page.locator(".test-record-card")).toHaveCount(2);
  await expect(
    page.locator(".test-record-card").filter({ hasText: failed.report.id }),
  ).toContainText("失败 · 复验通过");
  await page.getByLabel("原始结果").selectOption("failed");
  await expect(page.locator(".test-record-card")).toHaveCount(1);
  await page.locator(".test-record-card").first().getByRole("button").click();
  await expect(page.getByRole("region", { name: "记录详情" })).toContainText(
    passed.report.id,
  );
  await page.getByRole("button", { name: "查看证据摘要" }).click();
  await expect(page.getByRole("region", { name: "记录详情" })).toContainText(
    "actual wrong input",
  );
  await expect(
    page.getByRole("button", { name: /执行测试|运行测试|重跑测试|修改结果/ }),
  ).toHaveCount(0);
  await page.screenshot({
    path: info.outputPath("test-records.png"),
    fullPage: true,
  });
  writeFileSync(
    join(root, "evidence", "test-records", "records-proof.json"),
    JSON.stringify(
      {
        failed: failed.report.id,
        passed: passed.report.id,
        failed_exit: failed.exit,
        passed_exit: passed.exit,
        before_commit: failed.report.before_commit,
        fixed_commit: passed.report.before_commit,
        record_count: records.summary.total,
        project,
        independent: true,
        browser: true,
      },
      null,
      2,
    ),
  );
});
