import { afterEach, it, expect, vi } from "vitest";
import {
  render as testingRender,
  screen,
  fireEvent,
  waitFor,
  cleanup,
  act,
} from "@testing-library/react";
import { WorkspaceContext, type Workspace } from "../../app/context";
import { Api } from "../../shared/api/client";
import type { Candidate, Identity, Envelope } from "../../shared/api/types";
import { readDraft } from "../../shared/api/journal";
import { QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { ReactNode } from "react";
import { testClient } from "../../test/workspace";
import { Answer, draftKey } from "./answer";
afterEach(cleanup);
function render(ui: ReactNode) {
  const client = testClient();
  return testingRender(ui, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={client}>
        <MemoryRouter>{children}</MemoryRouter>
      </QueryClientProvider>
    ),
  });
}
it("submits native multi-question JSON once without duplicating it as a note", async () => {
  const api = new Api(
    { project_id: "native-multi", instance_id: "original" } as Identity,
    "csrf",
  );
  vi.spyOn(api, "get").mockResolvedValue({});
  const send = vi.fn().mockResolvedValue(true);
  const workspace = {
    api,
    identity: api.identity,
    online: true,
    busy: false,
    send,
    submit: async (e: Envelope) => {
      await send(e.action, e.target_id, e.expected_version, e.payload);
      return { ...e, original: e, status: "applied", sampled_at: "now" };
    },
  } as unknown as Workspace;
  const run = {
    run_id: "candidate",
    execution_id: "original-run",
    attempt_id: "original-attempt",
    participation: "active",
    submission_status: "accepted",
    stop_requested: false,
    control_revision: 4,
    recovery_version: 7,
    workflow_status: "waiting_input",
    task_status: "in_progress",
    decision: {
      id: "native-question",
      version: 7,
      status: "pending",
      request: {
        question: "回答全部问题",
        context: "color: 颜色\nred: 红色\ndetails: 说明",
        options: [],
        has_native_questions: true,
      },
    },
  } as unknown as Candidate;
  render(
    <WorkspaceContext.Provider value={workspace}>
      <Answer run={run} />
    </WorkspaceContext.Provider>,
  );
  await waitFor(() =>
    expect(screen.getByRole("textbox", { name: "回答内容" })).toBeEnabled(),
  );
  fireEvent.change(screen.getByRole("textbox", { name: "回答内容" }), {
    target: { value: '{"color":["red"],"details":["保留原任务"]}' },
  });
  fireEvent.click(screen.getByRole("button", { name: "回复并继续" }));
  await waitFor(() => expect(send).toHaveBeenCalledTimes(1));
  const payload = send.mock.calls[0][3];
  expect(payload.answers).toEqual({ color: ["red"], details: ["保留原任务"] });
  expect(payload).not.toHaveProperty("note");
  expect(payload).not.toHaveProperty("option_id");
  expect(payload).toMatchObject({
    run_id: "candidate",
    source_attempt_id: "original-attempt",
    expected_run_revision: 4,
  });
});
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
    submit: async (e: Envelope) => {
      await send(e.action, e.target_id, e.expected_version, e.payload);
      return { ...e, original: e, status: "applied", sampled_at: "now" };
    },
    refresh: () => {},
  } as Workspace;
  const candidate = {
    run_id: "run-1",
    participation: "active",
    submission_status: "accepted",
    stop_requested: false,
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
    expect(screen.getByRole("textbox", { name: "回答内容" })).toBeEnabled(),
  );
  const input = screen.getByRole("textbox", { name: "回答内容" });
  fireEvent.change(input, { target: { value: "中文回答\n第二行" } });
  fireEvent.compositionStart(input);
  expect(screen.getByRole("button", { name: "回复并继续" })).toBeDisabled();
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
    expect(screen.getByRole("textbox", { name: "回答内容" })).toBeEnabled(),
  );
  expect(screen.getByRole("textbox", { name: "回答内容" })).toHaveValue("");
  view.rerender(draw(candidate));
  await waitFor(() =>
    expect(screen.getByRole("textbox", { name: "回答内容" })).toBeEnabled(),
  );
  await waitFor(() =>
    expect(screen.getByRole("textbox", { name: "回答内容" })).toHaveValue(
      "中文回答\n第二行",
    ),
  );
  fireEvent.click(screen.getByRole("button", { name: "回复并继续" }));
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

it.each(["unmount", "offline", "revision", "collapse"])(
  "cancels late eligibility without sending when %s",
  async (reason) => {
    const api = new Api(
      { project_id: "cancel-input", instance_id: "original" } as Identity,
      "csrf",
    );
    let resolve!: (value: unknown) => void;
    const get = vi.spyOn(api, "get").mockImplementation((path) =>
      path.includes("decision-eligibility")
        ? (new Promise((done) => {
            resolve = done;
          }) as never)
        : (Promise.resolve({ record: null }) as never),
    );
    const send = vi.fn();
    const workspace = {
      api,
      identity: api.identity,
      online: true,
      busy: false,
      send,
      submit: async (e: Envelope) => {
        await send(e.action, e.target_id, e.expected_version, e.payload);
        return { ...e, original: e, status: "applied", sampled_at: "now" };
      },
    } as unknown as Workspace;
    const run = {
      run_id: "run",
      execution_id: "exec",
      attempt_id: "attempt",
      control_revision: 1,
      recovery_version: 0,
      participation: "active",
      submission_status: "accepted",
      stop_requested: false,
      workflow_status: "waiting_input",
      task_status: "in_progress",
      decision: {
        id: "question",
        version: 1,
        status: "pending",
        request: {
          question: "继续？",
          options: [],
          context: "",
          has_native_questions: false,
        },
      },
    } as unknown as Candidate;
    const draw = (online = true, version = 1) => (
      <WorkspaceContext.Provider value={{ ...workspace, online }}>
        <Answer run={{ ...run, control_revision: version }} />
      </WorkspaceContext.Provider>
    );
    const view = render(draw());
    await waitFor(() =>
      expect(screen.getByRole("textbox", { name: "回答内容" })).toBeEnabled(),
    );
    fireEvent.change(screen.getByRole("textbox", { name: "回答内容" }), {
      target: { value: "保留原候选" },
    });
    fireEvent.click(screen.getByRole("button", { name: "回复并继续" }));
    await waitFor(() =>
      expect(
        get.mock.calls.filter(([path]) =>
          path.includes("decision-eligibility"),
        ),
      ).toHaveLength(1),
    );
    if (reason === "unmount") view.unmount();
    else if (reason === "collapse")
      fireEvent.click(screen.getByRole("button", { name: "收起" }));
    else
      view.rerender(draw(reason !== "offline", reason === "revision" ? 2 : 1));
    await act(async () => {
      resolve({});
    });
    expect(send).not.toHaveBeenCalled();
  },
);
