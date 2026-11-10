import { afterEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { projectQueries } from "../../shared/api/queries";
import { identity, run, mountFeature, testClient } from "../../test/workspace";
import { RecentActivity } from "./recent-activity";
afterEach(cleanup);
it("limits visible events, keeps expanded state across revisions and removes internal events", () => {
  const api = new Api(identity, "csrf"),
    client = testClient();
  vi.spyOn(api, "get").mockImplementation(() => new Promise(() => {}) as never);
  const query = projectQueries(api).activity(run);
  const events = Array.from({ length: 7 }, (_, i) => ({
    seq: i + 1,
    ts: "2026-11-10T00:00:00Z",
    kind: "worker_state",
    status: "running",
    title: `事件 ${i + 1}`,
    detail: "细节",
  }));
  client.setQueryData(query.queryKey, {
    page: {
      diagnostics: [],
      activity: [
        ...events,
        {
          ...events[0],
          seq: 8,
          kind: "runner_event",
          title: "private-operation",
        },
      ],
    },
  });
  mountFeature(<RecentActivity run={run} />, api, client);
  expect(screen.getAllByText(/^事件/)).toHaveLength(5);
  expect(screen.queryByText("private-operation")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "更多活动" }));
  fireEvent.click(screen.getByText("事件 1"));
  act(() =>
    client.setQueryData(query.queryKey, {
      page: { diagnostics: [], activity: [...events] },
    }),
  );
  expect(screen.getAllByText(/^事件/)).toHaveLength(7);
  expect(screen.getByText("事件 1").closest("details")).toHaveAttribute("open");
  client.clear();
});
