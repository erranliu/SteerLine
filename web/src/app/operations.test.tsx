import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render as renderUI,
  screen,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";
import { Operations } from "./operations";
import type { Receipt, StoredRequest } from "../shared/api/types";

afterEach(cleanup);
const render = (ui: ReactNode) => renderUI(ui, { wrapper: MemoryRouter });
function request(
  action: string,
  status?: Receipt["status"],
  created_at = "2026-11-02T06:30:00Z",
): StoredRequest {
  const envelope = {
    request_id: `opaque-request-${action}`,
    project_id: "project",
    instance_id: "opaque-instance",
    action,
    target_id: "opaque-target",
    expected_version: 7,
    payload: {},
  };
  return {
    envelope,
    created_at,
    error: "opaque transport detail",
    checked_at: created_at,
    response: status
      ? {
          ...envelope,
          status,
          sampled_at: created_at,
          diagnostic: "opaque server detail",
        }
      : undefined,
  };
}
it("shows only the latest concise environment result and preserves the input records", () => {
  const requests = [
    request("environment_stop", "applied", "2026-11-02T07:30:00Z"),
    request("environment_start", "applied"),
    request("start_task", "rejected"),
  ];
  const original = structuredClone(requests);
  render(
    <Operations
      requests={requests}
      compact
      online
      reconcile={vi.fn()}
      reconciling={false}
    />,
  );
  expect(screen.getByText("停止检测：操作成功")).toBeVisible();
  expect(screen.queryByText(/开始检测/)).not.toBeInTheDocument();
  expect(screen.queryByText(/opaque/)).not.toBeInTheDocument();
  expect(screen.queryByText(/start_task|操作回执/)).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "复制执行信息" })).toBeEnabled();
  expect(screen.getByRole("link", { name: "查看对应记录" })).toHaveAttribute(
    "href",
    "/projects/project/environment?activity=opaque-target&request_id=opaque-request-environment_stop",
  );
  expect(requests).toEqual(original);
});
it("reconciles an uncertain operation using its exact original record", () => {
  const row = request("environment_stop", "outcome_unknown");
  const reconcile = vi.fn();
  const { rerender } = render(
    <Operations
      requests={[row]}
      compact
      online
      reconcile={reconcile}
      reconciling={false}
    />,
  );
  expect(screen.getByText("停止检测：结果待核实")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "重新查询" }));
  expect(reconcile).toHaveBeenCalledWith(row);
  rerender(
    <Operations
      requests={[row]}
      compact
      online={false}
      reconcile={reconcile}
      reconciling={false}
    />,
  );
  expect(screen.getByRole("button", { name: "重新查询" })).toBeDisabled();
  rerender(
    <Operations
      requests={[row]}
      compact
      online
      reconcile={reconcile}
      reconciling
    />,
  );
  expect(screen.getByRole("button", { name: "重新查询" })).toBeDisabled();
});
it("keeps older unresolved operations visible alongside the latest result", () => {
  render(
    <Operations
      requests={[
        request("environment_start", "accepted"),
        request("environment_stop", "rejected", "2026-11-02T07:30:00Z"),
      ]}
      compact
      online
      reconcile={vi.fn()}
      reconciling={false}
    />,
  );
  expect(screen.getByText("开始检测：正在确认启动")).toBeVisible();
  expect(screen.getByText("停止检测：操作未成功，请刷新后重试")).toBeVisible();
  expect(screen.getAllByRole("button", { name: "重新查询" })).toHaveLength(1);
});

it.each(["rejected"] as const)(
  "shows the specific environment diagnostic for %s without changing the request",
  (status) => {
    const row = request("environment_start", status);
    row.response!.diagnostic =
      "environment.md requires one steerline-environment JSON block";
    const before = structuredClone(row);
    render(
      <Operations
        requests={[row]}
        compact
        online
        reconcile={vi.fn()}
        reconciling={false}
      />,
    );
    expect(screen.getByText(row.response!.diagnostic)).toBeVisible();
    expect(screen.queryByText(row.error!)).not.toBeInTheDocument();
    expect(row).toEqual(before);
  },
);

