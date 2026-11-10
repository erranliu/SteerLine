import { afterEach, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { WorkspaceContext, type Workspace } from "../../app/context";
import { Api } from "../../shared/api/client";
import type { Candidate } from "../../shared/api/types";
import { identity, run, snapshot, testClient } from "../../test/workspace";
import { FinalOutput } from "./final-output";
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});
function setup(initial: Candidate) {
  const api = new Api(identity, "csrf", "home"),
    client = testClient();
  const final = vi
    .spyOn(api, "final")
    .mockImplementation(async (candidate) => ({
      workflow_id: candidate.run_id,
      execution_id: candidate.execution_id,
      attempt_id: candidate.attempt_id,
      bodies: [
        {
          item_key: "old",
          revision: 1,
          text: "先前轮次结果",
          complete: true,
          completeness: "complete",
        },
      ],
      page: { events: [], has_more: false },
    }));
  const workspace = {
    api,
    identity,
    online: true,
    snapshot: snapshot(api, [initial]),
    busy: false,
  } as Workspace;
  const tree = (candidate: Candidate) => (
    <QueryClientProvider client={client}>
      <WorkspaceContext.Provider value={workspace}>
        <FinalOutput
          key={candidate.execution_id + candidate.attempt_id}
          run={candidate}
        />
      </WorkspaceContext.Provider>
    </QueryClientProvider>
  );
  const view = render(tree(initial));
  return {
    final,
    client,
    view,
    change: (candidate: Candidate) => view.rerender(tree(candidate)),
  };
}
it.each(["direct link", "refresh", "candidate switch"])(
  "opens a terminal result on %s without declaring history current",
  async (mode) => {
    const terminal = { ...run, execution_status: "succeeded" };
    const f = setup(mode === "candidate switch" ? run : terminal);
    if (mode === "refresh") {
      f.view.unmount();
      f.client.clear();
      const again = setup(terminal);
      await screen.findByText("先前轮次结果");
      expect(again.final).toHaveBeenCalledTimes(1);
      again.client.clear();
      return;
    }
    if (mode === "candidate switch")
      f.change({ ...terminal, execution_id: "other-execution" });
    expect(await screen.findByText("先前轮次结果")).toBeVisible();
    expect(screen.getByText("回复历史，包含先前轮次。")).toBeVisible();
    expect(screen.getByRole("button", { name: "最终回复" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    expect(f.final).toHaveBeenCalledTimes(1);
    f.client.clear();
  },
);
it("opens on terminal transition but respects explicit collapse on subsequent snapshots", async () => {
  const f = setup(run);
  expect(f.final).not.toHaveBeenCalled();
  f.change({ ...run, execution_status: "succeeded" });
  await screen.findByText("先前轮次结果");
  fireEvent.click(screen.getByRole("button", { name: "最终回复" }));
  f.change({ ...run, execution_status: "succeeded", workflow_stage: "closed" });
  expect(screen.getByRole("button", { name: "最终回复" })).toHaveAttribute(
    "aria-expanded",
    "false",
  );
  expect(screen.queryByText("先前轮次结果")).toBeNull();
  vi.useFakeTimers();
  await act(() => vi.advanceTimersByTimeAsync(6000));
  expect(f.final).toHaveBeenCalledTimes(1);
  f.client.clear();
});

it("keeps cached replies with the last successful read time when refresh fails", async () => {
  const f = setup({ ...run, execution_status: "succeeded" });
  await screen.findByText("先前轮次结果");
  f.final.mockRejectedValue(new Error("final unavailable"));
  // Use the actual query entry, without broad invalidation or a new request identity.
  const query = f.client
    .getQueryCache()
    .findAll()
    .find((q) => q.queryKey.includes("final"))!;
  await act(async () => {
    await f.client.refetchQueries({ queryKey: query.queryKey, exact: true });
  });
  expect(await screen.findByText(/最后成功读取于/)).toHaveAttribute(
    "datetime",
    new Date(query.state.dataUpdatedAt).toISOString(),
  );
  expect(screen.getByText("先前轮次结果")).toBeVisible();
  f.client.clear();
});
