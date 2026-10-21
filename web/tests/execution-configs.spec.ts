import { test, expect } from "@playwright/test";
import { execFileSync } from "node:child_process";
test("configuration.execution-profiles: global editor, saved and loaded snapshots", async ({
  page,
}, info) => {
  const binary = process.env.STEERLINE_CONFIG_WEB;
  if (
    !binary ||
    !process.env.STEERLINE_HOME ||
    !process.env.STEERLINE_CONFIG_PHASE
  )
    throw new Error("owned controlled scenario driver required");
  const url = new URL(
    execFileSync(binary, ["open", "-print-url"], {
      encoding: "utf8",
      windowsHide: true,
    }).trim(),
  );
  url.pathname = "/settings/execution-configs";
  await page.goto(url.href);
  await expect(page.getByLabel("配置名称")).toBeVisible();
  if (process.env.STEERLINE_CONFIG_PHASE === "edit") {
    for (const agent of ["codex", "pi", "dsh"]) {
      await page.getByRole("button", { name: "复制", exact: true }).click();
      await page.getByLabel("配置名称").fill("浏览器保存的新配置 " + agent);
      await page.getByLabel("执行链路").selectOption("orca-terminal");
      await page.getByLabel("Agent").selectOption(agent);
      await page.getByRole("button", { name: "手动填写", exact: true }).click();
      await page.getByLabel("模型标识").fill("fixture-" + agent);
      await page
        .getByLabel("Thinking effort", { exact: true })
        .selectOption("high");
      if (agent === "codex")
        await page.getByText("用作新任务的默认配置").click();
      await page.getByRole("button", { name: "保存配置", exact: true }).click();
      await expect(page.getByText("配置已保存，待重启生效")).toBeVisible();
      await expect(page.getByText("服务仍使用已加载配置")).toBeVisible();
    }
    await page.screenshot({
      path: info.outputPath("saved-pending.png"),
      fullPage: true,
    });
    await page.reload();
    await expect(page.getByText("配置已保存，待重启生效")).toBeVisible();
  } else {
    await expect(page.getByText("当前服务已加载保存配置")).toBeVisible();
    await expect(page.getByText("配置已保存，待重启生效")).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: /浏览器保存的新配置/ }),
    ).toHaveCount(3);
    await page.screenshot({
      path: info.outputPath("loaded.png"),
      fullPage: true,
    });
    await page.setViewportSize({ width: 375, height: 1000 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  }
});
