import { test, expect } from "@playwright/test";
import { createHash, randomUUID } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { preparePackageRuntime } from "./helpers/package";

test("web.unified-startup: shared host, original commands and active turn survive Web restart", async ({
  page,
  context,
}, info) => {
  const runtime = preparePackageRuntime();
  const { root, manifest, home, binary, web, helper, command, launch } =
    runtime;
  const provider = () =>
    JSON.parse(readFileSync(join(root, "provider", "state.json"), "utf8"));
  const origin = "http://" + manifest.web_address;
  const facts: Record<string, unknown> = {
    source_commit: manifest.source_commit,
    source_dirty: manifest.source_dirty,
    package: root,
  };
  const failures: string[] = [];
  let primaryFailure: unknown;
  const state = provider();
  state.recovery = { HoldStep: "implement.execute", HoldStream: true };
  state.web_feedback = false;
  writeFileSync(join(root, "provider", "state.json"), JSON.stringify(state));
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
    // Native open creates a fresh browser document, even when the prior URL matches.
    await page.goto("about:blank");
    await page.goto(url);
  };
  try {
    facts.startup = await runtime.start();
    for (const target of [binary, web]) {
      let refused = false;
      try {
        command(target, ["install"]);
      } catch {
        refused = true;
      }
      expect(refused).toBe(true);
    }
    facts.running_install_rejected = true;
    await open();
    const combo = page.getByRole("combobox", { name: "当前项目" });
    await expect
      .poll(() => combo.locator("option:not([disabled])").count())
      .toBe(2);
    const projects = await (
      await page.request.get(origin + "/api/v2/application/projects")
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
            await page.request.get(origin + "/api/v2/application/projects")
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
                origin + "/api/v2/application/projects/" + project + route,
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
    expect(originalIdentity.process).toEqual(otherIdentity.process);
    expect(originalIdentity.instance_id).not.toBe(otherIdentity.instance_id);
    const statusBefore = JSON.parse(command(binary, ["status", "--json"]));
    command(binary, ["start", "--no-open"]);
    expect(
      JSON.parse(command(binary, ["status", "--json"])).backend.identity,
    ).toEqual(statusBefore.backend.identity);
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
      "**/api/v2/application/projects/**/commands",
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
    // CLI-only mutations and original receipts remain outside the browser proxy.
    const batch = JSON.parse(
      readFileSync(
        join(root, "seed", "SteerlineTest.json.bound-tasks.json"),
        "utf8",
      ),
    );
    const canonical = (value: any): any =>
      Array.isArray(value)
        ? value.map(canonical)
        : value && typeof value === "object"
          ? Object.fromEntries(
              Object.keys(value)
                .sort()
                .map((key) => [key, canonical(value[key])]),
            )
          : value;
    batch.tasks = [3, 4].map((id) => {
      const contract = {
        ...batch.tasks[0].contract,
        contract_id: "startup-cli-" + id,
        digest: "",
      };
      contract.digest = createHash("sha256")
        .update(JSON.stringify(canonical(contract)))
        .digest("hex");
      return {
        ...batch.tasks[0],
        contract,
        local_key: "cli-" + id,
        task_id: id,
        title: "CLI startup verification " + id,
      };
    });
    batch.dependencies = [];
    const input = join(root, "evidence", "cli-batch.json");
    writeFileSync(input, JSON.stringify(batch));
    const valid = JSON.parse(
      command(binary, ["task", "validate", "--project", aid, "--file", input]),
    );
    expect(valid.valid).toBe(true);
    const batchID = randomUUID().replaceAll("-", "");
    command(binary, [
      "task",
      "create",
      "--project",
      aid,
      "--file",
      input,
      "--request-id",
      batchID,
      "--expected-authority-version",
      String(valid.authority_version),
      "--by",
      "startup-test",
    ]);
    const batchReceipt = JSON.parse(
      command(binary, [
        "task",
        "receipt",
        "--project",
        aid,
        "--request-id",
        batchID,
      ]),
    );
    const dependencyID = randomUUID().replaceAll("-", "");
    command(binary, [
      "task",
      "add-predecessors",
      "--project",
      aid,
      "--task-id",
      "4",
      "--predecessors",
      "3",
      "--request-id",
      dependencyID,
      "--expected-version",
      "1",
      "--expected-graph-revision",
      String(batchReceipt.graph_revision),
      "--reason",
      "startup dependency check",
      "--by",
      "startup-test",
    ]);
    const predecessor = JSON.parse(
      command(binary, [
        "task",
        "receipt",
        "--project",
        aid,
        "--task-id",
        "4",
        "--request-id",
        dependencyID,
      ]),
    );
    expect(predecessor.request_id).toBe(dependencyID);
    expect((await get(aid, "/tasks")).tasks).toHaveLength(4);
    expect((await get(bid, "/tasks")).tasks).toHaveLength(2);
    expect(
      (
        await page.request.get(
          origin +
            "/api/v2/application/projects/" +
            aid +
            "/task-batches/" +
            batchID,
        )
      ).status(),
    ).toBe(404);
    facts.cli = { batchReceipt, predecessor };
    const heldState = provider();
    heldState.recovery = { HoldStep: "implement.execute", HoldStream: true };
    writeFileSync(
      join(root, "provider", "state.json"),
      JSON.stringify(heldState),
    );
    const dispatchesBeforeNewTask = provider().dispatches;
    command(binary, [
      "control",
      "--project",
      aid,
      "--action",
      "start_task",
      "--task-id",
      "3",
      "--execution-profile",
      "simulate",
      "--wait",
    ]);
    await expect
      .poll(() => provider().dispatches, { timeout: 120000 })
      .toBe(dispatchesBeforeNewTask + 1);
    await expect
      .poll(() => provider().turns?.[0]?.status, { timeout: 120000 })
      .toBe("inProgress");
    const inputsBefore = provider().dispatches;
    facts.active_at_host_stop = (
      await get(aid, "/snapshot")
    ).connection.snapshot.page.runs.find((run: any) => run.task_id === "3");
    command(binary, ["stop"]);
    const shutdown = JSON.parse(
      readFileSync(join(home, "application-api", "shutdown.json"), "utf8"),
    );
    expect(shutdown.clean).toBe(true);
    expect(shutdown.projects).toHaveLength(2);
    facts.shutdown = shutdown;
    command(binary, ["start", "--no-open"]);
    await open(aid);
    await expect
      .poll(
        async () => {
          const a = await get(aid, "/identity"),
            b = await get(bid, "/identity");
          return (
            a.ready &&
            b.ready &&
            a.instance_id !== originalIdentity.instance_id &&
            b.instance_id !== otherIdentity.instance_id
          );
        },
        { timeout: 180000 },
      )
      .toBe(true);
    const recovery = (
      await get(aid, "/snapshot")
    ).connection.snapshot.page.runs.find((run: any) => run.task_id === "3");
    expect(recovery.recovery_required).toBe(true);
    expect(provider().dispatches).toBe(inputsBefore);
    expect(
      (await get(aid, "/commands/" + requestID)).original.instance_id,
    ).toBe(originalIdentity.instance_id);
    expect(submissions).toBe(1);
    facts.recovery_after_host_restart = recovery;
    facts.restarted_project = await get(aid, "/identity");
    await second.close();
    await page.screenshot({
      path: info.outputPath("offline-project.png"),
      fullPage: true,
    });
    facts.submissions = submissions;
  } catch (error) {
    primaryFailure = error;
    facts.business_error = String(error);
    writeFileSync(
      join(root, "evidence", "web-project-switch.json"),
      JSON.stringify(facts, null, 2),
    );
    throw error;
  } finally {
    failures.push(...(await runtime.stop()));
    try {
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
    } catch (error) {
      failures.push(String(error));
    }
    const inventory = JSON.parse(
      command("powershell.exe", [
        "-NoProfile",
        "-Command",
        `$p=@(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -like '${root.replaceAll("'", "''")}\\*' }); $l=@(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Where-Object { $_.OwningProcess -in $p.ProcessId -or $_.LocalPort -eq ${manifest.web_address.split(":")[1]} }); @{processes=$p.Count;listeners=$l.Count}|ConvertTo-Json -Compress`,
      ]),
    );
    facts.cleanup = inventory;
    let finalShutdown: unknown;
    try {
      finalShutdown = JSON.parse(
        readFileSync(join(home, "application-api", "shutdown.json"), "utf8"),
      );
    } catch (error) {
      failures.push("shutdown evidence unavailable: " + String(error));
    }
    facts.stop_errors = failures;
    writeFileSync(
      join(root, "evidence", "cleanup.json"),
      JSON.stringify(
        { inventory, stop_errors: failures, shutdown: finalShutdown ?? null },
        null,
        2,
      ),
    );
    await info.attach("process-command-turn-evidence", {
      body: JSON.stringify(facts, null, 2),
      contentType: "application/json",
    });
    writeFileSync(
      join(root, "evidence", "web-project-switch.json"),
      JSON.stringify(facts, null, 2),
    );
    if (!primaryFailure) {
      expect(inventory).toEqual({ processes: 0, listeners: 0 });
      expect(failures).toEqual([]);
    }
  }
});
