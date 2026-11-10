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

it("keeps PR, native completion, command acceptance and cleanup independent without I/O", () => {
  const fetch = vi.spyOn(globalThis, "fetch");
  const original = { ...run, runner_delivery: structuredClone(observation) };
  const before = JSON.stringify(original);
  const view = render(<DeliverySummary run={original} />);
  expect(screen.getByText("待合并")).toBeVisible();
  expect(
    screen.getByText("原生执行 · 本轮成功 · 最终输出已核实"),
  ).toBeVisible();
  expect(screen.getByText("资源停止 · 未取得停止证明")).toBeVisible();
  expect(
    screen.queryByText(/original-reply|观察 revision|当前可用操作/),
  ).toBeNull();
  expect(view.container.querySelector("time")).toHaveAttribute(
    "datetime",
    observation.pr_checked_at,
  );
  expect(screen.getByText(/PR 缓存观察/)).toHaveTextContent("非实时状态");
  expect(screen.queryByText("任务已交付")).toBeNull();
  view.rerender(<DeliverySummary run={original} />);
  expect(fetch).not.toHaveBeenCalled();
  expect(JSON.stringify(original)).toBe(before);
});
it.each([undefined, "provider unavailable"])(
  "does not invent missing PR query time (%s)",
  (pr_query_error) => {
    const fetch = vi.spyOn(globalThis, "fetch");
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
    expect(screen.getByText(/核实时间未知/)).toBeVisible();
    expect(view.container.querySelector("time")).toBeNull();
    expect(screen.getByText("待合并")).toBeVisible();
    if (pr_query_error) expect(screen.getByText(/PR 查询未核实/)).toBeVisible();
    expect(fetch).not.toHaveBeenCalled();
  },
);
it("retains last verified time alongside failed PR queries", () => {
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
});
it.each(["running", "waiting_input", "reconciling", "cancelled"])(
  "does not promote %s to task delivery",
  (status) => {
    render(
      <DeliverySummary
        run={{ ...run, runner_delivery: { ...observation, status } }}
      />,
    );
    expect(screen.queryByText("任务已交付")).toBeNull();
    expect(screen.getByText("资源停止 · 未取得停止证明")).toBeVisible();
  },
);
it("shows generated success independently from cleanup", () => {
  render(
    <DeliverySummary
      run={{
        ...run,
        runner_delivery: {
          ...observation,
          content_status: "succeeded",
          status: "settling",
          closed: true,
        },
      }}
    />,
  );
  expect(screen.getByText("业务结果 · 本轮成功")).toBeVisible();
  expect(screen.getByText("资源停止 · 未取得停止证明")).toBeVisible();
  expect(screen.queryByText("任务已交付")).toBeNull();
});
it("does not consume unavailable Runner observations", () => {
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
  expect(screen.queryByText(/本轮成功|待合并|当前可用操作/)).toBeNull();
  expect(screen.getByText("未核实")).toBeVisible();
});
it("keeps conflicting PR observations unverified and only trusts actual TaskDelivery", () => {
  const original = {
    ...run,
    pr: { state: "merged", url: "https://example.com/pr" },
    runner_delivery: observation,
  };
  const view = render(<DeliverySummary run={original} />);
  expect(screen.getByText("PR 状态待核实")).toBeVisible();
  expect(screen.queryByText("任务已交付")).toBeNull();
  view.rerender(
    <DeliverySummary
      run={{
        ...original,
        runner_delivery: { ...observation, pr_state: "merged" },
      }}
    />,
  );
  expect(screen.getByText("已合并，交付待核实")).toBeVisible();
});
