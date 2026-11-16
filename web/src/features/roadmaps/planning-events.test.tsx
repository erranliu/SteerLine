import { renderHook, waitFor } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { usePlanningEvents } from "./planning-events";

test("resumes a turn by cursor and assembles mirrored text and tool transitions", async () => {
  const marker = (number: number, kind: string, text?: string) => ({
    id: String(number),
    turn_id: "turn",
    item_id: "message",
    kind,
    text,
  });
  const tool = (id: string, status: string) => ({
    id,
    turn_id: "turn",
    item_id: "command",
    kind: "tool",
    tool: "commandExecution",
    status,
  });
  const get = vi.fn()
    .mockResolvedValueOnce({
      events: [
        marker(1, "assistant_started"),
        marker(2, "assistant_started"),
        marker(3, "assistant_delta", "one"),
        marker(4, "assistant_delta", " two"),
        tool("5", "running"),
      ],
      next_cursor: "first",
      has_more: true,
      active: true,
    })
    .mockResolvedValueOnce({
      events: [
        marker(6, "assistant_delta", "one"),
        marker(7, "assistant_delta", " two"),
        marker(8, "assistant_delta", " three"),
        tool("9", "completed"),
      ],
      next_cursor: "second",
      has_more: false,
      active: false,
    });
  const api = { get };
  const { result, unmount } = renderHook(() =>
    usePlanningEvents("task", true, api, "/task/events"),
  );
  await waitFor(() => {
    const turn = result.current.turns.get("turn");
    expect(turn?.messages.get("message")?.text).toBe("one two three");
    expect(Array.from(turn?.tools.values() ?? [])).toEqual([
      { tool: "commandExecution", status: "completed" },
    ]);
  });
  expect(get.mock.calls.map(([path]) => path)).toEqual([
    "/task/events",
    "/task/events?after=first",
  ]);
  unmount();
});
