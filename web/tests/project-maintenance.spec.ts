import { test, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync, appendFileSync } from "node:fs";
import { dirname, resolve, join } from "node:path";
test("runtime.project-maintenance: review, apply and freeze in real Go/Web", async ({
  page,
}, info) => {
  test.setTimeout(300000);
  page.setDefaultTimeout(20000);
  const config = process.env.STEERLINE_WEB_CONFIG;
  if (!config) throw new Error("fresh debug configuration required");
  const root = resolve(dirname(config), "../..");
  const manifest = JSON.parse(
    readFileSync(join(root, "manifest.json"), "utf8"),
  );
  const scheme = JSON.parse(readFileSync(join(root, "scheme.json"), "utf8"));
  if (
    !root.toLowerCase().startsWith("c:\\steerlinedebug\\") ||
    !manifest.project_maintenance ||
    scheme.scheme !== "simulate"
  )
    throw new Error("fresh controlled maintenance package required");
  const repo = join(root, "projects", "SteerlineTest"),
    remote = join(root, "remotes", "erranliu", "SteerlineTest.git");
  const git = (...args: string[]) =>
    execFileSync("git", ["-C", repo, ...args], {
      encoding: "utf8",
      timeout: 30000,
    }).trim();
  const opening = execFileSync(
    join(root, "bin", "steerline-web.exe"),
    ["open", "-print-url"],
    {
      env: { ...process.env, STEERLINE_HOME: join(root, "home") },
      encoding: "utf8",
      timeout: 30000,
    },
  ).trim();
  await page.goto(opening);
  await expect(page.getByRole("combobox", { name: "当前项目" })).toBeVisible();
  const catalog = await (
    await page.request.get(
      new URL("/api/v2/application/projects", page.url()).href,
    )
  ).json();
  const primary = catalog.projects.find(
    (p: any) => p.config_id === "SteerlineTest.json",
  );
  if (!primary)
    throw new Error("primary fixture absent: " + JSON.stringify(catalog));
  const project = primary.project_id;
  await page.getByRole("combobox", { name: "当前项目" }).selectOption(project);
  const prefix = "/api/v2/application/projects/" + project;
  const get = async (path: string) => {
    const r = await page.request.get(new URL(prefix + path, page.url()).href);
    expect(r.ok(), await r.text()).toBeTruthy();
    return r.json();
  };
  const beforeOther = git(
    "-C",
    join(root, "projects", "SteerlineTestB"),
    "rev-parse",
    "HEAD",
  );
  const section = page.getByRole("region", { name: "项目维护" });
  await section.getByRole("button", { name: /项目维护/ }).click();
  await section.getByRole("button", { name: "检查远端" }).click();
  await expect(section).toContainText("docs/spec/task.plan.md");
  const call = async (
    action: string,
    target: string,
    payload: object,
    version = 1,
    extra: Record<string, number> = {},
  ) =>
    page.evaluate(
      async ({ prefix, action, target, payload, version, extra }) => {
        const auth = await (await fetch("/auth/status")).json();
        const identity = await (await fetch(prefix + "/identity")).json();
        const envelope = {
          request_id: crypto.randomUUID().replaceAll("-", ""),
          project_id: identity.project_id,
          instance_id: identity.instance_id,
          action,
          target_id: target,
          expected_version: Math.max(1, version),
          ...extra,
          payload,
        };
        const r = await fetch(prefix + "/commands", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": auth.csrf_token,
          },
          body: JSON.stringify(envelope),
        });
        if (!r.ok) throw new Error(await r.text());
        return r.json();
      },
      { prefix, action, target, payload, version, extra },
    );
  const plan = "docs/spec/task.plan.md";
  const a = await get("/maintenance?plan=" + encodeURIComponent(plan));
  expect(a.assessment.items[0].status).toBe("needs_review");
  for (const mode of ["initialize", "local", "baseline"]) {
    await section.getByLabel("维护范围").selectOption(mode);
    await expect(section).toContainText("docs/spec/task.plan.md");
  }
  await section.getByRole("button", { name: "选择此 Plan" }).click();
  const file = join(repo, "docs", "spec", "task.tests.json"),
    before = readFileSync(file, "utf8");
  await section.getByRole("button", { name: "生成测试安排" }).click();
  await expect(section.getByLabel("原生成请求")).not.toHaveValue("");
  await expect(
    section.getByRole("button", { name: "应用已审阅内容" }),
  ).toBeEnabled();
  expect(readFileSync(file, "utf8")).toBe(before);
  await page.screenshot({
    path: info.outputPath("review-before-apply.png"),
    fullPage: true,
  });
  const generationID = await section.getByLabel("原生成请求").inputValue();
  const originalGeneration = await get("/tests/generations/" + generationID);
  const wrongVersion = await call(
    "maintenance_apply",
    generationID,
    {
      generation_id: generationID,
      expected_revision: originalGeneration.revision,
      review_digest: originalGeneration.review.review_digest,
      allow_empty: false,
    },
    originalGeneration.revision + 1,
  );
  expect(wrongVersion.status).toBe("rejected");
  expect(readFileSync(file, "utf8")).toBe(before);
  await section.getByRole("button", { name: "应用已审阅内容" }).click();
  await expect(section).toContainText("已应用到本地");
  expect(JSON.parse(readFileSync(file, "utf8")).tests).toEqual([
    "fixture.marker",
  ]);
  git("add", "docs/spec/task.tests.json");
  git(
    "commit",
    "--allow-empty",
    "-qm",
    "Explicitly apply reviewed test arrangement",
  );
  git("push", remote, "HEAD:refs/heads/" + manifest.base_branch);
  await section.getByRole("button", { name: "检查远端" }).click();
  await section.getByRole("button", { name: "已审阅，采用当前安排" }).click();
  await expect(section).toContainText("材料已核对");
  // A source-only remote advance must survive repeated checks and retain approval.
  git("commit", "--allow-empty", "-qm", "Remote source baseline");
  const latest = git("rev-parse", "HEAD");
  git("push", remote, "HEAD:refs/heads/" + manifest.base_branch);
  git("reset", "--hard", "HEAD~1");
  await section.getByRole("button", { name: "检查远端" }).click();
  await section.getByRole("button", { name: "检查远端" }).click();
  expect(
    (await get("/maintenance?plan=" + encodeURIComponent(plan))).local_commit,
  ).not.toBe(latest);
  await expect(section).toContainText("本地与远端基线不同");
  // The test driver, representing normal Git maintenance, pulls; product never does.
  git("fetch", remote, manifest.base_branch);
  git("merge", "--ff-only", "FETCH_HEAD");
  const control = (action: string, task = "") =>
    execFileSync(
      join(root, "bin", "debug-helper.exe"),
      [
        "debug",
        "control",
        "--simulate",
        "-config",
        config,
        "-action",
        action,
        ...(task ? ["-task-id", task, "-profile", "simulate"] : []),
      ],
      { env: process.env, encoding: "utf8", timeout: 60000 },
    );
  control("resume_dispatch");
  control("start_task", "1");
  let candidates: any[] = [];
  await expect
    .poll(async () => {
      const v = await get("/snapshot");
      candidates = v.connection.snapshot.page.runs;
      return candidates.length;
    })
    .toBeGreaterThan(0);
  const first = candidates.find(
    (c) => c.domain_task_id === "1" || c.task_id === "1",
  );
  expect(first).toBeTruthy();
  const firstID = first.run_id;
  const planned = await get("/runs/" + firstID + "/tests");
  expect(planned.total).toBe(1);
  expect(planned.rows[0].id).toBe("fixture.marker");
  expect(planned.commit).toBe(latest);
  // Changed Task content needs explicit review before its first reservation.
  appendFileSync(
    join(repo, "docs", "spec", "task.plan.md"),
    "\nMaterial maintenance acceptance detail.\n",
  );
  git("add", ".");
  git("commit", "-qm", "Change Plan content");
  git("push", remote, "HEAD:refs/heads/" + manifest.base_branch);
  const review = await get("/maintenance/tasks/2");
  expect(review.diffs.some((d: any) => d.path === plan)).toBeTruthy();
  const confirmation = await call(
    "maintenance_confirm_task",
    "2",
    {
      task_id: review.task_id,
      version: review.version,
      contract: review.contract,
      review_digest: review.review_digest,
    },
    review.version,
  );
  expect(confirmation.status).toBe("applied");
  const taskPage = await get("/tasks?limit=100");
  const snapshot = await get("/snapshot");
  const originalTask = taskPage.tasks.find((t: any) => t.task_id === "1");
  const second = await call(
    "start_candidate",
    "1",
    { execution_profile_id: "simulate" },
    originalTask.version,
    {
      expected_graph_revision: taskPage.graph_revision,
      expected_control_revision:
        snapshot.connection.snapshot.page.control.revision,
    },
  );
  expect(["submitted", "accepted", "applied"]).toContain(second.status);
  let secondCandidate: any;
  await expect
    .poll(async () => {
      const v = await get("/snapshot");
      secondCandidate = v.connection.snapshot.page.runs.find(
        (c: any) =>
          c.run_id !== firstID &&
          (c.domain_task_id === "1" || c.task_id === "1"),
      );
      return !!secondCandidate?.execution_id;
    })
    .toBe(true);
  expect((await get("/runs/" + secondCandidate.run_id + "/tests")).commit).toBe(
    latest,
  );
  const after = await get("/runs/" + firstID + "/tests");
  expect(after.commit).toBe(latest);
  expect(
    git("-C", join(root, "projects", "SteerlineTestB"), "rev-parse", "HEAD"),
  ).toBe(beforeOther);
  const identities = await page
    .getByRole("combobox", { name: "当前项目" })
    .locator("option")
    .evaluateAll((os) =>
      os.map((o) => (o as HTMLOptionElement).value).filter(Boolean),
    );
  expect(identities.length).toBe(2);
  const otherProject = catalog.projects.find(
    (p: any) => p.project_id !== project,
  );
  const otherView = await (
    await page.request.get(
      new URL(
        "/api/v2/application/projects/" +
          otherProject.project_id +
          "/maintenance",
        page.url(),
      ).href,
    )
  ).json();
  expect(otherView.generations).toEqual([]);
  await page.goto(
    new URL(`/projects/${project}/runs?task=1&run=${firstID}`, page.url()).href,
  );
  const planSection = page.getByRole("region", { name: "计划测试" });
  await expect(planSection).toContainText(
    "共 1 项 · 通过 0 · 失败 0 · 待验证 1",
  );
  await expect(planSection).toContainText(latest.slice(0, 12));
  await page.screenshot({
    path: info.outputPath("maintenance-complete.png"),
    fullPage: true,
  });
  writeFileSync(
    info.outputPath("facts.json"),
    JSON.stringify(
      {
        project,
        latest,
        firstID,
        planned,
        confirmation,
        second,
        secondCandidate,
        other: beforeOther,
      },
      null,
      2,
    ),
  );
});
