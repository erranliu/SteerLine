import { test, expect } from "@playwright/test";
import { execFileSync, spawnSync } from "node:child_process";
import { readFileSync, writeFileSync, unlinkSync } from "node:fs";
import { dirname, resolve, join } from "node:path";
import { packageCommand } from "./helpers/package";

test("testing.records: actual failure and pass exported to Git, read-only history and deletion", async ({
  page,
}, info) => {
  test.setTimeout(240000);
  const config = process.env.STEERLINE_WEB_CONFIG,
    writer = process.env.STEERLINE_TEST_RECORD_WRITER;
  if (!config || !writer)
    throw new Error(
      "fresh controlled debug configuration and record writer required",
    );
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
  const repo = join(root, "projects", "SteerlineTest"),
    remote = join(root, "remotes", "erranliu", "SteerlineTest.git");
  const configuration = JSON.parse(readFileSync(config, "utf8"));
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
  const refresh = async () => {
    git("push", remote, "HEAD:refs/heads/" + configuration.branch);
    const response = await page.evaluate(
      async ({ prefix, project }) => {
        const auth = await (await fetch("/auth/status")).json(),
          identity = await (await fetch(prefix + "/identity")).json();
        const r = await fetch(prefix + "/commands", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": auth.csrf_token,
          },
          body: JSON.stringify({
            request_id: crypto.randomUUID().replaceAll("-", ""),
            project_id: identity.project_id,
            instance_id: identity.instance_id,
            action: "maintenance_check",
            target_id: project,
            expected_version: 1,
            payload: {},
          }),
        });
        if (!r.ok) throw new Error(await r.text());
        return r.json();
      },
      { prefix, project },
    );
    expect(response.status).toBe("applied");
  };
  const input = join(repo, "fixture-case.txt");
  const execute = (name: string) => {
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
      ],
      { cwd: repo, env, encoding: "utf8", windowsHide: true, timeout: 30000 },
    );
    expect(result.error, result.stderr).toBeUndefined();
    const report = JSON.parse(
      readFileSync(join(out, "managed-fixture.marker.json"), "utf8"),
    );
    expect(report.before_clean).toBe(true);
    expect(report.after_clean).toBe(true);
    return { out, report, exit: result.status };
  };
  const exportRecord = (out: string) =>
    execFileSync(writer, ["record", "-report-dir", out], {
      cwd: repo,
      env,
      encoding: "utf8",
      windowsHide: true,
    }).trim();
  writeFileSync(input, "actual wrong input");
  git("add", "fixture-case.txt");
  git("commit", "-qm", "Fixture actual failing input");
  const failed = execute("first-failed");
  expect(failed.exit).toBe(1);
  expect(failed.report.results[0].status).toBe("failed");
  const failedPath = exportRecord(failed.out);
  expect(
    (await get("/tests/records?test_id=fixture.marker")).summary.total,
  ).toBe(0);
  git("add", "SteerlineFlow/testing/records");
  git("commit", "-qm", "Record actual failure");
  await refresh();
  expect(
    (await get("/tests/records?test_id=fixture.marker")).summary.total,
  ).toBe(1);
  writeFileSync(input, "fixture-ok");
  git("add", "fixture-case.txt");
  git("commit", "-qm", "Fix actual fixture input");
  const passed = execute("second-passed");
  expect(passed.exit).toBe(0);
  expect(passed.report.before_commit).not.toBe(failed.report.before_commit);
  const passedPath = exportRecord(passed.out),
    original = readFileSync(passedPath, "utf8");
  exportRecord(passed.out);
  expect(readFileSync(passedPath, "utf8")).toBe(original);
  expect(
    (await get("/tests/records?test_id=fixture.marker")).summary.total,
  ).toBe(1);
  git("add", "SteerlineFlow/testing/records");
  git("commit", "-qm", "Record actual pass");
  await refresh();
  const records = await get("/tests/records?test_id=fixture.marker");
  expect(records.summary.total).toBe(2);
  expect(records.summary.failed).toBe(1);
  const failure = records.records.find((r: any) => r.status === "failed");
  expect(failure.commit).toBe(failed.report.before_commit);
  const detail = await get("/tests/records/" + failure.id);
  expect(detail.note).toContain("Marker assertion failed");
  expect(detail.path).toContain(failed.report.before_commit + ".json");
  await page.goto(
    new URL(`/projects/${project}/tests?test=fixture.marker`, page.url()).href,
  );
  await expect(
    page.getByRole("heading", { name: "测试管理", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: /Fixture marker/ })).toBeVisible();
  await expect(page.locator(".test-record-card")).toHaveCount(2);
  await page.getByLabel("原始结果").selectOption("failed");
  await expect(page.locator(".test-record-card")).toHaveCount(1);
  await page.locator(".test-record-card").first().getByRole("button").click();
  await expect(page.getByRole("region", { name: "记录详情" })).toContainText(
    failed.report.before_commit,
  );
  await expect(
    page.getByRole("button", {
      name: /执行测试|运行测试|重跑测试|修改结果|查看证据摘要/,
    }),
  ).toHaveCount(0);
  await page.screenshot({
    path: info.outputPath("test-records.png"),
    fullPage: true,
  });
  unlinkSync(failedPath);
  git("add", "SteerlineFlow/testing/records");
  git("commit", "-qm", "Prune old test history");
  await refresh();
  const pruned = await get("/tests/records?test_id=fixture.marker");
  expect(pruned.summary.total).toBe(1);
  expect(pruned.records[0].status).toBe("passed");
  const removed = await page.request.get(
    new URL(prefix + "/tests/records/" + failure.id, page.url()).href,
  );
  expect(removed.status()).toBe(404);
  writeFileSync(
    join(root, "evidence", "test-records", "records-proof.json"),
    JSON.stringify(
      {
        failed_commit: failed.report.before_commit,
        passed_commit: passed.report.before_commit,
        failed_exit: failed.exit,
        passed_exit: passed.exit,
        record_count: records.summary.total,
        after_prune: pruned.summary.total,
        uncommitted_hidden: true,
        repeat_export_unchanged: true,
        project,
        browser: true,
      },
      null,
      2,
    ),
  );
});
