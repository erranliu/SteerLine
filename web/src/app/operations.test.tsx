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
  expect(screen.queryByText(/超时/)).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "重新查询" })).toBeEnabled();
});
