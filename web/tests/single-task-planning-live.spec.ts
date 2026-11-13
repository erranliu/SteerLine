import { expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import type {
  RoadmapRecord,
  RoadmapView,
} from "../src/features/roadmaps/roadmaps";
import type { GraphPage } from "../src/shared/api/types";

test("single-task planning uses native conversation and admits only after the real document PR merges", async ({
  page,
}, info) => {
  const root = resolve(process.env.STEERLINE_ROADMAP_BROWSER_ROOT || ".");
  expect(root.toLowerCase()).toMatch(/^c:\\steerlinedebug\\/);
  const manifest = JSON.parse(
    readFileSync(join(root, "manifest.json"), "utf8"),
  );
  expect(manifest.repository).toBe("erranliu/SteerlineTest");
  expect(manifest.base_branch).toMatch(/^codex\/roadmap-e2e\/[\w-]+\/base$/);
  const facts: {
    planning_id?: string;
    document_id?: string;
    pull_request?: string;
    task_id?: string;
    stages: string[];
  } = { stages: [] };
  function stage(value: string) {
    facts.stages.push(value);
    writeFileSync(
      join(root, "evidence", "browser-facts.json"),
      JSON.stringify(facts, null, 2),
    );
    console.log(value);
  }
  function native(executable: string, args: string[]) {
    return execFileSync(executable, args, {
      encoding: "utf8",
      timeout: 45_000,
      stdio: "pipe",
    }).trim();
  }
  let opening = "";
  await expect
    .poll(
      () => {
        try {
          opening = execFileSync(
            join(root, "bin", "steerline-web.exe"),
            ["open", "-print-url"],
            {
              env: { ...process.env, STEERLINE_HOME: join(root, "home") },
              encoding: "utf8",
              timeout: 30_000,
              stdio: "pipe",
            },
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
    await expect(combo.locator("option:not([disabled])")).toHaveCount(1);
    await combo.selectOption({ index: 1 });
  }
  await expect(page.getByText("服务在线", { exact: true })).toBeVisible({
    timeout: 120_000,
  });
  const project = page.url().split("/projects/")[1].split("/")[0];
  const prefix = `http://${manifest.web_address}/api/v2/application/projects/${project}`;
  async function get<T>(path: string): Promise<T> {
    const response = await page.request.get(prefix + path);
    expect(response.ok(), `${path}: ${response.status()}`).toBe(true);
    return response.json();
  }
  async function command(action: string, click: () => Promise<void>) {
    const sent = page.waitForRequest(
      (request) =>
        request.url().endsWith("/commands") &&
        request.method() === "POST" &&
        request.postDataJSON().action === action,
      { timeout: 120_000 },
    );
    await click();
    const request = await sent;
    const id = request.postDataJSON().request_id as string;
    if (action === "roadmap_planning_create") facts.planning_id = id;
    if (action === "roadmap_task_generate") facts.document_id = id;
    if (
      action === "roadmap_planning_create" ||
      action === "roadmap_task_generate"
    )
      stage(`Recorded original ${action} request`);
    const response = await request.response();
    if (response?.ok()) return response.json();
    let receipt;
    await expect
      .poll(
        async () => {
          const observed = await page.request.get(`${prefix}/commands/${id}`);
          if (!observed.ok()) return observed.status();
          receipt = await observed.json();
          return receipt.status;
        },
        { timeout: 120_000 },
      )
      .toMatch(/^(applied|rejected)$/);
    return receipt;
  }
  const graph = () => get<GraphPage>("/graph");
  expect((await graph()).nodes.map((node) => node.task_id)).toEqual(["0"]);
  await page.getByRole("link", { name: "任务规划", exact: true }).click();
  await page.getByRole("button", { name: "新建规划" }).click();
  await page.getByLabel("规划类型").selectOption("task");
  await page.getByLabel("名称", { exact: true }).fill("新增整数加法");
  await page
    .getByLabel("规划目标")
    .fill(
      "在绑定路径写一张单任务卡。目标为在已有 logic 包规划纯函数 Sum(a,b int) int。范围为加法和 Go 单元测试，验收为 Sum(2,3)=5。约束为仅整数。直接用工具写任务卡，保持现有代码。",
    );
  const created = await command("roadmap_planning_create", () =>
    page.getByRole("button", { name: "开始规划" }).click(),
  );
  expect(created.status, created.diagnostic).toBe("applied");
  facts.planning_id = created.roadmap.id;
  stage("Single-task planning intent recorded");
  await expect(page).toHaveURL(
    new RegExp(`/roadmap-planning\\?task=${facts.planning_id}$`),
  );
  const stable = page.url();
  const card = page.getByRole("form", { name: "任务卡" });
  await expect
    .poll(
      () =>
        card
          .getByRole("textbox", { name: "验收条件", exact: true })
          .inputValue({ timeout: 8_000 }),
      { timeout: 360_000 },
    )
    .toContain("Sum");
  await page
    .getByLabel("消息", { exact: true })
    .fill(
      "在原任务卡的 constraints 中补充确定性约束“保持现有 Go 模块路径”，保留其它字段，然后等待。",
    );
  const replied = await command("roadmap_planning_message", () =>
    page.getByRole("button", { name: "发送", exact: true }).click(),
  );
  expect(replied.status, replied.diagnostic).toBe("applied");
  await expect
    .poll(
      () =>
        card
          .getByRole("textbox", { name: "约束", exact: true })
          .inputValue({ timeout: 8_000 }),
      { timeout: 360_000 },
    )
    .toContain("保持现有 Go 模块路径");
  stage("Original native conversation wrote and then revised the task card");

  await card
    .getByRole("checkbox", { name: "#0 Arithmetic foundation" })
    .check();
  const queued = await command("roadmap_task_generate", () =>
    page.getByRole("button", { name: "确认并生成 Spec / Plan" }).click(),
  );
  expect(queued.status, queued.diagnostic).toBe("applied");
  facts.document_id = queued.roadmap.id;
  stage("Confirmed the task card and the selected formal predecessor");
  const record = () => get<RoadmapRecord>(`/roadmaps/${facts.document_id}`);
  async function waitRecord(state: string) {
    let current: RoadmapRecord;
    await expect
      .poll(
        async () => {
          current = await record();
          if (
            [
              "documents-blocked",
              "failed",
              "rejected",
              "invalid",
              "cancelled",
            ].includes(current.state)
          ) {
            throw new Error(
              JSON.stringify({
                state: current.state,
                diagnostic: current.diagnostic,
                questions: current.document_questions,
              }),
            );
          }
          return current.state;
        },
        { timeout: 360_000 },
      )
      .toBe(state);
    return current!;
  }
  await page.reload();
  let documents = await waitRecord("documents-review");
  expect(documents.code_commit).toBe(manifest.base_commit);
  expect(documents.generations).toHaveLength(1);
  expect(documents.generations?.[0].run_id).toBeTruthy();
  expect(documents.analysis?.tasks[0].proposed_dependencies.formal).toEqual([
    "0",
  ]);
  expect(
    documents.investigation?.some((entry) => entry.path === "logic.go"),
  ).toBe(true);
  expect(documents.documents).toHaveLength(1);
  expect(documents.publication_preview?.files).toHaveLength(4);
  expect(documents.publication_preview?.branch).toBe(manifest.base_branch);
  const reviewed = documents
    .publication_preview!.files.map((file) => file.path)
    .sort();
  for (const name of ["Spec", "Plan", "验收材料"])
    await page.getByRole("button", { name, exact: true }).click();
  await page.screenshot({
    path: info.outputPath("document-review.png"),
    fullPage: true,
  });
  expect((await graph()).nodes).toHaveLength(1);
  stage("Reviewed one native generation result and the four proposed files");

  const published = await command("roadmap_publish", () =>
    page.getByRole("button", { name: "确认发布文档 PR" }).click(),
  );
  expect(published.status, published.diagnostic).toBe("applied");
  documents = await waitRecord("awaiting-merge");
  const pr = documents.publication!.url;
  facts.pull_request = pr;
  stage("Real document PR created; admission remains unavailable");
  expect(pr).toMatch(
    /^https:\/\/github\.com\/erranliu\/SteerlineTest\/pull\/\d+$/,
  );
  await expect(
    page.getByRole("button", { name: "加入 DAG", exact: true }),
  ).toHaveCount(0);
  const number = pr.split("/").at(-1)!;
  const endpoint = `repos/${manifest.repository}/pulls/${number}`;
  const original = JSON.parse(
    native("gh", [
      "api",
      endpoint,
      "--jq",
      "{head:.head.sha,base:.base.ref,merged:.merged}",
    ]),
  );
  expect(original.base).toBe(manifest.base_branch);
  expect(original.merged).toBe(false);
  const files = JSON.parse(
    native("gh", ["api", `${endpoint}/files`, "--jq", "map(.filename)"]),
  );
  expect(files.sort()).toEqual(reviewed);
  native("gh", [
    "pr",
    "merge",
    number,
    "--repo",
    manifest.repository,
    "--merge",
    "--match-head-commit",
    original.head,
  ]);
  await command("roadmap_observe", () =>
    page.getByRole("button", { name: "查询原操作结果", exact: true }).click(),
  );
  documents = await waitRecord("awaiting-admission");
  expect(documents.publication?.merged_commit).toMatch(/^[0-9a-f]{40}$/);
  expect((await graph()).nodes).toHaveLength(1);
  stage("Verified the actual merged PR without allocating a task number");
  const admitted = await command("roadmap_admit", () =>
    page.getByRole("button", { name: "加入 DAG", exact: true }).click(),
  );
  expect(admitted.status, admitted.diagnostic).toBe("applied");
  documents = await waitRecord("admitted");
  facts.task_id = documents.formal_mapping.T1;
  const final = await graph();
  expect(final.nodes).toHaveLength(2);
  expect(final.edges).toEqual([{ predecessor: "0", successor: facts.task_id }]);
  expect((await get<RoadmapView>("/roadmaps")).sources).toHaveLength(0);
  await expect(page).toHaveURL(stable);
  await expect(
    page.getByRole("link", { name: "查看正式任务 DAG" }),
  ).toHaveAttribute("href", `/projects/${project}/dag?task=${facts.task_id}`);
  await page.reload();
  await expect(
    page.getByRole("link", { name: "查看正式任务 DAG" }),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("admitted.png"),
    fullPage: true,
  });
  stage(
    "Admitted exactly one formal task with its selected edge on the original planning detail",
  );
});
