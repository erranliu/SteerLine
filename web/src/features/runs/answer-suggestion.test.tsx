import { afterEach, expect, it, vi } from "vitest";
import {
  act,
  render,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { saveRequest, writeDraft } from "../../shared/api/journal";
import { identity, run, testClient } from "../../test/workspace";
import type {
  Candidate,
  Envelope,
  Receipt,
  StoredRequest,
} from "../../shared/api/types";
import { QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { WorkspaceContext, type Workspace } from "../../app/context";
import { Answer, draftKey } from "./answer";
import type { AdvisoryDetail } from "./continuation-suggestion";

afterEach(cleanup);
const candidate = {
  ...run,
  run_id: "b".repeat(32),
  control_revision: 7,
  recovery_version: 3,
  workflow_status: "waiting_input",
  decision: {
    id: "question",
    attempt_id: "attempt",
    version: 3,
    status: "pending",
    request: {
      question: "原问题",
      context: "原问题",
      options: [],
      has_native_questions: false,
      blocks_progress: true,
      interaction_kind: "blocking",
      allow_free_text: true,
    },
  },
} as Candidate;
const ready: AdvisoryDetail = {
  id: "a".repeat(32),
  revision: 2,
  candidate_id: candidate.run_id,
  occurrence: "attempt:question",
  state: "ready",
  settled: true,
  prompt: "请按已确认的方案继续。\r\n保留原文。",
};
function setup(
  record: AdvisoryDetail | null = ready,
  home = "suggested-answer",
  delay = false,
) {
  const api = new Api(identity, "csrf", home);
  let resolve!: (data: unknown) => void;
  const get = vi.spyOn(api, "get").mockImplementation(async (path) => {
    if (path.includes("main-agent?"))
      return delay
        ? await new Promise((done) => {
            resolve = done;
          })
        : ({ record } as never);
    return {} as never;
  });
  const send = vi.fn().mockResolvedValue(true);
  const client = testClient();
  const submit = vi.fn(async (e: Envelope): Promise<Receipt | undefined> => {
    await send(e.action, e.target_id, e.expected_version, e.payload);
    return { ...e, original: e, status: "applied", sampled_at: "now" };
  });
  const draw = (value = candidate, requests: StoredRequest[] = []) => (
    <QueryClientProvider client={client}>
      <WorkspaceContext.Provider
        value={
          {
            api,
            identity,
            online: true,
            busy: false,
            send,
            submit,
            requests,
          } as unknown as Workspace
        }
      >
        <MemoryRouter>
          <Answer run={value} />
        </MemoryRouter>
      </WorkspaceContext.Provider>
    </QueryClientProvider>
  );
  const view = render(draw());
  return {
    api,
    get,
    send,
    client,
    view,
    submit,
    update: (value = candidate, requests: StoredRequest[] = []) =>
      view.rerender(draw(value, requests)),
    resolve: (value: unknown) => resolve(value),
  };
}

it("defaults to the exact current suggestion and replies once with original identities and bytes", async () => {
  const w = setup();
  await waitFor(() =>
    expect(screen.getByRole("textbox", { name: "回答内容" })).toHaveValue(
      ready.prompt!.replace(/\r\n/g, "\n"),
    ),
  );
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "回复并继续" })).toBeEnabled(),
  );
  expect(screen.queryByRole("button", { name: "打开回答" })).toBeNull();
  expect(screen.getAllByText("原问题")).toHaveLength(1);
  expect(w.get.mock.calls[0][0]).toContain("occurrence=attempt%3Aquestion");
  expect(w.send).not.toHaveBeenCalled();
  const button = screen.getByRole("button", { name: "回复并继续" });
  fireEvent.click(button);
  fireEvent.click(button);
  await waitFor(() => expect(w.send).toHaveBeenCalledTimes(1));
  expect(w.send).toHaveBeenCalledWith("answer_decision", "question", 3, {
    run_id: candidate.run_id,
    decision_id: "question",
    option_id: "",
    note: ready.prompt,
    expected_run_revision: 7,
    source_attempt_id: "attempt",
    expected_recovery_version: 3,
  });
  expect(await screen.findByText("回答已提交，等待状态更新。")).toBeVisible();
  w.client.clear();
});

