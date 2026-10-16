import { test, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";

test("native session: original candidate viewer and replay through product HTTP", async ({
  page,
}, testInfo) => {
  test.setTimeout(240000);
  const config = process.env.STEERLINE_WEB_CONFIG;
  if (!config) throw new Error("fresh simulate package required");
  const root = resolve(dirname(config), "../..");
  if (!root.toLowerCase().startsWith("c:\\steerlinedebug\\"))
    throw new Error("isolated package required");
  const manifest = JSON.parse(
    readFileSync(join(root, "manifest.json"), "utf8"),
  );
  const scheme = JSON.parse(readFileSync(join(root, "scheme.json"), "utf8"));
  expect(scheme.scheme).toBe("simulate");
  const env = { ...process.env, STEERLINE_HOME: join(root, "home") };
  let opening = "";
  await expect
    .poll(
      () => {
        try {
          opening = execFileSync(
            join(root, "bin", "steerline-web.exe"),
            ["open", "-print-url"],
            {
              env,
              encoding: "utf8",
              timeout: 30000,
              windowsHide: true,
              stdio: "pipe",
            },
          ).trim();
          return !!opening;
        } catch {
          return false;
        }
      },
      { timeout: 120000 },
    )
    .toBe(true);
  await page.goto(opening);
  console.log("native viewer: original Web opened");
  if (!/\/projects\//.test(page.url())) {
    const combo = page.getByRole("combobox", { name: "当前项目" });
    await expect
      .poll(() => combo.locator("option:not([disabled])").count())
      .toBe(1);
    await combo.selectOption({ index: 1 });
  }
  await expect(page.getByText("服务已连接", { exact: true })).toBeVisible();
  console.log("native viewer: project connected");
  // The debug owner seeds tasks after the project becomes online. Do not
  // advance authority while its fixed initial task transaction is pending.
  await expect(page.locator(".graph-task")).toHaveCount(2);
  const projectID = page.url().split("/projects/")[1].split("/")[0];
  const prefix = `http://${manifest.web_address}/api/v2/application/projects/${projectID}`;
  const snapshot = async () =>
    (await (await page.request.get(prefix + "/snapshot")).json()).connection
      .snapshot.page;
  expect((await snapshot()).service.capabilities).toContain(
    "open_native_session:codex-app-server",
  );
  const providerPath = join(root, "provider", "state.json");
  const provider = JSON.parse(readFileSync(providerPath, "utf8"));
  provider.hold = true;
  writeFileSync(providerPath, JSON.stringify(provider));
  if (
    await page.getByRole("button", { name: "恢复派发", exact: true }).count()
  ) {
    await page.getByRole("button", { name: "恢复派发", exact: true }).click();
  }
  await page.getByRole("button", { name: "任务列表", exact: true }).click();
  await page
    .getByRole("button")
    .filter({ hasText: "SteerlineTest 独立调试任务 A" })
    .click();
  await page.getByRole("button", { name: "手动启动", exact: true }).click();
  await page.getByRole("button", { name: "确认启动", exact: true }).click();
  await expect
    .poll(async () => !!(await snapshot()).runs?.[0]?.session_id, {
      timeout: 90000,
    })
    .toBe(true);
  const before = (await snapshot()).runs[0];
  await page.getByRole("link", { name: /执行与待处理/ }).click();
  const button = page.getByRole("button", { name: /在 Codex 中查看/ });
  await expect(button).toBeEnabled();
  const requestPromise = page.waitForRequest(
    (r) =>
      r.method() === "POST" &&
      r.url() === prefix + "/commands" &&
      r.postDataJSON().action === "open_native_session",
  );
  const responsePromise = page.waitForResponse(
    (r) =>
      r.request().method() === "POST" &&
      r.url() === prefix + "/commands" &&
      r.request().postDataJSON().action === "open_native_session",
  );
  await button.click();
  const envelope = (await requestPromise).postDataJSON();
  const result = await (await responsePromise).json();
  expect(result.status).toBe("applied");
  expect(envelope.payload).toMatchObject({
    run_id: before.run_id,
    execution_id: before.execution_id,
    attempt_id: before.attempt_id,
    session_id: before.session_id,
    session_generation: before.session_generation,
  });
  const replay = await page.evaluate(
    async ({ url, body }) => {
      const auth = await (await fetch("/auth/status")).json();
      return await (
        await fetch(url, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": auth.csrf_token,
          },
          body: JSON.stringify(body),
        })
      ).json();
    },
    { url: prefix + "/commands", body: envelope },
  );
  expect(replay.status).toBe("applied");
  const after = (await snapshot()).runs[0];
  expect(after).toMatchObject({
    execution_id: before.execution_id,
    attempt_id: before.attempt_id,
    session_id: before.session_id,
    session_generation: before.session_generation,
    current_turn_id: before.current_turn_id,
  });
  const fixturePath = readdirSync(join(root, "home", "state"))
    .map((name) =>
      join(root, "home", "state", name, "runner", "caller-fixture.json"),
    )
    .find((path) => existsSync(path));
  expect(fixturePath).toBeTruthy();
  const fixture = JSON.parse(readFileSync(fixturePath!, "utf8"));
  expect(
    fixture.calls.filter((call: string) => call === "open-native-session"),
  ).toHaveLength(1);
  expect(Object.values(fixture.requests).map((r: any) => r.command)).toEqual([
    "submit",
  ]);
  await page.screenshot({
    path: testInfo.outputPath("native-session.png"),
    fullPage: true,
  });
  await testInfo.attach("original-viewer", {
    body: JSON.stringify({
      package: root,
      before,
      after,
      envelope,
      result,
      replay,
      viewer_calls: 1,
    }),
    contentType: "application/json",
  });
});
