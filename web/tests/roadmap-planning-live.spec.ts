import { expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";

test.use({ actionTimeout: 15_000 });

test("Roadmap planning uses the original Codex conversation and hands off the merged draft", async ({
  page,
}) => {
  test.setTimeout(540_000);
  const root = resolve(process.env.STEERLINE_ROADMAP_BROWSER_ROOT || ".");
  if (!root.toLowerCase().startsWith("c:\\steerlinedebug\\"))
    throw new Error("Fresh isolated live test root required");
  const manifest = JSON.parse(
    readFileSync(join(root, "manifest.json"), "utf8"),
  );
  expect(manifest.repository).toBe("erranliu/SteerlineTest");
  expect(manifest.base_branch).toMatch(/^codex\/roadmap-e2e\/[\w-]+\/base$/);
  const facts: { planning_id?: string; stages: string[] } = { stages: [] };
  const stage = (text: string) => {
    facts.stages.push(text);
    writeFileSync(
      join(root, "evidence", "browser-facts.json"),
      JSON.stringify(facts, null, 2),
    );
    console.log(text);
  };
  page.on("request", (request) => {
    if (request.method() === "POST" && request.url().endsWith("/commands")) {
      const envelope = request.postDataJSON();
      if (envelope.action === "roadmap_planning_create") {
        facts.planning_id = envelope.request_id;
        stage("Original planning intent");
      }
    }
  });
  const env = { ...process.env, STEERLINE_HOME: join(root, "home") };
  let opening = "";
  await expect
    .poll(
      () => {
        try {
          opening = execFileSync(
            join(root, "bin", "steerline-web.exe"),
            ["open", "-print-url"],
            { env, encoding: "utf8", timeout: 30000, stdio: "pipe" },
          ).trim();
          return !!opening;
        } catch {
          return false;
        }
      },
      { timeout: 120_000 },
    )
    .toBe(true);
  await page.goto(opening);
  if (!page.url().includes("/projects/")) {
    const combo = page.getByRole("combobox", { name: "当前项目" });
    await expect
      .poll(() => combo.locator("option:not([disabled])").count())
      .toBe(1);
    await combo.selectOption({ index: 1 });
  }
  await expect(page.getByText("服务在线", { exact: true })).toBeVisible({
    timeout: 120_000,
  });
  const project = page.url().split("/projects/")[1].split("/")[0];
  const prefix =
    "http://" +
    manifest.web_address +
    "/api/v2/application/projects/" +
    project;
  const task = async () => {
    if (!facts.planning_id) return {};
    const response = await page.request.get(
      prefix + "/roadmap-planning/" + facts.planning_id,
    );
    if (response.status() === 404) return {};
    expect(response.ok()).toBe(true);
    return response.json();
  };

  await page.getByRole("link", { name: "Roadmap 规划", exact: true }).click();
  await page.getByRole("button", { name: "新建规划" }).click();
  await page.getByLabel("名称", { exact: true }).fill("原生多轮规划验收");
  await page.getByLabel("草稿文件名").fill("live-planning.md");
  await page
    .getByLabel("规划目标")
    .fill(
      "直接用工具在绑定路径新建 Roadmap 草稿，标题为 Planning acceptance，记录本轮已确认的产品取舍“使用整数”。只写草稿，并把根目录 planning-conflict.txt 的整行改为 planning side，同时新建 planning-excluded.txt 写入 local only。完成本轮后等待。",
    );
  const creation = page.waitForResponse(
    (response) =>
      response.url().endsWith("/commands") &&
      response.request().method() === "POST" &&
      response.request().postDataJSON().action === "roadmap_planning_create",
  );
  await page.getByRole("button", { name: "开始规划" }).click();
  const created = await (await creation).json();
  expect(created.status, created.diagnostic).toBe("applied");
  await expect
    .poll(async () => (await task()).body, { timeout: 180_000 })
    .toContain("Planning acceptance");
  await expect(
    page.getByRole("button", { name: "发送", exact: true }),
  ).toBeDisabled();
  await page
    .getByLabel("消息", { exact: true })
    .fill("把上一轮确认的具体取舍再写到草稿的“第二轮”小节。完成后等待。");
  await page.getByRole("button", { name: "发送", exact: true }).click();
  await expect
    .poll(async () => (await task()).body, { timeout: 160_000 })
    .toMatch(/第二轮[\s\S]*使用整数/);
  stage("Original native session continued and updated the draft");
  const stable = page.url();

  const repo = join(root, "project");
  const upstream = join(root, "upstream");
  const git = (...args: string[]) =>
    execFileSync("git", args, {
      encoding: "utf8",
      stdio: "pipe",
      timeout: 30000,
    }).trim();
  git(
    "-C",
    repo,
    "worktree",
    "add",
    "--detach",
    upstream,
    manifest.base_commit,
  );
  writeFileSync(join(upstream, "planning-conflict.txt"), "upstream side\n");
  git("-C", upstream, "add", "planning-conflict.txt");
  git("-C", upstream, "commit", "-m", "Concurrent isolated planning change");
  git(
    "-C",
    upstream,
    "push",
    "origin",
    "HEAD:refs/heads/" + manifest.base_branch,
  );
  await page.getByRole("button", { name: "Rebase", exact: true }).click();
  await expect
    .poll(async () => (await task()).conflict?.files, { timeout: 60_000 })
    .toEqual(["planning-conflict.txt"]);
  await page
    .getByLabel("补充冲突取舍")
    .fill("保留规划侧 planning side，完成原 Rebase。");
  await page.getByRole("button", { name: "让 SteerAgent 解决冲突" }).click();
  await expect
    .poll(
      async () => {
        const current = await task();
        return !current.conflict && current.can_rebase;
      },
      { timeout: 180_000 },
    )
    .toBe(true);
  stage("Original native session resolved the real Git conflict");

  await page.getByRole("button", { name: "审阅文件差异" }).click();
  await expect
    .poll(
      async () => {
        const current = await task();
        return !current.busy && current.can_rebase && !!current.review?.digest;
      },
      { timeout: 60_000 },
    )
    .toBe(true);
  const summary = page
    .locator("summary")
    .filter({ hasText: /^planning-conflict\.txt$/ });
  const extra = page.locator("details").filter({ has: summary });
  await summary.click();
  await extra.getByRole("checkbox", { name: "纳入 PR" }).check();
  await page.getByRole("button", { name: "确认并创建 PR" }).click();
  await expect
    .poll(async () => (await task()).pr, { timeout: 90_000 })
    .toMatch(/\/pull\/\d+$/);
  const pr = (await task()).pr;
  const number = pr.split("/").pop();
  const gh = (...args: string[]) =>
    execFileSync("gh", args, {
      encoding: "utf8",
      stdio: "pipe",
      timeout: 30000,
    }).trim();
  const files = JSON.parse(
    gh("api", "repos/" + manifest.repository + "/pulls/" + number + "/files"),
  );
  expect(
    files.map((entry: { filename: string }) => entry.filename).sort(),
  ).toEqual([
    "SteerlineFlow/Roadmap/drafts/live-planning.md",
    "planning-conflict.txt",
  ]);
  const head = gh(
    "api",
    "repos/" + manifest.repository + "/pulls/" + number,
    "--jq",
    ".head.sha",
  );
  gh(
    "pr",
    "merge",
    number,
    "--repo",
    manifest.repository,
    "--merge",
    "--match-head-commit",
    head,
  );
  await expect
    .poll(async () => (await task()).status, { timeout: 90_000 })
    .toBe("merged_pending_sync");
  stage("Reviewed files merged; handoff waited for the local checkout");
  git("-C", repo, "pull", "--ff-only", "origin", manifest.base_branch);
  await expect(page).toHaveURL(/\/roadmaps\?source=[a-f0-9]{32}$/, {
    timeout: 90_000,
  });
  await page.goto(stable);
  await expect(page).toHaveURL(/\/roadmaps\?source=[a-f0-9]{32}$/, {
    timeout: 45_000,
  });
  await expect(
    page.getByRole("textbox", { name: "Markdown 原文 · 只读" }),
  ).toHaveValue(/Planning acceptance/);
  stage("Stable planning link opened the actual merged source");
});
