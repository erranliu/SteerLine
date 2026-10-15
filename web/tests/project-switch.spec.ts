import { test, expect } from "@playwright/test";
import { spawn, type ChildProcess } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { freshDualPackage, packageCommand } from "./helpers/package";

test("web.project-switch: independent processes, original commands and active turn survive restart", async ({
  page,
  context,
}, info) => {
  const { root, manifest } = freshDualPackage();
  const home = join(root, "home"),
    binary = join(root, "bin", "steerline.exe"),
    web = join(root, "bin", "steerline-web.exe"),
    helper = join(root, "bin", "debug-helper.exe");
  const env = {
    ...process.env,
    STEERLINE_HOME: home,
    STEERLINE_E2E_CONTROLLED_STATE: join(root, "provider", "state.json"),
  };
  const command = packageCommand(env);
  const children: ChildProcess[] = [];
  const launch = (exe: string, args: string[]) => {
    const child = spawn(exe, args, { env, windowsHide: true, stdio: "ignore" });
    children.push(child);
    return child;
  };
  const provider = () =>
    JSON.parse(readFileSync(join(root, "provider", "state.json"), "utf8"));
  const origin = "http://" + manifest.web_address;
  const facts: Record<string, unknown> = {
    source_commit: manifest.source_commit,
    source_dirty: manifest.source_dirty,
    package: root,
  };
  const failures: string[] = [];
  let stoppedB = false;
  command(helper, ["debug", "prepare", "--simulate"]);
  const state = provider();
  state.recovery = { HoldStep: "implement.execute", HoldStream: true };
  state.web_feedback = false;
  writeFileSync(join(root, "provider", "state.json"), JSON.stringify(state));
  launch(helper, ["debug", "run", "--simulate"]);
  let aid = "",
    bid = "";
  const open = async (project = "") => {
    let url = "";
    await expect
      .poll(
        () => {
          try {
            url = command(web, [
              "open",
              ...(project ? ["-project", project] : []),
              "-print-url",
            ]);
            return true;
          } catch {
            return false;
          }
        },
        { timeout: 90000 },
      )
      .toBe(true);
    await page.goto(url);
  };
  try {
    await open();
    const combo = page.getByRole("combobox", { name: "当前项目" });
    await expect
      .poll(() => combo.locator("option:not([disabled])").count())
      .toBe(2);
    const projects = await (
      await page.request.get(origin + "/api/v1/application/projects")
    ).json();
    const a = projects.projects.find(
        (p: { config_id: string }) => p.config_id === "SteerlineTest.json",
      ),
      b = projects.projects.find(
        (p: { config_id: string }) => p.config_id === "SteerlineTestB.json",
      );
    aid = a.project_id;
    bid = b.project_id;
    expect(aid).not.toBe(bid);
    await expect
      .poll(
        async () => {
          const catalog = await (
            await page.request.get(origin + "/api/v1/application/projects")
          ).json();
          facts.last_catalog = catalog;
          return catalog.projects.every((p: { online: boolean }) => p.online);
        },
        { timeout: 180000 },
      )
      .toBe(true);
    console.log("web.project-switch: both configured projects online");
    await combo.selectOption(aid);
    await expect(page.getByText("服务已连接", { exact: true })).toBeVisible();
    await expect(page.locator(".graph-task")).toHaveCount(2);
    const get = async (project: string, route: string) => {
      let body: any;
      await expect
        .poll(
          async () => {
            try {
              const response = await page.request.get(
                origin + "/api/v1/application/projects/" + project + route,
                { timeout: 20000 },
              );
              body = await response.json();
              return response.ok();
            } catch {
              return false;
            }
          },
          { timeout: 90000, message: "original project query: " + route },
        )
        .toBe(true);
      return body;
    };
    const originalIdentity = await get(aid, "/identity"),
      otherIdentity = await get(bid, "/identity");
    facts.project_a = originalIdentity;
    facts.project_b = otherIdentity;
    // The second tab retains A while this tab switches to B, even with same Task IDs.
    const second = await context.newPage();
    await second.goto(origin + "/projects/" + aid + "/settings");
    await expect(second.getByText("服务已连接", { exact: true })).toBeVisible();
    await combo.selectOption(bid);
    await expect(page).toHaveURL(origin + "/projects/" + bid + "/dag");
    await expect(page.locator(".graph-task")).toHaveCount(2);
    expect(second.url()).toContain(aid);
    await page.screenshot({
      path: info.outputPath("project-b.png"),
      fullPage: true,
    });
    await page.getByRole("combobox", { name: "当前项目" }).selectOption(aid);
    let submissions = 0,
      requestID = "";
    await page.route(
      "**/api/v1/application/projects/**/commands",
      async (route) => {
        const envelope = route.request().postDataJSON();
        if (envelope.action === "start_task") {
          submissions++;
          requestID = envelope.request_id;
          if (submissions === 1) {
            await route.fetch();
            await route.abort("failed");
            return;
          }
        }
        await route.continue();
      },
    );
    await page.getByRole("button", { name: "恢复派发", exact: true }).click();
    await expect(
      page.getByRole("button", { name: "暂停派发", exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "任务列表", exact: true }).click();
    await page
      .getByRole("button")
      .filter({ hasText: "SteerlineTest 独立调试任务 A" })
      .click();
    await page.getByRole("button", { name: "手动启动", exact: true }).click();
    await page.getByRole("button", { name: "确认启动", exact: true }).click();
    await expect
      .poll(() => provider().turns?.[0]?.status, { timeout: 90000 })
      .toBe("inProgress");
    await expect
      .poll(
        async () => {
          const receipt = await get(aid, "/commands/" + requestID);
          return receipt.status;
        },
        { timeout: 45000 },
      )
      .toBe("applied");
    expect(submissions).toBe(1);
    let active: any;
    await expect
      .poll(async () => {
        const snap = await get(aid, "/snapshot");
        active = snap.connection.snapshot.page.runs[0];
        return !!active?.current_turn_id && !!active?.session_id;
      })
      .toBe(true);
    const turn = provider().turns[0].id,
      session = provider().session;
    facts.active_before = active;
    const nativeProcessPath = join(
      home,
      "state",
      "SteerlineTest-" + aid.slice(0, 12),
      "runs",
      active.run_id,
      "app-server",
      "process.json",
    );
    const nativeProcess = JSON.parse(readFileSync(nativeProcessPath, "utf8"));
    expect(
      Number.isInteger(nativeProcess.identity.pid) &&
        nativeProcess.identity.pid > 0,
    ).toBe(true);
    facts.native_process = nativeProcess;
    const checkNativeProcess = () => {
      expect(JSON.parse(readFileSync(nativeProcessPath, "utf8"))).toEqual(
        nativeProcess,
      );
      const proof = JSON.parse(
        command("powershell.exe", [
          "-NoProfile",
          "-Command",
          `$p=Get-Process -Id ${nativeProcess.identity.pid};@{pid=$p.Id;created=$p.StartTime.ToFileTimeUtc().ToString('x16');executable=$p.Path.ToLowerInvariant()}|ConvertTo-Json -Compress`,
        ]),
      );
      expect(proof).toEqual({
        pid: nativeProcess.identity.pid,
        created: nativeProcess.identity.created,
        executable: nativeProcess.identity.executable,
      });
    };
    checkNativeProcess();
    console.log("web.project-switch: original native turn held");
    facts.original_request_id = requestID;
    command(web, ["stop"]);
    launch(web, ["serve"]);
    await open(aid);
    await expect(page.getByText("服务已连接", { exact: true })).toBeVisible();
    const assertOriginal = async () => {
      checkNativeProcess();
      expect(provider().sessions).toBe(1);
      expect(provider().dispatches).toBe(1);
      expect(await get(aid, "/identity")).toMatchObject({
        instance_id: originalIdentity.instance_id,
        process: originalIdentity.process,
      });
      expect(await get(bid, "/identity")).toMatchObject({
        instance_id: otherIdentity.instance_id,
        process: otherIdentity.process,
      });
      const snap = await get(aid, "/snapshot");
      expect(snap.connection.snapshot.page.runs[0]).toMatchObject({
        execution_id: active.execution_id,
        attempt_id: active.attempt_id,
        session_id: active.session_id,
        current_turn_id: active.current_turn_id,
      });
      expect(provider().session).toBe(session);
      expect(provider().turns[0]).toMatchObject({
        id: turn,
        status: "inProgress",
      });
    };
    await assertOriginal();
    const appBefore = JSON.parse(
      readFileSync(join(home, "application-api", "discovery.json"), "utf8"),
    ).identity;
    command(binary, ["stop-app"]);
    launch(binary, ["serve-app"]);
    await expect(page.getByText("服务已连接", { exact: true })).toBeVisible();
    await assertOriginal();
    await expect
      .poll(
        () =>
          JSON.parse(
            readFileSync(
              join(home, "application-api", "discovery.json"),
              "utf8",
            ),
          ).identity.instance_id,
      )
      .not.toBe(appBefore.instance_id);
    console.log(
      "web.project-switch: Web and application restarted; original turn unchanged",
    );
    command(helper, [
      "release-recovery",
      "--e2e-state",
      join(root, "provider", "state.json"),
    ]);
    await expect
      .poll(() => provider().turns.length, { timeout: 120000 })
      .toBe(3);
    await expect
      .poll(
        async () => {
          const snap = await get(aid, "/snapshot");
          return snap.connection.snapshot.page.runs[0].workflow_stage;
        },
        { timeout: 120000 },
      )
      .toBe("await_merge");
    const finished = (await get(aid, "/snapshot")).connection.snapshot.page
      .runs[0];
    expect(finished.execution_id).toBe(active.execution_id);
    expect(finished.attempt_id).toBe(active.attempt_id);
    expect(finished.session_id).toBe(session);
    facts.active_after = finished;
    console.log("web.project-switch: original execution completed publication");
    // Restart A after delivery while B remains independently available.
    command(helper, ["debug", "merge", "--simulate"]);
    await expect
      .poll(async () => !!(await get(aid, "/tasks/1")).tasks[0].delivery, {
        timeout: 60000,
      })
      .toBe(true);
    command(binary, [
      "stop-service",
      "-config",
      join(home, "etc", "SteerlineTest.json"),
    ]);
    expect(await get(bid, "/identity")).toMatchObject({
      instance_id: otherIdentity.instance_id,
      process: otherIdentity.process,
    });
    launch(binary, [
      "run",
      "-config",
      join(home, "etc", "SteerlineTest.json"),
      "-dispatch",
      "manual",
      "-console",
      "off",
    ]);
    await expect
      .poll(
        async () => {
          const current = await get(aid, "/identity");
          return (
            !!current.instance_id &&
            current.instance_id !== originalIdentity.instance_id
          );
        },
        { timeout: 180000 },
      )
      .toBe(true);
    await expect(page.getByText("服务已连接", { exact: true })).toBeVisible();
    expect(
      (await get(aid, "/commands/" + requestID)).original.instance_id,
    ).toBe(originalIdentity.instance_id);
    expect(submissions).toBe(1);
    expect(provider().sessions).toBe(1);
    expect(provider().dispatches).toBe(3);
    facts.restarted_project = await get(aid, "/identity");
    console.log(
      "web.project-switch: project restarted; original receipt retained without a new worker",
    );
    command(binary, [
      "stop-service",
      "-config",
      join(home, "etc", "SteerlineTestB.json"),
    ]);
    stoppedB = true;
    await page.getByRole("combobox", { name: "当前项目" }).selectOption(bid);
    await expect(page.getByText(/当前不可提交业务操作/)).toBeVisible();
    expect(
      (await get(aid, "/commands/" + requestID)).original.instance_id,
    ).toBe(originalIdentity.instance_id);
    await second.close();
    await page.screenshot({
      path: info.outputPath("offline-project.png"),
      fullPage: true,
    });
    facts.submissions = submissions;
  } finally {
    for (const [exe, args] of [
      [web, ["stop"]],
      [binary, ["stop-app"]],
      [
        binary,
        ["stop-service", "-config", join(home, "etc", "SteerlineTestB.json")],
      ],
      [
        binary,
        ["stop-service", "-config", join(home, "etc", "SteerlineTest.json")],
      ],
    ] as [string, string[]][]) {
      if (stoppedB && args.includes(join(home, "etc", "SteerlineTestB.json")))
        continue;
      try {
        command(exe, args);
      } catch (error) {
        if (!String(error).includes("discovery")) failures.push(String(error));
      }
    }
    await expect
      .poll(
        () =>
          Number(
            command("powershell.exe", [
              "-NoProfile",
              "-Command",
              `@(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -like '${root.replaceAll("'", "''")}\\*' }).Count`,
            ]),
          ),
        { timeout: 30000 },
      )
      .toBe(0);
    const inventory = JSON.parse(
      command("powershell.exe", [
        "-NoProfile",
        "-Command",
        `$p=@(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -like '${root.replaceAll("'", "''")}\\*' }); $l=@(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Where-Object { $_.OwningProcess -in $p.ProcessId -or $_.LocalPort -eq ${manifest.web_address.split(":")[1]} }); @{processes=$p.Count;listeners=$l.Count}|ConvertTo-Json -Compress`,
      ]),
    );
    facts.cleanup = inventory;
    facts.stop_errors = failures;
    await info.attach("process-command-turn-evidence", {
      body: JSON.stringify(facts, null, 2),
      contentType: "application/json",
    });
    writeFileSync(
      join(root, "evidence", "web-project-switch.json"),
      JSON.stringify(facts, null, 2),
    );
    expect(inventory).toEqual({ processes: 0, listeners: 0 });
    expect(failures).toEqual([]);
  }
});
