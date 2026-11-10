import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import {
  CopyExecutionInfo,
  formatExecutionInfo,
  requestExecutionInfo,
} from "./execution-info";
import { requestDestination } from "./model";
import type { Envelope, Receipt } from "../../shared/api/types";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("merges identical IDs and copies only labelled identities", () => {
  expect(
    formatExecutionInfo({
      title: "Roadmap 解析",
      project: "C:\\Projects\\SteerlineTest",
      requestID: "original",
      record: { id: "original", label: "解析记录" },
      runner: { run_id: "runner", attempt_id: "attempt" },
    }),
  ).toBe(
    "SteerlineTest · Roadmap 解析\n请求 / 解析记录 ID：original（本次提交及结果）\nRunner Run ID：runner（底层运行）\nAttempt ID：attempt（具体执行轮次）",
  );
});
it("copies local publication as its own request without borrowing the generation Run", () => {
  const envelope = {
    project_id: "project",
    action: "roadmap_publish",
    target_id: "record",
    request_id: "publish-request",
  } as Envelope;
  const receipt = {
    roadmap: { id: "record", run_id: "generation-run" },
  } as Receipt;
  const text = formatExecutionInfo(requestExecutionInfo(envelope, receipt));
  expect(text).toContain("请求 ID：publish-request");
  expect(text).toContain("Roadmap 记录 ID：record");
  expect(text).not.toContain("Runner");
  expect(text).not.toContain("generation-run");
  expect(requestDestination(envelope, receipt)).toBe(
    "/projects/project/roadmaps?record=record&request_id=publish-request",
  );
});
it("distinguishes an uncreated Runner from an operation that does not use Runner", () => {
  expect(
    formatExecutionInfo({ title: "解析", requestID: "pending", runner: {} }),
  ).toContain("Runner 运行尚未创建");
  expect(
    formatExecutionInfo({ title: "入图", requestID: "local" }),
  ).not.toContain("Runner");
});
it("keeps the exact selected identities available for manual copying after clipboard failure", async () => {
  vi.stubGlobal("navigator", {
    clipboard: { writeText: vi.fn().mockRejectedValue(new Error("denied")) },
  });
  render(
    <CopyExecutionInfo
      info={{
        title: "历史检测",
        requestID: "old",
        runner: { run_id: "old-run" },
      }}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "复制执行信息" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("old-run");
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal("navigator", { clipboard: { writeText } });
  fireEvent.click(screen.getByRole("button", { name: "复制执行信息" }));
  expect(await screen.findByRole("status")).toHaveTextContent("已复制执行信息");
  expect(writeText.mock.calls[0][0]).toContain("请求 ID：old");
});
