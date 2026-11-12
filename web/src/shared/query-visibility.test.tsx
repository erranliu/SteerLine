import { afterEach, expect, it, vi } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";
import { QueryClientProvider, useQuery } from "@tanstack/react-query";
import type { PropsWithChildren } from "react";
import { testClient } from "../test/workspace";
import { createWebQueryClient } from "./query-client";
import { useQueryVisibility } from "./query-visibility";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

it("gates visibility and cancels only its query on hiding, disable and unmount", async () => {
  const visibility = vi
    .spyOn(document, "visibilityState", "get")
    .mockReturnValue("hidden");
  const subscribe = vi.spyOn(document, "addEventListener");
  const unsubscribe = vi.spyOn(document, "removeEventListener");
  const show = (state: DocumentVisibilityState) =>
    act(() => {
      visibility.mockReturnValue(state);
      document.dispatchEvent(new Event("visibilitychange"));
    });
  const client = testClient();
  const calls: AbortSignal[] = [];
  const unrelated = client.prefetchQuery({
    queryKey: ["other"],
    queryFn: () => new Promise(() => {}),
  });
  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const { rerender, unmount } = renderHook(
    (online) => {
      const queryKey = ["selected"];
      const enabled = useQueryVisibility(online, queryKey);
      return useQuery({
        queryKey,
        enabled,
        queryFn: ({ signal }) => {
          calls.push(signal);
          return new Promise<string>(() => {});
        },
      });
    },
    { wrapper, initialProps: true },
  );
  expect(calls).toHaveLength(0);
  show("visible");
  expect(calls).toHaveLength(1);
  show("hidden");
  expect(calls[0].aborted).toBe(true);
  show("visible");
  expect(calls).toHaveLength(2);
  rerender(false);
  expect(calls[1].aborted).toBe(true);
  rerender(true);
  expect(calls).toHaveLength(3);
  unmount();
  expect(unsubscribe).toHaveBeenCalledWith(
    "visibilitychange",
    subscribe.mock.calls.find(([event]) => event === "visibilitychange")![1],
  );
  expect(calls[2].aborted).toBe(true);
  expect(client.getQueryState(["other"])?.fetchStatus).toBe("fetching");
  client.clear();
  await unrelated;
});

it("pauses background polling and refreshes active queries on return before the next interval", async () => {
  vi.useFakeTimers();
  const visibility = vi
    .spyOn(document, "visibilityState", "get")
    .mockReturnValue("visible");
  const show = (state: DocumentVisibilityState) =>
    act(() => {
      visibility.mockReturnValue(state);
      document.dispatchEvent(new Event("visibilitychange", { bubbles: true }));
    });
  const client = createWebQueryClient();
  const read = vi.fn(async () => "current");
  const readGated = vi.fn(async () => "gated");
  const readSnapshot = vi.fn(async () => "snapshot");
  const readInactive = vi.fn(async () => "inactive");
  await client.prefetchQuery({ queryKey: ["inactive"], queryFn: readInactive });
  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  renderHook(
    () => {
      useQuery({ queryKey: ["current"], queryFn: read, refetchInterval: 5000 });
      const key = ["gated"];
      const enabled = useQueryVisibility(true, key);
      useQuery({
        queryKey: key,
        queryFn: readGated,
        enabled,
        staleTime: 60000,
        refetchInterval: 5000,
      });
      useQuery({
        queryKey: ["snapshot"],
        queryFn: readSnapshot,
        refetchOnWindowFocus: false,
      });
    },
    { wrapper },
  );
  await act(() => vi.advanceTimersByTimeAsync(0));
  show("hidden");
  await act(() => vi.advanceTimersByTimeAsync(15000));
  expect(read).toHaveBeenCalledTimes(1);
  expect(readGated).toHaveBeenCalledTimes(1);

  show("visible");
  await act(() => vi.advanceTimersByTimeAsync(0));
  expect(read).toHaveBeenCalledTimes(2);
  expect(readGated).toHaveBeenCalledTimes(2);
  expect(readSnapshot).toHaveBeenCalledTimes(1);
  expect(readInactive).toHaveBeenCalledTimes(1);
  cleanup();
  client.clear();
});
