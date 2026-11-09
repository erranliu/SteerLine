import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, run, testClient } from "../../test/workspace";
import type { Candidate, Envelope } from "../../shared/api/types";
import { MainAgent, type MaterialAnalysis } from "./main-agent";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
const candidate = {
  ...run,
  run_id: "b".repeat(32),
  control_revision: 7,
  decision: { id: "question", version: 8, status: "pending" },
  runner_delivery: { controls: { can_reply: true } },
} as Candidate;
const original: MaterialAnalysis = {
  id: "a".repeat(32),
  revision: 4,
  created_at: "2026-11-09T00:00:00Z",
  state: "review",
  submission: "started",
  projection: { stop_status: "stopped" },
  review: "reviewed-exact-output",
  input: { kind: "clarification", note: "How to handle empty input?" },
  context: {
    candidate_id: candidate.run_id,
    candidate_revision: 7,
    runner_revision: 8,
    repository_commit: "current",
    original_base: "original",
    question: { id: "question", text: "What happens for empty input?" },
    original: [],
    current: [
      {
        path: "task.spec.md",
        body: "Old requirement",
        commit: "current",
        sha256: "old-sha",
      },
    ],
  },
  result: {
    disposition: "clarification",
    summary: "The materials already answer this",
    recommendation: "Return an error.",
    questions: [],
    evidence: [
      { path: "task.spec.md", commit: "original", quote: "Return an error." },
    ],
    answer: { note: "Return an error.", option_id: "", answers: {} },
    changes: [],
  },
};
function mount(record = original, value = candidate) {
  const api = new Api(identity, "csrf", "home");
  const submit = vi.fn(async (e: Envelope) => ({
    ...e,
    original: e,
    status: "applied" as const,
    sampled_at: "now",
  }));
  const fetch = vi.fn(async (url: unknown) => {
    const path = String(url);
    if (path.endsWith("/options"))
      return new Response(
        JSON.stringify({
          commit: "current",
          rules_ready: true,
          model: "gpt-6.1-sol",
          reasoning_effort: "high",
        }),
      );
    if (path.includes("?candidate_id="))
      return new Response(
        JSON.stringify({
          records: [
            {
              id: record.id,
              state: record.state,
              created_at: record.created_at,
              note: record.input.note,
              kind: record.input.kind,
            },
          ],
          has_more: false,
        }),
      );
    return new Response(JSON.stringify(record));
  });
  vi.stubGlobal("fetch", fetch);
  const client = testClient();
  const view = mountFeature(<MainAgent run={value} />, api, client, true, {
    submit,
  });
  return { fetch, submit, view, client };
}
function open() {
  const details = screen
    .getByText("主 Agent · 分析材料问题")
    .closest("details")!;
  details.open = true;
  fireEvent(details, new Event("toggle"));
}
it("makes no reads or model requests until opened and explicit analysis", async () => {
  const { fetch, submit } = mount();
  expect(fetch).not.toHaveBeenCalled();
  open();
  await screen.findByText("The materials already answer this");
  expect(submit).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText("材料问题描述"), {
    target: { value: "Clarify validation" },
  });
  fireEvent.click(screen.getByRole("button", { name: "分析并给建议" }));
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0]).toMatchObject({
    action: "main_agent_analyze",
    target_id: candidate.run_id,
    expected_version: 7,
    payload: { kind: "clarification", note: "Clarify validation" },
  });
});
it("applies a reviewed answer through backend analysis binding", async () => {
  const { submit } = mount();
  open();
  const button = await screen.findByRole("button", { name: "提交此澄清回答" });
  expect(button).toBeEnabled();
  fireEvent.click(button);
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0]).toMatchObject({
    action: "main_agent_answer",
    target_id: original.id,
    expected_version: 4,
    payload: { review: original.review },
  });
});
it("refuses a stale original question and does not invent one", async () => {
  mount(original, { ...candidate, control_revision: 9 });
  open();
  expect(
    await screen.findByRole("button", { name: "提交此澄清回答" }),
  ).toBeDisabled();
  expect(screen.getByText(/原问题或候选已变化/)).toBeVisible();
  cleanup();
  const record = structuredClone(original);
  delete record.context.question;
  record.result!.answer = null;
  mount(record);
  open();
  await screen.findByText(record.result!.summary);
  expect(screen.queryByRole("button", { name: "提交此澄清回答" })).toBeNull();
});
it("shows both complete document versions before a PR action and never offers merge", async () => {
  const record = structuredClone(original);
  record.result!.disposition = "revision";
  record.result!.answer = null;
  record.result!.changes = [
    {
      path: "task.spec.md",
      before_sha256: "old-sha",
      body: "New complete requirement",
      reason: "Resolve ambiguity",
    },
  ];
  const { submit } = mount(record);
  open();
  await screen.findByText("Resolve ambiguity");
  fireEvent.click(screen.getByText("查看完整原文与修订正文"));
  expect(screen.getByText("Old requirement")).toBeInTheDocument();
  expect(screen.getByText("New complete requirement")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "创建材料修订 PR" }));
  await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
  expect(submit.mock.calls[0][0]).toMatchObject({
    action: "main_agent_publish",
    target_id: record.id,
    payload: { review: record.review },
  });
  expect(screen.queryByRole("button", { name: /合并 PR/ })).toBeNull();
});
it("keeps stop visible when business failed but resources remain unsettled", async () => {
  const record = structuredClone(original);
  record.state = "failed";
  record.result = undefined;
  record.projection = { stop_status: "unknown" };
  mount(record);
  open();
  expect(
    await screen.findByRole("button", { name: "停止本次分析" }),
  ).toBeEnabled();
  expect(screen.getByRole("button", { name: "分析并给建议" })).toBeDisabled();
});
