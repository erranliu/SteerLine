import { afterEach, expect, it, vi } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";
import { QueryClientProvider, useQuery } from "@tanstack/react-query";
import type { PropsWithChildren } from "react";
import { testClient } from "../test/workspace";
import { useQueryVisibility } from "./query-visibility";

afterEach(cleanup);

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
