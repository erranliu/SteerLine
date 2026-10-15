import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { projectQueries } from "../../shared/api/queries";
import { identity, mountFeature, run, testClient } from "../../test/workspace";
import { LastMessage } from "./last-message";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});
const at = "2026-10-15T10:00:00Z";
function view(
  execution = run.execution_id,
  attempt = run.attempt_id,
  text = "正在检查代码",
) {
  return {
    run_id: execution,
    attempt_id: attempt,
    message: {
      run_id: execution,
      attempt_id: attempt,
      session_id: "session",
      turn_id: "turn",
      kind: "assistant",
      text,
      observed_at: at,
    },
  };
}

it("polls only the selected visible execution, preserves source time, and retains a sample on error", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-15T10:01:00Z"));
  const visibility = vi
    .spyOn(document, "visibilityState", "get")
    .mockReturnValue("visible");
  const client = testClient();
  const api = new Api(identity, "csrf", "home");
  const get = vi.spyOn(api, "get").mockResolvedValue(view());
  const mounted = mountFeature(<LastMessage run={run} />, api, client);
  await act(() => vi.advanceTimersByTimeAsync(1));
  expect(screen.getByText("正在检查代码")).toBeVisible();
  const timestamp = screen.getByText(/观察于/).textContent?.split(" · ")[0];
  await act(() => vi.advanceTimersByTimeAsync(6100));
  expect(get).toHaveBeenCalledTimes(3);
  expect(screen.getByText(/观察于/).textContent?.split(" · ")[0]).toBe(
    timestamp,
  );
  get.mockRejectedValue(new Error("暂不可用"));
  await act(() => vi.advanceTimersByTimeAsync(3000));
  expect(screen.getByText("正在检查代码")).toBeVisible();
  expect(screen.getByText(/消息读取失败/)).toBeVisible();
  visibility.mockReturnValue("hidden");
  fireEvent(document, new Event("visibilitychange"));
  const count = get.mock.calls.length;
  await act(() => vi.advanceTimersByTimeAsync(10000));
  expect(get).toHaveBeenCalledTimes(count);
  visibility.mockReturnValue("visible");
  fireEvent(document, new Event("visibilitychange"));
  await act(() => vi.advanceTimersByTimeAsync(3100));
  expect(get.mock.calls.length).toBeGreaterThan(count);
  mounted.unmount();
  const stopped = get.mock.calls.length;
  await act(() => vi.advanceTimersByTimeAsync(10000));
  expect(get).toHaveBeenCalledTimes(stopped);
  client.clear();
});

it("cancels old requests on selection and backgrounding; late responses cannot replace the selected message", async () => {
  vi.useFakeTimers();
  const visibility = vi
    .spyOn(document, "visibilityState", "get")
    .mockReturnValue("visible");
  const api = new Api(identity, "csrf", "home");
  const client = testClient();
  let finishOld!: (value: unknown) => void;
  let oldSignal: AbortSignal | undefined;
  let newSignal: AbortSignal | undefined;
  const get = vi.spyOn(api, "get").mockImplementation((path, signal) => {
    if (path.includes("execution_id=execution")) {
      oldSignal = signal;
      return new Promise((resolve) => {
        finishOld = resolve;
      }) as never;
    }
    newSignal = signal;
    return new Promise(() => {}) as never;
  });
  function Selected() {
    const [next, setNext] = useState(false);
    const target = next
      ? {
          ...run,
          run_id: "next-candidate",
          execution_id: "next",
          attempt_id: "next-attempt",
        }
      : run;
    return (
      <>
        <button onClick={() => setNext(true)}>切换执行</button>
        <LastMessage key={target.run_id} run={target} />
      </>
    );
  }
  const mounted = mountFeature(<Selected />, api, client);
  await act(() => vi.advanceTimersByTimeAsync(1));
  fireEvent.click(screen.getByText("切换执行"));
  await act(() => vi.advanceTimersByTimeAsync(1));
  expect(oldSignal?.aborted).toBe(true);
  await act(async () => finishOld(view()));
  expect(screen.queryByText("正在检查代码")).not.toBeInTheDocument();
  visibility.mockReturnValue("hidden");
  fireEvent(document, new Event("visibilitychange"));
  expect(newSignal?.aborted).toBe(true);
  const count = get.mock.calls.length;
  await act(() => vi.advanceTimersByTimeAsync(10000));
  expect(get).toHaveBeenCalledTimes(count);
  mounted.unmount();
  client.clear();
});

it("isolates query identity and rejects a mismatched Runner payload", async () => {
  const api = new Api(identity, "csrf", "home");
  const client = testClient();
  vi.spyOn(api, "get").mockResolvedValue(view("foreign"));
  await expect(
    client.fetchQuery(projectQueries(api).lastMessage(run)),
  ).rejects.toThrow("身份已变化");
  expect(
    projectQueries(api).lastMessage({ ...run, attempt_id: "new" }).queryKey,
  ).not.toEqual(projectQueries(api).lastMessage(run).queryKey);
  client.clear();
});

it("does not query when first mounted in a background tab or without an execution binding", async () => {
  vi.useFakeTimers();
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
  const api = new Api(identity, "csrf", "home");
  const client = testClient();
  const get = vi.spyOn(api, "get").mockResolvedValue(view());
  const mounted = mountFeature(<LastMessage run={run} />, api, client);
  await act(() => vi.advanceTimersByTimeAsync(10000));
  expect(get).not.toHaveBeenCalled();
  mounted.unmount();
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
  mountFeature(<LastMessage run={{ ...run, attempt_id: "" }} />, api, client);
  await act(() => vi.advanceTimersByTimeAsync(10000));
  expect(get).not.toHaveBeenCalled();
  client.clear();
});
