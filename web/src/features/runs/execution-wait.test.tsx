import { afterEach, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import type { ExecutionWait } from "../../shared/api/types";
import { ExecutionWaitSummary, preparationWait } from "./execution-wait";
import { run } from "../../test/workspace";

const wait: ExecutionWait = {
  contract: "steerrunner-execution-wait/v1", operation_id: "original-operation", query: "repository_inventory",
  stage: "queue", reason_code: "orca_cli_busy", started_at: "2026-10-25T00:00:00Z",
  updated_at: "2026-10-25T00:00:00Z", deadline_at: "2026-10-25T00:01:30Z",
  calls_started: 0, max_calls: 5, next_attempt_at: null, diagnostic: "Orca CLI channel is busy.",
};
afterEach(() => { cleanup(); vi.useRealTimers(); vi.restoreAllMocks(); });

it("keeps queue waiting beyond the old timeout, uses a monotonic clock and never executes requests", () => {
  vi.useFakeTimers();
  let monotonic = 100;
  vi.spyOn(performance,"now").mockImplementation(() => monotonic);
  const fetch = vi.spyOn(globalThis,"fetch");
  render(<ExecutionWaitSummary wait={wait} sampledAt="2026-10-25T00:00:30Z" />);
  expect(screen.getByText("本次查询已等待 30 秒")).toBeInTheDocument();
  vi.setSystemTime(new Date("2020-01-01"));
  monotonic += 61000;
  // A rerender reads the same anchor without relying on the browser wall clock.
  // Timer execution itself performs no network operation.
  act(() => vi.advanceTimersByTime(1000));
  expect(screen.getByText("本次查询已等待 91 秒")).toBeInTheDocument();
  expect(screen.getByText("等待 Runner 更新")).toBeVisible();
  expect(fetch).not.toHaveBeenCalled();
});

it("does not promote an expired source deadline to an execution failure or operation", () => {
  const fetch = vi.spyOn(globalThis,"fetch");
  render(<ExecutionWaitSummary wait={wait} sampledAt="2026-10-25T00:01:31Z" />);
  expect(screen.getByText("等待 Runner 更新")).toBeVisible();
  expect(screen.getByText("等待 Orca 调用通道")).toBeVisible();
  expect(screen.queryByRole("button")).not.toBeInTheDocument();
  expect(fetch).not.toHaveBeenCalled();
});

it("shows actual read attempts, source timestamps and stale information without a live countdown", () => {
  const backoff: ExecutionWait = {...wait, stage:"backoff", reason_code:"orca_query_transient", calls_started:2, next_attempt_at:"2026-10-25T00:00:35Z"};
  const view = render(<ExecutionWaitSummary wait={backoff} sampledAt="2026-10-25T00:00:30Z" />);
  expect(screen.getByText(/已尝试 2\/5 次，约 5 秒后再次查询/)).toBeVisible();
  view.rerender(<ExecutionWaitSummary wait={backoff} stale sampledAt="2026-10-25T00:00:30Z" />);
  expect(screen.getByText(/最后已知状态/)).toBeVisible();
  expect(screen.queryByText(/约 5 秒后/)).not.toBeInTheDocument();
  expect(view.container.querySelector("time")).toHaveAttribute("datetime",wait.started_at);
});

it("uses absolute source times without a trusted sample and retains exhausted source facts", () => {
  const view = render(<ExecutionWaitSummary wait={wait} />);
  expect(screen.queryByText(/本次查询已等待/)).not.toBeInTheDocument();
  view.rerender(<ExecutionWaitSummary wait={{...wait, stage:"exhausted", reason_code:"query_attempts_exhausted",calls_started:5}} sampledAt="2026-10-25T00:05:00Z" />);
  expect(screen.getByText("查询重试已耗尽")).toBeVisible();
  expect(screen.queryByText("Runner 将自动继续。")).not.toBeInTheDocument();
});

it("does not replace stop, human questions or unknown-operation state with preparation waiting", () => {
  const delivery = { available:true, execution_wait:wait, status:"queued", closed:false, commands:[], controls:{can_reply:false,can_resume:false,can_stop:true,can_message:false,can_interrupt:false,can_close:false,reason:""}, submission:"accepted",revision:1,final_verified:false,resume_required:false,new_inputs_allowed:true };
  const candidate = {...run, participation:"active", task_status:"in_progress",stop_requested:false,decision:undefined,runner_delivery:delivery};
  expect(preparationWait(candidate)).toEqual(wait);
  expect(preparationWait({...candidate,stop_requested:true})).toBeNull();
  expect(preparationWait({...candidate,runner_delivery:{...delivery,status:"reconciling"}})).toBeNull();
  expect(preparationWait({...candidate,runner_delivery:{...delivery,commands:[{command:"reply",request_id:"original",receipt_state:"unknown"}]}})).toBeNull();
  expect(preparationWait({...candidate,runner_delivery:{...delivery,execution_wait:null}})).toBeNull();
  expect(preparationWait({...candidate,participation:"abandoned"})).toBeNull();
});
