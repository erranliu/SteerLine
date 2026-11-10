import { expect, it } from "vitest";
import { run } from "../../test/workspace";
import {
  commandIssues,
  candidateNotices,
  currentStage,
  executionEnded,
  stageLabel,
} from "./presentation";
it("shows the question once while retaining independent diagnostics and deduplicating other copies", () => {
  const repeated = "完整问题\r\n未完成";
  const candidate = {
    ...run,
    error: repeated,
    decision: { request: { question: repeated, context: "不同上下文" } },
    runner_delivery: {
      diagnostic: "完整问题\n未完成",
      controls: { reason: "独立诊断" },
    },
  } as typeof run;
  expect(candidateNotices(candidate)).toEqual(["独立诊断"]);
  expect(candidateNotices(candidate, false)).toEqual([
    "完整问题\n未完成",
    "独立诊断",
  ]);
  expect(candidateNotices({ ...candidate, decision: undefined })).toEqual([
    "完整问题\n未完成",
    "独立诊断",
  ]);
});
it("uses public current steps without inferring topology or parsing messages", () => {
  expect(currentStage(run)).toBe("实施中");
  expect(stageLabel("audit.code.execute")).toBe("审查中");
  expect(stageLabel("publish.verify")).toBe("发布中");
  expect(stageLabel("new-unrecognized-stage")).toBe("阶段待核实");
  expect(stageLabel("implement.report")).toBe("实施中");
  expect(executionEnded({ ...run, participation: "abandoned" })).toBe(false);
  expect(executionEnded({ ...run, execution_status: "unknown" })).toBe(false);
  expect(executionEnded({ ...run, execution_status: "succeeded" })).toBe(true);
});

it("retains command failures and uncertainty independently of a normally running Run", () => {
  const runner_delivery = {
    available: true,
    submission: "accepted",
    status: "running",
    diagnostic: "",
    commands: [
      {
        command: "reply",
        request_id: "hidden-reply",
        receipt_state: "unknown",
      },
      {
        command: "message",
        request_id: "hidden-message",
        receipt_state: "accepted",
        action_status: "failed",
      },
    ],
  } as NonNullable<typeof run.runner_delivery>;
  expect(commandIssues({ ...run, runner_delivery })).toEqual([
    "回答操作结果待核实，请继续核实原操作。",
    "发送消息操作未成功，请核对原操作结果。",
  ]);
  expect(
    commandIssues({
      ...run,
      runner_delivery: { ...runner_delivery, commands: [] },
    }),
  ).toEqual([]);
});
