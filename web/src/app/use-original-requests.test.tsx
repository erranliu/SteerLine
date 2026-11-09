import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { useOriginalRequests } from "./use-original-requests";
import { Api } from "../shared/api/client";
import * as journal from "../shared/api/journal";
import type { Identity, Receipt, StoredRequest } from "../shared/api/types";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
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

it("exposes in-flight identities synchronously and retains applied receipts across journal refresh gaps", async () => {
  const api = new Api(
    { project_id: "project", instance_id: "instance" } as Identity,
    "csrf",
    "feedback",
  );
  vi.spyOn(journal, "listRequests").mockResolvedValue([]);
  vi.spyOn(journal, "requestByID").mockResolvedValue(undefined);
  let finish!: (receipt: Receipt) => void;
  const send = vi.spyOn(api, "submit").mockImplementation(
    () =>
      new Promise<Receipt>((resolve) => {
        finish = resolve;
      }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const view = renderHook(() => useOriginalRequests(api, true), { wrapper });
  const envelope = api.envelope("roadmap_parse", "draft", 1, {});
  let result!: Promise<Receipt | undefined>;
  act(() => {
    result = view.result.current.submit(envelope);
    expect(view.result.current.pendingRequests()).toEqual([envelope]);
  });
  await waitFor(() => expect(send).toHaveBeenCalledTimes(1));
  await waitFor(() =>
    expect(view.result.current.observedRequests[0]?.envelope).toEqual(envelope),
  );
  const receipt: Receipt = {
    ...envelope,
    status: "applied",
    sampled_at: new Date().toISOString(),
    roadmap: { id: "new-record", revision: 1 },
  };
  await act(async () => {
    finish(receipt);
    await result;
  });
  await waitFor(() =>
    expect(view.result.current.pendingRequests()).toEqual([]),
  );
  expect(view.result.current.observedRequests[0]?.response).toEqual(receipt);
  await act(async () => {
    await view.result.current.requests.refetch();
  });
  expect(view.result.current.observedRequests[0]?.response).toEqual(receipt);
  expect(send).toHaveBeenCalledTimes(1);
  view.unmount();
  client.clear();
});

it.each([false, true])(
  "distinguishes proven unsent from unreadable journal after a failed submit (unreadable=%s)",
  async (unreadable) => {
    const api = new Api(
      { project_id: "project", instance_id: "instance" } as Identity,
      "csrf",
      "failure",
    );
    vi.spyOn(journal, "listRequests").mockResolvedValue([]);
    const lookup = vi.spyOn(journal, "requestByID");
    if (unreadable) lookup.mockRejectedValue(new Error("journal unreadable"));
    else lookup.mockResolvedValue(undefined);
    const send = vi
      .spyOn(api, "submit")
      .mockRejectedValue(new Error("transport lost"));
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const view = renderHook(() => useOriginalRequests(api, true), { wrapper });
    await act(async () => {
      await view.result.current.submit(
        api.envelope("roadmap_parse", "draft", 1, {}),
      );
    });
    await waitFor(() =>
      expect(view.result.current.pendingRequests()).toHaveLength(0),
    );
    await waitFor(() =>
      expect(view.result.current.observedRequests).toHaveLength(
        unreadable ? 1 : 0,
      ),
    );
    if (unreadable)
      expect(view.result.current.observedRequests[0].error).toContain(
        "transport lost",
      );
    expect(send).toHaveBeenCalledTimes(1);
    view.unmount();
    client.clear();
  },
);

it("isolates late old-instance rejection from the new workspace", async () => {
  const first = new Api(
    { project_id: "project", instance_id: "first" } as Identity,
    "csrf",
    "switch",
  );
  const second = new Api(
    { project_id: "project", instance_id: "second" } as Identity,
    "csrf",
    "switch",
  );
  vi.spyOn(journal, "listRequests").mockResolvedValue([]);
  vi.spyOn(journal, "requestByID").mockResolvedValue(undefined);
  let finish!: (receipt: Receipt) => void;
  vi.spyOn(first, "submit").mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const view = renderHook(({ api }) => useOriginalRequests(api, true), {
    wrapper,
    initialProps: { api: first },
  });
  const envelope = first.envelope("roadmap_parse", "draft", 1, {});
  let result!: Promise<Receipt | undefined>;
  act(() => {
    result = view.result.current.submit(envelope);
  });
  await waitFor(() => expect(finish).toBeTypeOf("function"));
  view.rerender({ api: second });
  await act(async () => {
    finish({
      ...envelope,
      status: "rejected",
      diagnostic: "old failure",
      sampled_at: new Date().toISOString(),
    });
    await result;
  });
  expect(view.result.current.error).toBe("");
  expect(view.result.current.pendingRequests()).toEqual([]);
  expect(view.result.current.observedRequests).toEqual([]);
  view.unmount();
  client.clear();
});

it("keeps a late environment journal error out of a switched workspace", async () => {
  const first = new Api(
    { project_id: "project", instance_id: "first" } as Identity,
    "csrf",
    "late-journal",
  );
  const second = new Api(
    { project_id: "project", instance_id: "second" } as Identity,
    "csrf",
    "late-journal",
  );
  vi.spyOn(journal, "listRequests").mockResolvedValue([]);
  let finish!: (row: undefined) => void;
  const lookup = vi
    .spyOn(journal, "requestByID")
    .mockResolvedValue(undefined)
    .mockResolvedValueOnce(undefined)
    .mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
  vi.spyOn(first, "submit").mockRejectedValue(new Error("old storage error"));
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const view = renderHook(({ api }) => useOriginalRequests(api, true), {
    wrapper,
    initialProps: { api: first },
  });
  let result!: Promise<Receipt | undefined>;
  act(() => {
    result = view.result.current.submit(
      first.envelope("environment_start", "project", 1, {}),
    );
  });
  await waitFor(() => expect(lookup).toHaveBeenCalledTimes(2));
  view.rerender({ api: second });
  await act(async () => {
    finish(undefined);
    await result;
  });
  expect(view.result.current.error).toBe("");
  expect(view.result.current.pendingRequests()).toEqual([]);
  expect(view.result.current.observedRequests).toEqual([]);
  view.unmount();
  client.clear();
});
