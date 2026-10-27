import { test, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve, join } from "node:path";

async function readable(locator: import("@playwright/test").Locator) {
  const contrast = await locator.evaluate(element => {
    const style = getComputedStyle(element);
    const luminance = (color: string) => {
      const rgb = color.match(/[\d.]+/g)!.slice(0, 3).map(Number).map(v => {
        v /= 255; return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
      });
      return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722;
    };
    const text = luminance(style.color), background = luminance(style.backgroundColor);
    return (Math.max(text, background) + 0.05) / (Math.min(text, background) + 0.05);
  });
  expect(contrast, "Roadmap content must have readable contrast").toBeGreaterThanOrEqual(4.5);
}

test("roadmap.live: browser, native generate, external PR and atomic DAG admission", async ({ page }, info) => {
  const root = resolve(process.env.STEERLINE_ROADMAP_BROWSER_ROOT || ".");
  if (!root.toLowerCase().startsWith("c:\\steerlinedebug\\")) throw new Error("Fresh isolated live test root required");
  const manifest = JSON.parse(readFileSync(join(root, "manifest.json"), "utf8"));
  expect(manifest.schema).toBe("roadmap-browser-live/v1");
  expect(manifest.repository).toBe("erranliu/SteerlineTest");
  expect(manifest.base_branch).toMatch(/^codex\/roadmap-e2e\/[\w-]+\/base$/);
  const facts: any = { schema: manifest.schema, stages: [] };
  const save = () => writeFileSync(join(root, "evidence", "browser-facts.json"), JSON.stringify(facts, null, 2));
  const stage = (value: string) => { facts.stages.push(value); save(); console.log(value); };
  const commands: any[] = [];
  const errors: string[] = [];
  page.on("pageerror", e => errors.push(e.message));
  page.on("request", request => {
    if (request.method() === "POST" && request.url().endsWith("/commands")) {
      commands.push(request.postDataJSON());
      facts.commands = commands; save();
    }
  });
  const env = { ...process.env, STEERLINE_HOME: join(root, "home") };
  let opening = "";
  await expect.poll(() => {
    try { opening = execFileSync(join(root, "bin", "steerline-web.exe"), ["open", "-print-url"], { env, encoding: "utf8", timeout: 30000, stdio: "pipe" }).trim(); return !!opening; }
    catch { return false; }
  }, { timeout: 180000 }).toBe(true);
  await page.goto(opening);
  if (!/\/projects\//.test(page.url())) {
    const combo = page.getByRole("combobox", { name: "当前项目" });
    await expect.poll(() => combo.locator("option:not([disabled])").count()).toBe(1);
    await combo.selectOption({ index: 1 });
  }
  await expect(page.getByText("服务已连接", { exact: true })).toBeVisible({ timeout: 180000 });
  const project = page.url().split("/projects/")[1].split("/")[0];
  const origin = "http://" + manifest.web_address;
  const prefix = origin + "/api/v2/application/projects/" + project;
  const get = async (path: string) => {
    const response = await page.request.get(prefix + path, { timeout: 30000 });
    expect(response.ok(), path + " " + response.status()).toBe(true);
    return response.json();
  };
  const record = async (id: string) => get("/roadmaps/" + id);
  const waitRecord = async (id: string, state: string) => {
    let current: any;
    const deadline = Date.now() + 600000;
    while (Date.now() < deadline) {
      current = await record(id); facts.latest = current; save();
      if (["blocked", "documents-blocked", "failed", "rejected", "invalid", "cancelled"].includes(current.state)) throw new Error(JSON.stringify(current));
      if (current.state === state) return current;
      await new Promise(resolve => setTimeout(resolve, 2000));
    }
    throw new Error("Original record did not reach " + state + ": " + JSON.stringify(current));
  };
  const replay = async (action: string) => {
    const original = commands.find(c => c.action === action);
    expect(original).toBeTruthy();
    const session = await (await page.request.get(origin + "/auth/status")).json();
    const response = await page.request.post(prefix + "/commands", { data: original, headers: { "X-CSRF-Token": session.csrf_token, Origin: origin }, timeout: 90000 });
    expect(response.ok()).toBe(true);
    return response.json();
  };
  expect((await get("/tasks")).tasks).toHaveLength(0);
  await page.getByRole("link", { name: "Roadmap 草稿", exact: true }).click();
  const source = page.getByRole("textbox", { name: "Markdown 原文 · 只读" });
  await expect(source).toHaveValue(/T1[\s\S]*T2/);
  await expect(source).toHaveAttribute("readonly", "");
  await readable(source);
  await readable(page.locator(".roadmaps-library button.active"));
  expect((await get("/roadmaps")).records).toHaveLength(0);
  expect(commands).toHaveLength(0);
  await page.getByRole("button", { name: "Runner generate · 解析任务与依赖", exact: true }).click();
  await expect.poll(async () => (await get("/roadmaps")).records.length).toBe(1);
  const parseID = (await get("/roadmaps")).records[0].id;
  facts.parse_id = parseID; save();
  await expect.poll(async () => !!(await record(parseID)).run_id).toBe(true);
  await page.reload();
  let parsed = await waitRecord(parseID, "review");
  expect(parsed.analysis.tasks.map((t: any) => t.local_key).sort()).toEqual(["T1", "T2"]);
  expect(parsed.analysis.tasks.find((t: any) => t.local_key === "T2").proposed_dependencies.local).toContain("T1");
  expect(parsed.formal_mapping || {}).toEqual({});
  expect(commands.filter(c => c.action === "roadmap_parse")).toHaveLength(1);
  await expect(page.getByRole("button", { name: "确认任务与依赖", exact: true })).toBeEnabled();
  await page.getByRole("button", { name: "确认任务与依赖", exact: true }).click();
  await waitRecord(parseID, "confirmed");
  expect((await get("/tasks")).tasks).toHaveLength(0);
  stage("Parsed and confirmed local keys and T2 -> T1 dependency; zero formal tasks");
  await page.getByRole("button", { name: "生成 Spec / Plan", exact: true }).click();
  let docID = "";
  await expect.poll(async () => {
    docID = (await get("/roadmaps")).records.find((r: any) => r.purpose === "documents")?.id || "";
    return !!docID;
  }).toBe(true);
  facts.document_id = docID; save();
  await expect.poll(async () => !!(await record(docID)).run_id).toBe(true);
  await page.reload();
  let doc = await waitRecord(docID, "documents-review");
  expect(doc.code_commit).toBe(manifest.base_commit);
  expect(doc.investigation.length).toBeGreaterThan(0);
  expect(doc.documents).toHaveLength(2);
  for (const d of doc.documents) {
    for (const file of [d.spec, d.plan, ...d.acceptance]) {
      expect(file.body.length).toBeGreaterThan(100);
      expect(file.path).toBeTruthy();
    }
    await page.getByRole("button", { name: d.local_key, exact: true }).click();
    for (const label of ["Spec", "Plan", "验收材料"]) await page.getByRole("button", { name: label, exact: true }).click();
  }
  expect((await get("/tasks")).tasks).toHaveLength(0);
  expect(execFileSync("git", ["-C", join(root, "project"), "rev-parse", "HEAD"], { encoding: "utf8" }).trim()).toBe(manifest.base_commit);
  await page.getByRole("button", { name: "T1", exact: true }).click();
  await page.getByRole("button", { name: "Spec", exact: true }).click();
  await page.getByRole("button", { name: "修改生成正文", exact: true }).click();
  const editor = page.getByRole("textbox", { name: "修改生成正文", exact: true });
  await editor.fill((await editor.inputValue()) + "\n\n## 浏览器审阅\n保持纯算术范围，正式编号在入图时分配。\n");
  await page.getByRole("button", { name: "保存并重新核对", exact: true }).click();
  await expect.poll(async () => (await record(docID)).review_digest).not.toBe(doc.review_digest);
  doc = await record(docID);
  expect(doc.documents.find((d: any) => d.local_key === "T1").spec.body).toContain("浏览器审阅");
  expect(doc.publication_preview.files).toHaveLength(8);
  expect(doc.publication_preview.branch).toBe(manifest.base_branch);
  const reviewedFiles = doc.publication_preview.files;
  await readable(page.locator(".roadmaps-section pre:visible").first());
  await page.getByText("完整发布内容与目标", { exact: true }).click();
  await page.screenshot({ path: info.outputPath("documents-review.png"), fullPage: true });
  stage("Native repository investigation and six documents reviewed and edited; still zero formal tasks");
  await page.getByRole("button", { name: "确认发布并合并文档", exact: true }).click();
  doc = await waitRecord(docID, "awaiting-admission");
  facts.pull_request = doc.publication.url; save();
  expect(doc.publication.state).toBe("merged");
  expect((await get("/tasks")).tasks).toHaveLength(0);
  await replay("roadmap_publish");
  stage("Real external document PR merged; original publish request replayed; zero formal tasks");
  await expect(page.getByRole("button", { name: "正式入 DAG · 分配编号", exact: true })).toBeEnabled();
  await page.getByRole("button", { name: "正式入 DAG · 分配编号", exact: true }).click();
  doc = await waitRecord(docID, "admitted");
  expect(Object.keys(doc.formal_mapping).sort()).toEqual(["T1", "T2"]);
  expect(new Set(Object.values(doc.formal_mapping)).size).toBe(2);
  await replay("roadmap_admit");
  expect((await record(docID)).formal_mapping).toEqual(doc.formal_mapping);
  expect((await get("/tasks")).tasks).toHaveLength(2);
  const snapshot = await get("/snapshot");
  expect(snapshot.connection.snapshot.page.runs ?? []).toHaveLength(0);
  const graph = await get("/graph");
  expect(graph.nodes).toHaveLength(2);
  expect(graph.edges).toEqual([{ predecessor: doc.formal_mapping.T1, successor: doc.formal_mapping.T2 }]);
  const external = JSON.parse(execFileSync("gh", ["api", "repos/erranliu/SteerlineTest/pulls/" + new URL(doc.publication.url).pathname.split("/").at(-1)], { encoding: "utf8", timeout: 30000 }));
  expect(external.merged).toBe(true);
  expect(external.head.ref).toBe("codex/roadmap-" + docID);
  expect(external.base.ref).toBe(manifest.base_branch);
  expect(external.head.sha).toBe(doc.publication.commit);
  expect(external.merge_commit_sha).toBe(doc.publication.merged_commit);
  for (const file of reviewedFiles) {
    const merged = execFileSync("git", ["-C", join(root, "project"), "show", doc.publication.merged_commit + ":" + file.path], { encoding: "utf8" });
    expect(merged).toBe(file.after);
  }
  facts.graph = graph;
  facts.external_pr = external;
  facts.snapshot = snapshot; save();
  await page.reload();
  await page.getByRole("link", { name: "#" + doc.formal_mapping.T1, exact: true }).click();
  await expect(page.locator(".graph-task")).toHaveCount(2);
  await expect(page.getByRole("link", { name: "arithmetic-roadmap.md / T1", exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath("admitted-dag.png"), fullPage: true });
  await page.getByRole("link", { name: "arithmetic-roadmap.md / T1", exact: true }).click();
  await expect(page.getByRole("button", { name: "正式入 DAG · 分配编号", exact: true })).toBeDisabled();
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), "Narrow screen must not scroll the whole page horizontally").toBe(true);
  await page.screenshot({ path: info.outputPath("roadmap-mobile.png"), fullPage: true });
  expect(errors).toEqual([]);
  facts.mapping = doc.formal_mapping; facts.passed = true;
  stage("Atomic numbering, original admission replay, DAG and source navigation, reload and mobile viewport passed");
});
