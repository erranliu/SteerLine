import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { useOriginalRequests } from "./use-original-requests";
import { Api } from "../shared/api/client";
import * as journal from "../shared/api/journal";
import type { Identity, Receipt, StoredRequest } from "../shared/api/types";

afterEach(() => cleanup());
it("lets independent requests finish while one query hangs, and cancels on unmount", async () => {
  const api = new Api(
    { project_id: "project", instance_id: "instance" } as Identity,
    "csrf",
    "stop-hook",
  );
  const rows: StoredRequest[] = [0, 1, 2, 3].map((i) => ({
    envelope: api.envelope("stop_candidate", String(i), 1, { task_id: "18" }),
    created_at: new Date().toISOString(),
  }));
  vi.spyOn(journal, "listRequests").mockResolvedValue(rows);
  const signals: AbortSignal[] = [];
  const reconcile = vi
    .spyOn(api, "reconcile")
    .mockImplementation(async (envelope, signal) => {
      signals.push(signal!);
      if (envelope.request_id === rows[0].envelope.request_id)
        return new Promise(() => {});
      const result = {
        ...envelope,
        original: envelope,
        status: "rejected" as const,
        sampled_at: new Date().toISOString(),
      };
      rows.find(
        (row) => row.envelope.request_id === envelope.request_id,
      )!.response = result;
      return result;
    });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const view = renderHook(() => useOriginalRequests(api, true), { wrapper });
  await waitFor(() => expect(reconcile).toHaveBeenCalledTimes(4));
  expect(
    rows.slice(1).every((row) => row.response?.status === "rejected"),
  ).toBe(true);
  view.unmount();
  expect(signals.every((signal) => signal.aborted)).toBe(true);
  client.clear();
});

it.each(["start_task", "start_candidate"])(
  "keeps %s receipt uncertainty out of the failure banner and observes the original result",
  async (action) => {
    const api = new Api(
      { project_id: "project", instance_id: "instance" } as Identity,
      "csrf",
      "receipt-hook",
    );
    vi.spyOn(journal, "listRequests").mockResolvedValue([]);
    const envelope = api.envelope(action, "101", 1, {});
    const pending: Receipt = {
      ...envelope,
      original: envelope,
      status: "outcome_unknown",
      code: "request_receipt_persist_failed",
      diagnostic: "操作结果待核实，正在查询原请求，请勿重复操作。",
      sampled_at: new Date().toISOString(),
    };
    const submit = vi.spyOn(api, "submit").mockResolvedValue(pending);
    const reconcile = vi
      .spyOn(api, "reconcile")
      .mockRejectedValue(new Error("query unavailable"));
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const view = renderHook(() => useOriginalRequests(api, true), { wrapper });
    await act(async () => {
      expect(await view.result.current.submit(envelope)).toEqual(pending);
    });
    expect(view.result.current.error).toBe("");
    const row: StoredRequest = {
      envelope,
      response: pending,
      created_at: pending.sampled_at,
    };
    act(() => view.result.current.reconcile(row));
    await waitFor(() => expect(reconcile).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(view.result.current.reconciling).toBe(false));
    expect(view.result.current.error).toBe("");
    const applied = {
      ...pending,
      status: "applied" as const,
      code: undefined,
      diagnostic: undefined,
    };
    reconcile.mockResolvedValue(applied);
    vi.mocked(journal.listRequests).mockResolvedValue([
      { ...row, response: applied },
    ]);
    act(() => view.result.current.reconcile(row));
    await waitFor(() =>
      expect(view.result.current.requests.data?.[0]?.response?.status).toBe(
        "applied",
      ),
    );
    expect(view.result.current.error).toBe("");
    expect(submit).toHaveBeenCalledTimes(1);
    expect(
      reconcile.mock.calls.every(([original]) => original === envelope),
    ).toBe(true);
    const rejectedEnvelope = api.envelope(action, "102", 1, {});
    submit.mockResolvedValue({
      ...rejectedEnvelope,
      original: rejectedEnvelope,
      sampled_at: new Date().toISOString(),
      status: "rejected",
      code: "stale_target",
      diagnostic: "task changed",
    });
    await act(async () => {
      await view.result.current.submit(rejectedEnvelope);
    });
    expect(view.result.current.error).toBe("task changed");
    view.unmount();
    client.clear();
  },
);
