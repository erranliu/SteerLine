import { expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";

test.use({ actionTimeout: 15_000 });

type Run = {
  run_id: string;
  workspace_identity: string;
  workspace_origin_run_id: string;
  closed?: boolean;
  controls: { can_interrupt: boolean; can_message: boolean };
  execution: {
    session_id: string;
    workspace: string;
    turn_id: string;
    native_outcome: string;
  };
};

for (const kind of ["task", "roadmap"] as const) {
  test(`${kind} can interrupt and continue the same local Codex conversation`, async ({
    page,
  }) => {
    test.setTimeout(360_000);
    const root = resolve(process.env.STEERLINE_ROADMAP_BROWSER_ROOT || ".");
    expect(root.toLowerCase()).toMatch(/^c:\\steerlinedebug\\/);
    const home = join(root, "home");
    const bin = join(root, "bin");
    const native = (file: string, args: string[]) =>
      execFileSync(file, args, {
        env: { ...process.env, STEERLINE_HOME: home },
        encoding: "utf8",
        timeout: 30_000,
        stdio: "pipe",
      }).trim();

    let opening = "";
    await expect
      .poll(
        () => {
          try {
            opening = native(join(bin, "steerline-web.exe"), [
              "open",
              "-print-url",
            ]);
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
    const projectState = join(home, "state");
    const projectDirectory = readdirSync(projectState).find((name) =>
      name.toLowerCase().endsWith(project.slice(0, 12)),
    );
    expect(projectDirectory).toBeTruthy();
    const db = join(projectState, projectDirectory!, "steerline.db");
    const runner = join(bin, "steerrunner.exe");
    const { state_dir } = JSON.parse(
      readFileSync(join(home, "application-api", "runner-build.json"), "utf8"),
    );
    const scope = ["--state-dir", state_dir, "--json"];
    const service = JSON.parse(native(runner, ["service", "status", ...scope]));
    expect(service.ok).toBe(true);
    const binding = [
      ...scope,
      "--expected-instance-id",
      service.data.instance_id,
    ];

    await page.getByRole("link", { name: "任务规划", exact: true }).click();
    await page.getByRole("button", { name: "新建规划" }).click();
    await page.getByLabel("规划类型").selectOption(kind);
    await page
      .getByLabel("名称", { exact: true })
      .fill(`Stop and continue ${kind}`);
    if (kind === "roadmap")
      await page.getByLabel("草稿文件名").fill("interrupt-live.md");
    await page
      .getByLabel("规划目标")
      .fill(
        kind === "task"
          ? "讨论为整数加法补充验收条件。请直接回复任务范围，等我给下一条消息。"
          : "在绑定的 Roadmap 文件中写入最小整数加法草稿并等待下一条消息。",
      );
    await page
      .getByRole("button", {
        name: kind === "task" ? "开始沟通" : "开始规划",
        exact: true,
      })
      .click();
    await expect(page).toHaveURL(/\?task=[a-f0-9]{32}$/);
    const planning = new URL(page.url()).searchParams.get("task")!;
    const streaming = process.env.STEERLINE_PLANNING_STREAM === "1";
    const streamEvents = new Map<
      string,
      { turn_id: string; kind: string; tool?: string; status?: string }
    >();
    if (streaming) {
      page.on("response", async (response) => {
        if (
          !response.ok() ||
          !new URL(response.url()).pathname.endsWith("/conversation-events")
        )
          return;
        try {
          for (const event of (await response.json()).events)
            streamEvents.set(event.id, event);
        } catch {
          // Navigation may close the original browser response.
        }
      });
    }
    const send = page.getByRole("button", { name: "发送", exact: true });
    const message = page.locator(".planning-composer textarea");
    await message.fill("准备续聊");
    await expect(send).toBeEnabled({ timeout: 120_000 });

    const runId = native("python", [
      "-c",
      [
        "import json, pathlib, sqlite3, sys",
        "db = sqlite3.connect(pathlib.Path(sys.argv[1]).as_uri() + '?mode=ro', uri=True)",
        "row = db.execute('SELECT payload FROM roadmap_planning WHERE planning_id=?', (sys.argv[2],)).fetchone()",
        "print(json.loads(row[0])['run']['run_id'])",
      ].join("\n"),
      db,
      planning,
    ]);
    const inspect = (): Run => {
      const result = JSON.parse(
        native(runner, ["inspect", "--run-id", runId, ...binding]),
      );
      expect(result.ok).toBe(true);
      expect(result.data.run.correlation.steerline_planning).toBe(planning);
      return result.data.run;
    };
    let original = inspect();
    await expect
      .poll(
        () => {
          original = inspect();
          return original.controls.can_message;
        },
        { timeout: 120_000 },
      )
      .toBe(true);
    expect(original.execution.session_id).toBeTruthy();
    expect(original.workspace_identity).toBeTruthy();
    expect(original.execution.workspace.toLowerCase()).not.toBe(
      join(root, "project").toLowerCase(),
    );
    expect(existsSync(join(original.execution.workspace, ".git"))).toBe(true);

    const marker = "interrupt-live-marker.txt";
    const token = `resume-${kind}-${planning.slice(0, 8)}`;
    const streamMarker = `Stream check ${planning.slice(0, 8)}`;
    await message.fill(
      (streaming
        ? `先在面向用户的进度消息里写出“${streamMarker}”，说明你接下来将写入验收文件并等待。然后马上调用工具。`
        : "") +
        `这是中断验收。现在先在工作区根目录创建 ${marker}，正文只写 ${token}。写好后立即调用工具，用 PowerShell 执行 Start-Sleep -Seconds ${streaming ? 105 : 75}，等待我停止这一轮。`,
    );
    await send.click();
    const stop = page.getByRole("button", { name: "停止本轮", exact: true });
    await expect(stop).toBeEnabled({ timeout: 120_000 });
    const markerPath = join(original.execution.workspace, marker);
    await expect
      .poll(() => existsSync(markerPath), { timeout: 90_000 })
      .toBe(true);
    const running = inspect();
    expect(running.controls.can_interrupt).toBe(true);
    expect(running.execution.turn_id).not.toBe(original.execution.turn_id);
    if (streaming) {
      const checkLive = () =>
        page.locator(".planning-history article").filter({
          has: page
            .locator("[data-planning-reply]")
            .filter({ hasText: streamMarker }),
        });
      await expect(checkLive()).toHaveCount(1, { timeout: 25_000 });
      await expect
        .poll(
          () => {
            const items = [...streamEvents.values()].filter(
              (event) => event.turn_id === running.execution.turn_id,
            );
            return (
              items.some((event) => event.kind === "assistant_delta") &&
              items.some(
                (event) =>
                  event.kind === "tool" &&
                  event.tool === "commandExecution" &&
                  event.status === "running",
              )
            );
          },
          { timeout: 20_000 },
        )
        .toBe(true);
      await checkLive().locator("summary").click();
      await expect(checkLive().getByText("运行命令 · 进行中")).toBeVisible();
      await page.reload();
      await expect(checkLive()).toHaveCount(1);
      expect(inspect().execution.turn_id).toBe(running.execution.turn_id);
    }
    const receipt = page.waitForResponse(
      (response) =>
        response.url().endsWith("/commands") &&
        response.request().method() === "POST" &&
        response.request().postDataJSON().action ===
          "roadmap_planning_interrupt",
    );
    await stop.click();
    const accepted = await (await receipt).json();
    expect(accepted.status, accepted.diagnostic).toBe("applied");
    await expect(page.getByText("本轮已停止", { exact: true })).toBeVisible({
      timeout: 60_000,
    });
    const stopped = inspect();
    expect(stopped.closed).not.toBe(true);
    expect(stopped.controls.can_message).toBe(true);
    expect(stopped.execution.turn_id).toBe(running.execution.turn_id);
    expect(stopped.execution.native_outcome).toBe("cancelled");
    expect(readFileSync(markerPath, "utf8").trim()).toBe(token);
    await page.reload();
    await expect(page.getByText("本轮已停止", { exact: true })).toBeVisible();

    await message.fill(`读取刚才留在原工作区的 ${marker}，直接回复它的全文。`);
    await send.click();
    await expect(
      page
        .locator(".planning-history article")
        .filter({
          hasText: "SteerAgent",
        })
        .getByText(token, { exact: true }),
    ).toBeVisible({ timeout: 120_000 });
    const continued = inspect();
    expect(continued.execution.turn_id).not.toBe(stopped.execution.turn_id);
    for (const run of [running, stopped, continued]) {
      expect(run.run_id).toBe(original.run_id);
      expect(run.execution.session_id).toBe(original.execution.session_id);
      expect(run.execution.workspace).toBe(original.execution.workspace);
      expect(run.workspace_identity).toBe(original.workspace_identity);
      expect(run.workspace_origin_run_id).toBe(
        original.workspace_origin_run_id,
      );
    }
    const facts = {
      kind,
      planning,
      run_id: runId,
      session_id: original.execution.session_id,
      workspace: original.execution.workspace,
      workspace_identity: original.workspace_identity,
      stopped_turn: stopped.execution.turn_id,
      outcome: stopped.execution.native_outcome,
      continued_turn: continued.execution.turn_id,
      streaming,
      stream_event_kinds: [
        ...new Set([...streamEvents.values()].map((event) => event.kind)),
      ],
    };
    writeFileSync(
      join(root, "evidence", `interrupt-${kind}.json`),
      JSON.stringify(facts, null, 2),
    );
  });
}