it.each(["我修改后的回答", ""])(
  "preserves the saved draft %j instead of refilling on reload",
  async (draft) => {
    const home = "saved-answer-" + (draft ? "text" : "empty");
    await writeDraft(
      home +
        "/" +
        draftKey(identity.project_id, identity.instance_id, candidate),
      draft,
    );
    const w = setup(ready, home);
    await screen.findByText("生成状态：建议已生成");
    await waitFor(() =>
      expect(screen.getByRole("textbox", { name: "回答内容" })).toBeEnabled(),
    );
    expect(screen.getByRole("textbox", { name: "回答内容" })).toHaveValue(
      draft,
    );
    expect(w.send).not.toHaveBeenCalled();
    w.client.clear();
  },
);

it("preserves edits made before generation completes and submits the edited answer", async () => {
  const w = setup(null, "late-answer", true);
  const input = screen.getByRole("textbox", { name: "回答内容" });
  await waitFor(() => expect(input).toBeEnabled());
  fireEvent.change(input, { target: { value: "手写优先" } });
  await act(async () => w.resolve({ record: ready }));
  expect(input).toHaveValue("手写优先");
  fireEvent.click(screen.getByRole("button", { name: "回复并继续" }));
  await waitFor(() =>
    expect(w.send).toHaveBeenCalledWith(
      "answer_decision",
      "question",
      3,
      expect.objectContaining({ note: "手写优先" }),
    ),
  );
  w.client.clear();
});

it.each([
  { ...ready, candidate_id: "c".repeat(32) },
  { ...ready, occurrence: "old-attempt:question" },
  { ...ready, occurrence: "attempt:old-question" },
  { ...ready, state: "failed" },
  { ...ready, settled: false },
])(
  "does not fill foreign, historical, failed or unsettled suggestions: %j",
  async (record) => {
    const w = setup(
      record,
      "unusable-answer-" +
        record.occurrence +
        record.state +
        record.settled +
        record.candidate_id,
    );
    await waitFor(() => expect(w.get).toHaveBeenCalled());
    await waitFor(() =>
      expect(screen.getByRole("textbox", { name: "回答内容" })).toBeEnabled(),
    );
    expect(screen.getByRole("textbox", { name: "回答内容" })).toHaveValue("");
    expect(screen.getByRole("button", { name: "回复并继续" })).toBeDisabled();
    expect(w.send).not.toHaveBeenCalled();
    w.client.clear();
  },
);

