import { test, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { writeFileSync } from "node:fs";

test("native.discovery: Web-owned local and paired remote catalogs", async ({
  page,
}, info) => {
  const binary = process.env.STEERLINE_CONFIG_WEB;
  const environment = process.env.STEERLINE_CONFIG_REMOTE;
  const localOrca = process.env.STEERLINE_CONFIG_LOCAL_ORCA === "1";
  if (
    !binary ||
    (!environment && !localOrca) ||
    !process.env.STEERLINE_HOME?.startsWith("C:\\SteerlineDebug\\")
  )
    throw new Error("new debug package driver required");
  const url = new URL(
    execFileSync(binary, ["open", "-print-url"], {
      encoding: "utf8",
      windowsHide: true,
    }).trim(),
  );
  url.pathname = "/settings/execution-configs";
  await page.goto(url.href);
  await expect(page.getByLabel("配置名称")).toBeVisible();
  await expect
    .poll(
      async () => {
        const catalog = await (
          await page.request.get(url.origin + "/api/v2/application/projects")
        ).json();
        if (catalog.projects?.some((p: any) => p.runtime_state === "failed"))
          throw new Error("debug project initialization failed");
        return (
          catalog.projects?.length > 0 &&
          catalog.projects.every((p: any) => p.online)
        );
      },
      { timeout: 180000, intervals: [500, 1000, 2000] },
    )
    .toBe(true);
  const snapshot = await (
    await page.request.get(url.origin + "/api/web/execution-config")
  ).json();
  expect(snapshot.writable).toBe(false);
  const session = await (
    await page.request.get(url.origin + "/auth/status")
  ).json();
  const methods: unknown[] = [];
  const query = async (request: object) => {
    const response = await page.request.post(
      url.origin + "/api/web/execution-config/discover",
      {
        headers: { Origin: url.origin, "X-CSRF-Token": session.csrf_token },
        data: request,
        timeout: 40000,
      },
    );
    expect(response.ok()).toBe(true);
    const result = await response.json();
    methods.push({ request, result });
    writeFileSync(
      info.outputPath("discovery.json"),
      JSON.stringify(
        {
          scope: "catalog only; no execution authorization asserted",
          queries: methods,
        },
        null,
        2,
      ),
    );
    return result;
  };
  const local = await query({
    kind: "models",
    host: "local",
    adapter: "codex-app-server",
    agent: "codex",
    profile_id: snapshot.config.execution_profiles.find(
      (p: any) => p.execution.adapter === "codex-app-server",
    )?.id,
  });
  expect(local.state, local.diagnostic).toBe("ready");
  expect(local.models.length).toBeGreaterThan(0);
  expect(local.context_unverified).toBe(true);
  if (localOrca) {
    const catalog = await query({
      kind: "models",
      host: "local",
      adapter: "orca-terminal",
      agent: "codex",
      refresh: true,
    });
    expect(catalog.state, catalog.diagnostic).toBe("ready");
    expect(catalog.models.length).toBeGreaterThan(0);
    expect(catalog.context_unverified).toBe(true);
    for (const agent of ["pi", "dsh"]) {
      const result = await query({
        kind: "models",
        host: "local",
        adapter: "orca-terminal",
        agent,
        refresh: true,
      });
      expect(["ready", "unsupported", "unavailable", "in_progress"]).toContain(
        result.state,
      );
      if (result.state === "unsupported")
        expect(result.models ?? []).toHaveLength(0);
    }
    await page.getByLabel("执行链路").selectOption("orca-terminal");
    await expect(page.getByLabel("Agent")).toBeVisible();
    await page.screenshot({
      path: info.outputPath("native-local-orca.png"),
      fullPage: true,
    });
    return;
  }
  const environments = await query({
    kind: "environments",
    host: "orca_environment",
  });
  expect(
    environments.environments.some(
      (e: any) => e.id === environment || e.name === environment,
    ),
  ).toBe(true);
  let remote: any;
  for (let i = 0; i < 3; i++) {
    remote = await query({
      kind: "models",
      host: "orca_environment",
      adapter: "orca-terminal",
      environment,
      agent: "codex",
      refresh: true,
    });
    if (remote.state !== "in_progress") break;
  }
  expect(remote.state, remote.diagnostic).toBe("ready");
  expect(remote.models.length).toBeGreaterThan(0);
  expect(remote.context_unverified).toBe(true);
  await page.screenshot({
    path: info.outputPath("native-local.png"),
    fullPage: true,
  });
});
