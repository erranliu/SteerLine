import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { useOriginalRequests } from "./use-original-requests";
import { Api } from "../shared/api/client";
import * as journal from "../shared/api/journal";
import type { Identity, StoredRequest } from "../shared/api/types";

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