it("retains the editable answer when eligibility fails without sending", async () => {
  const w = setup(ready, "rejected-answer");
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "回复并继续" })).toBeEnabled(),
  );
  w.get.mockRejectedValueOnce(new Error("原问题已变更"));
  fireEvent.click(screen.getByRole("button", { name: "回复并继续" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("原问题已变更");
  expect(screen.getByRole("textbox", { name: "回答内容" })).toHaveValue(
    ready.prompt!.replace(/\r\n/g, "\n"),
  );
  expect(w.send).not.toHaveBeenCalled();
  w.client.clear();
});

it.each(["submitted", "accepted", "outcome_unknown"] as const)(
  "reopens a later rejected %s answer without overwriting the draft",
  async (status) => {
    const w = setup(ready, "receipt-" + status);
    w.submit.mockImplementation(async (e) => ({
      ...e,
      original: e,
      status,
      sampled_at: "now",
    }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "回复并继续" })).toBeEnabled(),
    );
    const input = screen.getByRole("textbox", { name: "回答内容" });
    fireEvent.change(input, { target: { value: "保留拒绝后的草稿" } });
    fireEvent.click(screen.getByRole("button", { name: "回复并继续" }));
    await waitFor(() => expect(w.submit).toHaveBeenCalledTimes(1));
    const original = w.submit.mock.calls[0][0];
    const row = (state: Receipt["status"]): StoredRequest => ({
      envelope: original,
      created_at: "now",
      response: {
        ...original,
        original,
        status: state,
        diagnostic: state === "rejected" ? "原版本已失效" : "",
        sampled_at: "now",
      },
    });
    w.update(candidate, [row("outcome_unknown")]);
    expect(screen.queryByRole("button", { name: "回复并继续" })).toBeNull();
    expect(w.submit).toHaveBeenCalledTimes(1);
    w.update(
      {
        ...candidate,
        control_revision: 8,
        decision: { ...candidate.decision!, version: 4 },
      },
      [row("rejected")],
    );
    expect(await screen.findByRole("alert")).toHaveTextContent("原版本已失效");
    expect(screen.getByRole("textbox", { name: "回答内容" })).toHaveValue(
      "保留拒绝后的草稿",
    );
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "回复并继续" })).toBeEnabled(),
    );
    fireEvent.click(screen.getByRole("button", { name: "回复并继续" }));
    await waitFor(() => expect(w.submit).toHaveBeenCalledTimes(2));
    const retried = w.submit.mock.calls[1][0];
    expect(retried.request_id).not.toBe(original.request_id);
    expect(retried.expected_version).toBe(4);
    expect(retried.payload.note).toBe("保留拒绝后的草稿");
    w.client.clear();
  },
);

it("allows another business feedback after the original feedback is applied", async () => {
  const w = setup(null, "more-feedback");
  const feedback = {
    ...candidate,
    workflow_status: "pr_ready",
    decision: {
      ...candidate.decision!,
      request: {
        ...candidate.decision!.request,
        interaction_kind: "business_feedback",
      },
    },
  };
  w.update(feedback);
  await waitFor(() =>
    expect(screen.getByRole("textbox", { name: "回答内容" })).toBeEnabled(),
  );
  fireEvent.change(screen.getByRole("textbox", { name: "回答内容" }), {
    target: { value: "第一条反馈" },
  });
  fireEvent.click(screen.getByRole("button", { name: "提交回答" }));
  fireEvent.click(await screen.findByRole("button", { name: "继续追加反馈" }));
  expect(screen.getByRole("textbox", { name: "回答内容" })).toHaveValue("");
  fireEvent.change(screen.getByRole("textbox", { name: "回答内容" }), {
    target: { value: "第二条反馈" },
  });
  fireEvent.click(screen.getByRole("button", { name: "提交回答" }));
  await waitFor(() => expect(w.submit).toHaveBeenCalledTimes(2));
  expect(w.submit.mock.calls[1][0].payload.note).toBe("第二条反馈");
  w.client.clear();
});

it.each([false, true])(
  "only keeps an uncertain submission locked when its original journal row exists: %s",
  async (persisted) => {
    const w = setup(ready, "unsent-answer-" + persisted);
    w.submit.mockImplementation(async (e) => {
      if (persisted) await saveRequest(e);
      return undefined;
    });
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "回复并继续" })).toBeEnabled(),
    );
    fireEvent.click(screen.getByRole("button", { name: "回复并继续" }));
    if (persisted) {
      expect(
        await screen.findByText("回答结果待核实，请在操作记录中核实原请求。"),
      ).toBeVisible();
      expect(screen.queryByRole("button", { name: "回复并继续" })).toBeNull();
    } else {
      expect(await screen.findByRole("alert")).toHaveTextContent(
        "回答尚未提交",
      );
      expect(screen.getByRole("textbox", { name: "回答内容" })).toHaveValue(
        ready.prompt!.replace(/\r\n/g, "\n"),
      );
      expect(screen.getByRole("button", { name: "回复并继续" })).toBeEnabled();
    }
    expect(w.submit).toHaveBeenCalledTimes(1);
    w.client.clear();
  },
);
