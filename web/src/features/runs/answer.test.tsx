import { afterEach, it, expect, vi } from "vitest";
import {
  render,
  screen,
  fireEvent,
  waitFor,
  cleanup,
} from "@testing-library/react";
import { WorkspaceContext, type Workspace } from "../../app/context";
import { Api } from "../../shared/api/client";
import type { Candidate, Identity } from "../../shared/api/types";
import { readDraft } from "../../shared/api/journal";
import { Answer, draftKey } from "./answer";
afterEach(cleanup);
it("keeps Chinese composition/Enter separate from explicit submit and original candidate drafts", async () => {
  const api = new Api(
    { project_id: "input-project", instance_id: "input-instance" } as Identity,
    "csrf",
  );
  vi.spyOn(api, "get").mockResolvedValue({});
  const send = vi.fn().mockResolvedValue(undefined);
  const workspace = {
    api,
    identity: api.identity,
    online: true,
    busy: false,
    send,
    refresh: () => {},
  } as Workspace;
  const candidate = {
    run_id: "run-1",
    workflow_status: "waiting_input",
    task_status: "in_progress",
    execution_id: "execution-1",
    attempt_id: "attempt-1",
    control_revision: 7,
    recovery_version: 0,
    decision: {
      id: "decision-1",
      version: 3,
      status: "pending",
      request: {
        question: "继续当前候选？",
        context: "<script>untrusted</script>",
        blocks_progress: true,
        options: [],
        has_native_questions: false,
      },
    },
  } as unknown as Candidate;
  const draw = (run: Candidate) => (
    <WorkspaceContext.Provider value={workspace}>
      <Answer
        key={draftKey(api.identity.project_id, api.identity.instance_id, run)}
        run={run}
      />
    </WorkspaceContext.Provider>
  );
  const view = render(draw(candidate));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "打开回答" })).toBeEnabled(),
  );
  fireEvent.click(screen.getByRole("button", { name: "打开回答" }));
  const input = screen.getByRole("textbox", { name: "回答内容" });
  fireEvent.change(input, { target: { value: "中文回答\n第二行" } });
  fireEvent.compositionStart(input);
  expect(screen.getByRole("button", { name: "提交回答" })).toBeDisabled();
  fireEvent.keyDown(input, { key: "Enter", isComposing: true });
  expect(send).not.toHaveBeenCalled();
  fireEvent.compositionEnd(input);
  fireEvent.keyDown(input, { key: "Enter" });
  expect(send).not.toHaveBeenCalled();
  expect(document.querySelector("script")).toBeNull();
  await waitFor(async () =>
    expect(
      await readDraft(
        draftKey(api.identity.project_id, api.identity.instance_id, candidate),
      ),
    ).toBe("中文回答\n第二行"),
  );
  const other = {
    ...candidate,
    run_id: "run-2",
    decision: { ...candidate.decision!, id: "decision-2" },
  };
  view.rerender(draw(other));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "打开回答" })).toBeEnabled(),
  );
  fireEvent.click(screen.getByRole("button", { name: "打开回答" }));
  expect(screen.getByRole("textbox", { name: "回答内容" })).toHaveValue("");
  view.rerender(draw(candidate));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "打开回答" })).toBeEnabled(),
  );
  fireEvent.click(screen.getByRole("button", { name: "打开回答" }));
  await waitFor(() =>
    expect(screen.getByRole("textbox", { name: "回答内容" })).toHaveValue(
      "中文回答\n第二行",
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: "提交回答" }));
  await waitFor(() =>
    expect(send).toHaveBeenCalledWith(
      "answer_decision",
      "decision-1",
      3,
      expect.objectContaining({
        run_id: "run-1",
        source_attempt_id: "attempt-1",
        expected_run_revision: 7,
        note: "中文回答\n第二行",
      }),
    ),
  );
});
