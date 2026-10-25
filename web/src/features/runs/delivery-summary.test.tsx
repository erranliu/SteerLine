import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { run } from "../../test/workspace";
import type { Candidate } from "../../shared/api/types";
import { DeliverySummary } from "./delivery-summary";

afterEach(cleanup);
const observation: NonNullable<Candidate["runner_delivery"]> = {
  execution_wait: null,
  available: true,
  run_id: "original",
  revision: 12,
  submission: "accepted",
  step: "publish.verify",
  status: "pr_ready",
  phase: "pr_ready",
  native_outcome: "succeeded",
  final_verified: true,
  pr_state: "open",
  pr_checked_at: "2026-10-21T01:02:03Z",
  closed: false,
  resume_required: false,
  new_inputs_allowed: true,
  controls: {
    can_reply: false,
    can_resume: false,
    can_stop: true,
    can_message: false,
    can_interrupt: false,
    can_close: false,
    reason: "",
  },
  commands: [
    {
      command: "reply",
      request_id: "original-reply",
      receipt_state: "accepted",
      action_status: "completed",
    },
  ],
};

it("separates accepted commands, PR readiness and unsettled resources with no I/O", () => {
  const request = vi.spyOn(globalThis, "fetch");
  const original = { ...run, runner_delivery: structuredClone(observation) };
  const before = JSON.stringify(original);
  const view = render(<DeliverySummary run={original} />);
  expect(screen.getByText("请求受理 · 已受理")).toBeVisible();
  expect(screen.getByText("业务结果 · PR 已就绪")).toBeVisible();
  expect(screen.getByText("资源停止 · 未取得停止证明")).toBeVisible();
  expect(screen.getByText("当前可用操作 · 停止")).toBeVisible();
  expect(screen.getByText(/original-reply/)).toBeInTheDocument();
  expect(view.container.querySelector("time")).toHaveAttribute(
    "datetime",
    observation.pr_checked_at,
  );
  expect(screen.getByText(/PR 缓存观察/)).toHaveTextContent(
    "非实时状态，本页刷新不查询外部 PR",
  );
  view.rerender(<DeliverySummary run={original} />);
  expect(request).not.toHaveBeenCalled();
  expect(JSON.stringify(original)).toBe(before);
  request.mockRestore();
});

it.each([undefined, "provider unavailable"])(
  "does not invent a missing PR query time or refresh a cached result (%s)",
  (pr_query_error) => {
    const request = vi.spyOn(globalThis, "fetch");
    const view = render(
      <DeliverySummary
        run={{
          ...run,
          runner_delivery: {
            ...observation,
            pr_checked_at: undefined,
            pr_query_error,
          },
        }}
      />,
    );
    expect(screen.getByText(/未记录查询时间/)).toBeVisible();
    expect(view.container.querySelector("time")).toBeNull();
    expect(screen.getByText("PR 观察 · 待合并")).toBeVisible();
    if (pr_query_error) {
      expect(screen.getByText(/PR 查询未核实/)).toBeVisible();
    }
    expect(request).not.toHaveBeenCalled();
    request.mockRestore();
  },
);

it("retains the last verified time alongside a failed PR query", () => {
  render(
    <DeliverySummary
      run={{
        ...run,
        runner_delivery: {
          ...observation,
          pr_query_error: "provider unavailable",
        },
      }}
    />,
  );
  expect(screen.getByText(/上次核实于/)).toHaveAttribute(
    "datetime",
    observation.pr_checked_at,
  );
  expect(
    screen.getByText("PR 查询未核实 · provider unavailable"),
  ).toBeVisible();
  expect(screen.getByText(/非实时状态/)).toBeVisible();
});

it.each([
  ["running", "当前步骤 · publish.verify · 执行中"],
  ["waiting_input", "待处理 · 等待明确用户输入。"],
  ["reconciling", "当前步骤 · publish.verify · 对账中"],
  ["cancelled", "业务结果 · 已取消"],
])("renders %s without promoting it to completion", (status, text) => {
  render(
    <DeliverySummary
      run={{ ...run, runner_delivery: { ...observation, status } }}
    />,
  );
  expect(screen.getByText(text)).toBeVisible();
  expect(screen.getByText("资源停止 · 未取得停止证明")).toBeVisible();
});

it("shows generated success independently from cleanup and conversation waiting", () => {
  const view = render(
    <DeliverySummary
      run={{
        ...run,
        runner_delivery: {
          ...observation,
          content_status: "succeeded",
          status: "settling",
          step: "closed",
          closed: true,
        },
      }}
    />,
  );
  expect(screen.getByText("业务结果 · 成功")).toBeVisible();
  expect(screen.getByText("资源停止 · 未取得停止证明")).toBeVisible();
  view.rerender(
    <DeliverySummary
      run={{
        ...run,
        runner_delivery: {
          ...observation,
          status: "waiting_input",
          step: "input",
          controls: {
            ...observation.controls,
            can_stop: false,
            can_message: true,
          },
        },
      }}
    />,
  );
  expect(screen.getByText("当前可用操作 · 发送消息")).toBeVisible();
  expect(screen.getByText("待处理 · 等待明确用户输入。")).toBeVisible();
});

it("keeps missing or conflicting observation unverified", () => {
  render(
    <DeliverySummary
      run={{
        ...run,
        runner_delivery: {
          ...observation,
          available: false,
          diagnostic: "原 Run 身份冲突",
        },
      }}
    />,
  );
  expect(screen.getByText("原 Run 身份冲突")).toBeVisible();
  expect(screen.queryByText(/业务结果/)).not.toBeInTheDocument();
  expect(screen.queryByText(/当前可用操作/)).not.toBeInTheDocument();
});
