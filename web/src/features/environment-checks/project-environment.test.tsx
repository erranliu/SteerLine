import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, testClient } from "../../test/workspace";
import { ProjectEnvironment } from "./project-environment";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function fixture(online = true, send = vi.fn().mockResolvedValue(true), emptyHistory = false) {
  const api = new Api(identity, "csrf", "home");
  const round = {
    number: 1,
    deadline: "2026-10-26T10:00:00Z",
    run_id: "original-run",
    settled: false,
    timed_out: false,
    diagnostic: "等待原执行收尾",
    run: {
      summary: "缺少必需工具",
      diagnostic: "",
      agent_reply: "请补充远端凭据引用",
      agent_reply_verified: true,
    },
    progress: {
      version: 9,
      collection_status: "observing",
      content_base64: btoa('{"pr":true}'),
    },
    result: {
      version: 0,
      collection_status: "pending",
    },
  };
  const record = {
    id: "a".repeat(32),
    revision: 5,
    kind: "adaptation",
    profile_id: "remote",
    state: "running",
    status: "not_ready",
    settled: false,
    diagnostic: "",
    rounds: [round],
  };
  vi.spyOn(api, "get").mockImplementation(
    async (path) =>
      (path === "/environment/options"
        ? {
            profiles: [
              {
                id: "remote",
                name: "远端一",
                status: "not_ready",
                stage: "blocked",
                environment_key: "e".repeat(64),
                current_revision: 2,
                reason: "缺少工具",
              },
            ],
            environments: [
              {
                environment_key: "e".repeat(64),
                name: "远端一",
                status: "not_ready",
                stage: "blocked",
                reason: "缺少工具",
                current_revision: 2,
                document_path: "SteerlineFlow/environment.md",
                activity_id: record.id,
                profiles: [{ id: "remote", name: "远端一" }],
              },
            ],
            generators: [{ id: "local", name: "本地生成" }],
          }
        : path === "/environment"
          ? { records: emptyHistory ? [] : [{ ...record, rounds: [] }] }
          : record) as never,
  );
  mountFeature(<ProjectEnvironment />, api, testClient(), online, { send });
  return { send, record };
}
it("keeps progress separate from binary admission and stops the exact original activity", async () => {
  const { send, record } = fixture();
  expect(await screen.findByText("远端一 · 阻断")).toBeVisible();
  fireEvent.click(await screen.findByRole("button", { name: "查看活动详情" }));
  expect(await screen.findByText(/Run original-run/)).toBeVisible();
  expect(screen.getByText(/进度文件（不作为验收结论）/)).toBeVisible();
  expect(screen.getByText(/验收结果文件/)).toBeVisible();
  expect(screen.getByText(/停止后封存一次/)).toBeVisible();
  expect(screen.getByText("原执行说明：缺少必需工具")).toBeVisible();
  expect(
    screen.getByText(/Agent 回复（原生来源已核实，仅供排障）/),
  ).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "停止原活动" }));
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "environment_stop",
      record.id,
      record.revision,
      {},
    ),
  );
});
it("shows a final collection failure without promoting binary admission", async () => {
  const { record } = fixture();
  Object.assign(record.rounds[0].result, {
    version: 0,
    collection_status: "closed",
    diagnostic: "file_missing",
  });
  fireEvent.click(await screen.findByRole("button", { name: "查看活动详情" }));
  fireEvent.click(await screen.findByText(/验收结果文件/));
  expect(await screen.findByText("结果采集诊断：file_missing")).toBeVisible();
  expect(screen.getByText("远端一 · 阻断")).toBeVisible();
});
it("freezes the selected profile and timeout and prevents duplicate clicks", async () => {
  let finish!: (v: boolean) => void;
  const send = vi.fn().mockImplementation(
    () =>
      new Promise<boolean>((resolve) => {
        finish = resolve;
      }),
  );
  fixture(true, send);
  await screen.findByText("远端一 · 阻断");
  fireEvent.change(screen.getByLabelText("验收执行方案"), {
    target: { value: "remote" },
  });
  fireEvent.change(screen.getByLabelText("每轮总超时（秒）"), {
    target: { value: "120" },
  });
  const start = screen.getByRole("button", { name: "2. 开始验收 / 重新准备" });
  fireEvent.click(start);
  fireEvent.click(start);
  expect(send).toHaveBeenCalledTimes(1);
  expect(send).toHaveBeenCalledWith(
    "environment_start",
    identity.project_id,
    1,
    {
      profile_id: "remote",
      round_timeout_seconds: 120,
      expected_current_revision: 2,
    },
  );
  finish(true);
});
it("keeps history readable while offline and disables mutations", async () => {
  fixture(false);
  fireEvent.click(await screen.findByRole("button", { name: "查看活动详情" }));
  await screen.findByText(/Run original-run/);
  expect(
    screen.getByRole("button", { name: "2. 开始验收 / 重新准备" }),
  ).toBeDisabled();
  expect(screen.getByRole("button", { name: "生成文档和脚本" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "停止原活动" })).toBeDisabled();
});