it("keeps confirming a start whose receipt query timed out", () => {
  const row = request("environment_start");
  row.error = "TimeoutError: 本次请求核实超时，保留原请求";
  render(
    <Operations
      requests={[row]}
      compact
      online
      reconcile={vi.fn()}
      reconciling={false}
    />,
  );
  expect(screen.getByText("开始检测：正在确认启动")).toBeVisible();
  expect(screen.getByText(/超时/)).toBeVisible();
  expect(screen.getByRole("button", { name: "重新查询" })).toBeEnabled();
});

it("names concurrent uncertain actions and binds their trusted business context", () => {
  const stop = request("stop_candidate", "outcome_unknown"),
    answer = request("answer_decision", "outcome_unknown"),
    cancel = request("main_agent_cancel", "outcome_unknown");
  stop.envelope.payload = { task_id: "42" };
  answer.envelope.target_id = "question";
  const reconcile = vi.fn();
  const candidate = {
    run_id: "candidate",
    task_id: "43",
    domain_task_id: "43",
    candidate_number: 2,
    decision: { id: "question" },
  } as never;
  const before = structuredClone([stop, answer, cancel]);
  render(
    <Operations
      requests={[stop, answer, cancel]}
      candidates={[candidate]}
      scope={{ project_id: "project", instance_id: "opaque-instance" }}
      online
      reconcile={reconcile}
      reconciling={false}
    />,
  );
  expect(screen.getByText("停止候选参与")).toBeVisible();
  expect(screen.getByText("回答问题")).toBeVisible();
  expect(screen.getByText("取消提示词建议生成")).toBeVisible();
  expect(screen.getByText("任务 #42 · 候选待核实")).toBeVisible();
  expect(screen.getByText("任务 #43 · 候选 C2")).toBeVisible();
  fireEvent.click(screen.getAllByRole("button", { name: "查询原请求" })[1]);
  expect(reconcile).toHaveBeenCalledWith(answer);
  fireEvent.click(screen.getAllByRole("button", { name: "查询原请求" })[2]);
  expect(reconcile).toHaveBeenLastCalledWith(cancel);
  expect([stop, answer, cancel]).toEqual(before);
});
it("bounds rejected history and keeps only current unresolved requests in the primary view", () => {
  const old = Array.from({ length: 20 }, (_, i) => {
    const r = request(
      "stop_candidate",
      "rejected",
      `2026-11-01T00:00:${String(i).padStart(2, "0")}Z`,
    );
    r.envelope.request_id += i;
    r.response!.diagnostic = `历史拒绝 ${i}`;
    return r;
  });
  const applied = request("start_task", "applied", "2026-11-10T00:00:00Z"),
    pending = request(
      "set_dispatch_mode",
      "outcome_unknown",
      "2026-11-10T00:00:01Z",
    ),
    foreign = request(
      "stop_service",
      "outcome_unknown",
      "2026-11-10T00:00:02Z",
    );
  foreign.envelope.instance_id = "old-instance";
  render(
    <Operations
      requests={[...old, applied, pending, foreign]}
      scope={{ project_id: "project", instance_id: "opaque-instance" }}
      online
      reconcile={vi.fn()}
      reconciling={false}
    />,
  );
  expect(document.querySelectorAll(".operations > .operation")).toHaveLength(1);
  expect(screen.getByText("切换自动派发")).toBeVisible();
  expect(screen.getByText("历史拒绝 19")).not.toBeVisible();
  expect(screen.queryByText("stop_service")).not.toBeInTheDocument();
  expect(document.querySelectorAll(".operations .operation")).toHaveLength(13);
  fireEvent.click(screen.getByText("操作回执 · 最近 12 项"));
  expect(screen.getByText("历史拒绝 19")).toBeVisible();
});
