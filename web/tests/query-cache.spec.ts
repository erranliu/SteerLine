import { expect, test } from "@playwright/test";
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { preparePackageRuntime } from "./helpers/package";

test("web.query-cache: real requests share cache, preserve content and stop hidden polling across projects", async ({
  page,
}, info) => {
  const runtime = preparePackageRuntime();
  const { root, manifest, home, binary, web, helper, command } = runtime;
  const origin = "http://" + manifest.web_address;
  const requests: string[] = [];
  const pageErrors: string[] = [];
  page.on("request", (request) => {
    if (request.method() === "GET") requests.push(request.url());
  });
  page.on("pageerror", (error) => pageErrors.push(String(error)));
  let primaryFailure: unknown;
  const facts: Record<string, unknown> = {
    source_commit: manifest.source_commit,
    source_dirty: manifest.source_dirty,
    package: root,
    requests,
  };
  let releaseTasks: (() => void) | undefined;
  let held = false;
  let holdNextTasks = true;
  let projectA = "";
  let projectB = "";
  let repositoryA = "";
  let repositoryB = "";
  const count = (project: string, resource: string) =>
    requests.filter(
      (url) =>
        new URL(url).pathname ===
        "/api/v2/application/projects/" + project + "/" + resource,
    ).length;
  await page.route("**/tasks?**", async (route) => {
    const response = await route.fetch();
    if (holdNextTasks) {
      holdNextTasks = false;
      held = true;
      await new Promise<void>((done) => {
        releaseTasks = done;
      });
    }
    await route.fulfill({ response });
  });
  try {
    facts.startup = await runtime.start();
    let opening = "";
    await expect
      .poll(
        () => {
          try {
            opening = command(web, ["open", "-print-url"]);
            return true;
          } catch {
            return false;
          }
        },
        { timeout: 90000 },
      )
      .toBe(true);
    await page.goto(opening);
    await expect
      .poll(
        async () => {
          const response = await page.request.get(
            origin + "/api/v2/application/projects",
          );
          const catalog = await response.json();
          projectA =
            catalog.projects.find(
              (p: { config_id: string }) =>
                p.config_id === "SteerlineTest.json",
            )?.project_id ?? "";
          projectB =
            catalog.projects.find(
              (p: { config_id: string }) =>
                p.config_id === "SteerlineTestB.json",
            )?.project_id ?? "";
          repositoryA =
            catalog.projects.find(
              (p: { project_id: string }) => p.project_id === projectA,
            )?.repository ?? "";
          repositoryB =
            catalog.projects.find(
              (p: { project_id: string }) => p.project_id === projectB,
            )?.repository ?? "";
          return (
            !!projectA &&
            !!projectB &&
            catalog.projects.every((p: { online: boolean }) => p.online)
          );
        },
        { timeout: 180000 },
      )
      .toBe(true);
    await page
      .getByRole("combobox", { name: "当前项目" })
      .selectOption(projectA);
    await expect(page.getByText("服务已连接", { exact: true })).toBeVisible({
      timeout: 180000,
    });
    await expect(page.locator(".graph-task")).toHaveCount(2);
    await expect.poll(() => held).toBe(true);
    // The list page attaches to the original, still pending bootstrap request.
    await page.getByRole("link", { name: "执行与待处理" }).click();
    await expect(
      page.getByRole("heading", { name: "执行与待处理", exact: true }),
    ).toBeVisible();
    expect(count(projectA, "tasks")).toBe(1);
    releaseTasks!();
    releaseTasks = undefined;
    await expect(page.locator(".run-task")).toHaveCount(2);
    expect(count(projectA, "tasks")).toBe(1);
    expect(count(projectB, "tasks")).toBe(0);
    expect(count(projectB, "presentation")).toBe(0);
    facts.concurrent_tasks_requests = count(projectA, "tasks");
    console.log("web.query-cache: pending bootstrap shared by both pages");

    await page.getByRole("link", { name: "执行设置" }).click();
    const presentationCount = count(projectA, "presentation");
    await page.waitForTimeout(1100); // Make the dynamic task page stale, with no observer polling it.
    held = false;
    holdNextTasks = true;
    await page.getByRole("link", { name: "执行与待处理" }).click();
    await expect.poll(() => held).toBe(true);
    // Cached rows are visible before releasing the actual server response.
    await expect(page.locator(".run-task")).toHaveCount(2);
    facts.cached_rows_before_response = await page.locator(".run-task").count();
    console.log("web.query-cache: cached rows visible before server response");
    releaseTasks!();
    releaseTasks = undefined;
    await page.unrouteAll({ behavior: "wait" });
    expect(count(projectA, "presentation")).toBe(presentationCount);

    await page.getByRole("link", { name: "任务 DAG" }).click();
    await expect(page.locator(".graph-task")).toHaveCount(2);
    await page.getByRole("button", { name: "任务列表", exact: true }).click();
    await expect(page.locator(".task-row")).toHaveCount(2);
    await page.waitForTimeout(3000); // Drain a graph request already in flight during the tab change.
    const hiddenGraphCount = count(projectA, "graph");
    await page.waitForTimeout(5500);
    expect(count(projectA, "graph")).toBe(hiddenGraphCount);
    console.log("web.query-cache: hidden graph polling stopped");
    facts.hidden_graph_new_requests =
      count(projectA, "graph") - hiddenGraphCount;
    expect(count(projectA, "presentation")).toBe(presentationCount);
    expect(count(projectB, "tasks")).toBe(0);

    await page
      .getByRole("combobox", { name: "当前项目" })
      .selectOption(projectB);
    await expect(page).toHaveURL(origin + "/projects/" + projectB + "/dag");
    await expect(page.getByText("服务已连接", { exact: true })).toBeVisible();
    await expect(page.locator(".graph-task")).toHaveCount(2);
    await expect.poll(() => count(projectB, "tasks")).toBeGreaterThan(0);
    await expect(page.locator(".project-current .path")).toHaveText(
      repositoryB,
    );
    await page
      .getByRole("combobox", { name: "当前项目" })
      .selectOption(projectA);
    await expect(page.getByText("服务已连接", { exact: true })).toBeVisible();
    await expect(page.locator(".graph-task")).toHaveCount(2);
    await expect(page.locator(".project-current .path")).toHaveText(
      repositoryA,
    );
    await page.screenshot({
      path: info.outputPath("query-cache.png"),
      fullPage: true,
    });
    expect(pageErrors).toEqual([]);
    console.log("web.query-cache: both project switches verified");
    facts.project_a = projectA;
    facts.project_b = projectB;
    facts.page_errors = pageErrors;
    facts.requests = requests;
  } catch (error) {
    primaryFailure = error;
    facts.business_error = String(error);
    throw error;
  } finally {
    releaseTasks?.();
    const stopErrors = await runtime.stop();
    const inspect = () =>
      JSON.parse(
        command("powershell.exe", [
          "-NoProfile",
          "-Command",
          `$p=@(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -like '${root.replaceAll("'", "''")}\\*' }); $l=@(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Where-Object { $_.OwningProcess -in $p.ProcessId -or $_.LocalPort -eq ${Number(manifest.web_address.split(":")[1])} }); @{processes=$p.Count;listeners=$l.Count}|ConvertTo-Json -Compress`,
        ]),
      );
    let inventory: { processes: number; listeners: number } | undefined;
    let cleanupError: unknown;
    try {
      await expect
        .poll(
          () => {
            inventory = inspect();
            return inventory;
          },
          { timeout: 30000 },
        )
        .toEqual({ processes: 0, listeners: 0 });
    } catch (error) {
      cleanupError = error;
      facts.cleanup_error = String(error);
    }
    facts.cleanup = inventory ?? null;
    facts.stop_errors = stopErrors;
    facts.page_errors = pageErrors;
    writeFileSync(
      join(root, "evidence", "web-query-cache.json"),
      JSON.stringify(facts, null, 2),
    );
    await info.attach("query-cache-facts", {
      body: JSON.stringify(facts, null, 2),
      contentType: "application/json",
    });
    if (!primaryFailure) {
      if (cleanupError) throw cleanupError;
      expect(stopErrors).toEqual([]);
    }
  }
});