it("shows the original intranet PR link without promoting an unfinished activity", async () => {
  const { record } = fixture();
  Object.assign(record.rounds[0], {
    report: {
      completed: true,
      checks: [
        { id: "git", status: "passed", message: "Git available" },
        {
          id: "unity.editor",
          status: "failed",
          message: "UNITY_EDITOR_PATH is not set",
        },
      ],
      message: "等待收尾",
      l2: [{ test_id: "one", status: "passed", message: "" }],
      pr: {
        url: "http://gitea.local/pulls/1",
        head_revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        cleanup_completed: true,
        outcome_unknown: false,
      },
    },
  });
  fireEvent.click(await screen.findByRole("button", { name: "查看活动详情" }));
  expect(
    await screen.findByRole("link", { name: "http://gitea.local/pulls/1" }),
  ).toHaveAttribute("href", "http://gitea.local/pulls/1");
  expect(screen.getByText("远端一 · 阻断")).toBeVisible();
  expect(screen.getByText("git · 通过 · Git available")).toBeVisible();
  expect(
    screen.getByText("unity.editor · 不通过 · UNITY_EDITOR_PATH is not set"),
  ).toBeVisible();
});

it("exports the script under its declared path even when prose mentions another interpreter", async () => {
  const OriginalURL = URL;
  vi.stubGlobal(
    "URL",
    class extends OriginalURL {
      static createObjectURL() {
        return "blob:fixture";
      }
      static revokeObjectURL() {}
    },
  );
  const names: string[] = [];
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
    this: HTMLAnchorElement,
  ) {
    names.push(this.download);
  });
  const { record } = fixture();
  Object.assign(record, {
    kind: "generation",
    state: "generated",
    settled: true,
    profile_id: "local",
    generated: {
      document:
        'PowerShell verify-environment.ps1 is not selected.\n```steerline-environment\n{"version":1,"script":"scripts/verify-environment.sh","interpreter":"bash","validation_base":"validation","dependencies":[]}\n```',
      script: "#!/usr/bin/env bash\n",
      gaps: ["fixture only"],
      script_path: "scripts/verify-environment.sh",
    },
  });
  fireEvent.click(await screen.findByRole("button", { name: "查看活动详情" }));
  fireEvent.click(await screen.findByRole("button", { name: "导出验收脚本" }));
  expect(names).toEqual(["verify-environment.sh"]);
});

it("shows the selected current report even outside the recent history window", async () => {
 const {send,record}=fixture(true,vi.fn().mockResolvedValue(true),true);
 fireEvent.click(await screen.findByRole("button",{name:"查看当前验收"}));
 expect(await screen.findByText(/Run original-run/)).toBeVisible();
 fireEvent.click(screen.getByRole("button",{name:"停止原活动"}));
 await waitFor(()=>expect(send).toHaveBeenCalledWith("environment_stop",record.id,record.revision,{}));
});

it("invalidates only the selected project context with its displayed current revision",async()=>{
 const {send}=fixture();
 await screen.findByText("远端一 · 阻断");
 fireEvent.change(screen.getByLabelText("验收执行方案"),{target:{value:"remote"}});
 fireEvent.change(screen.getByLabelText("环境失效原因"),{target:{value:"机器已重建"}});
 fireEvent.click(screen.getByRole("button",{name:"标记当前项目环境失效"}));
 await waitFor(()=>expect(send).toHaveBeenCalledWith("environment_invalidate",identity.project_id,1,{environment_key:"e".repeat(64),expected_current_revision:2,reason:"机器已重建"}));
});
