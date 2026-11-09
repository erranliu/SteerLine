import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, run, testClient } from "../../test/workspace";
import type { Candidate } from "../../shared/api/types";
import { MainAgent, type AdvisoryDetail } from "./main-agent";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
const candidate = { ...run, run_id: "b".repeat(32) } as Candidate;
const ready: AdvisoryDetail = {
  id: "a".repeat(32),
  revision: 2,
  candidate_id: candidate.run_id,
  state: "ready",
  settled: true,
  prompt: "请根据已提供的要求，继续讨论缺少的产品决定。",
  context: {
    goal: "澄清原要求",
    user_input: "",
    materials: [
      {
        id: "spec",
        label: "已提供 Spec",
        body: "Empty input is unspecified.",
        revision: "original",
      },
    ],
  },
};
function mount(record = ready, value = candidate) {
  const api = new Api(identity, "csrf", "home");
  const submit = vi.fn();
  const fetch = vi.fn(async (url: unknown) =>
    Response.json(
      String(url).includes("?candidate_id=")
        ? { records: [{ id: record.id, state: record.state }], has_more: false }
        : record,
    ),
  );
  vi.stubGlobal("fetch", fetch);
  const copy = vi.fn(async () => {});
  vi.stubGlobal("navigator", { clipboard: { writeText: copy } });
  const view = mountFeature(
    <MainAgent run={value} />,
    api,
    testClient(),
    true,
    { submit },
  );
  return { fetch, copy, submit, view };
}
function open() {
  const details = screen.getByText("后续对话提示词建议").closest("details")!;
  details.open = true;
  fireEvent(details, new Event("toggle"));
}
it("observes an automatic result without generating on mount or open", async () => {
  const { fetch, submit } = mount();
  expect(fetch).not.toHaveBeenCalled();
  open();
  expect(await screen.findByText(ready.prompt!)).toBeInTheDocument();
  expect(submit).not.toHaveBeenCalled();
  expect(
    fetch.mock.calls.every((c) => String(c[0]).includes("main-agent")),
  ).toBe(true);
  for (const name of [
    "提交此澄清回答",
    "创建材料修订 PR",
    "分析并给建议",
    "补充信息并修订建议",
  ]) {
    expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
  }
  expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
});
it("copies the immutable suggestion even after the original candidate ends", async () => {
  const { copy, submit } = mount(ready, {
    ...candidate,
    participation: "withdrawn",
    task_status: "closed",
    stop_requested: true,
  } as Candidate);
  open();
  fireEvent.click(await screen.findByRole("button", { name: "复制提示词" }));
  await waitFor(() => expect(copy).toHaveBeenCalledWith(ready.prompt));
  expect(submit).not.toHaveBeenCalled();
});
it("shows generate failure without opening a generation path", async () => {
  const { submit } = mount({
    ...ready,
    state: "failed",
    prompt: undefined,
    diagnostic: "Runner generate 配置不可用，未提交模型请求",
  });
  open();
  expect(
    await screen.findByText(/Runner generate 配置不可用/),
  ).toBeInTheDocument();
  expect(screen.queryByRole("button")).not.toBeInTheDocument();
  expect(submit).not.toHaveBeenCalled();
});
it("does not show a response from another candidate", async () => {
  const { copy } = mount({ ...ready, candidate_id: "c".repeat(32) });
  open();
  expect(
    await screen.findByText("建议不属于当前候选，未显示其他范围的结果。"),
  ).toBeInTheDocument();
  expect(screen.queryByText(ready.prompt!)).not.toBeInTheDocument();
  expect(copy).not.toHaveBeenCalled();
});

it.each(["cancelling", "failed"])(
  "keeps observing %s until resource settlement",
  async (state) => {
    const record: AdvisoryDetail = {
      ...ready,
      state,
      settled: false,
      prompt: undefined,
      diagnostic: "等待原资源结算",
    };
    const { fetch } = mount(record);
    open();
    expect(await screen.findByText("等待原资源结算")).toBeInTheDocument();
    record.state = state === "cancelling" ? "cancelled" : "failed";
    record.settled = true;
    record.diagnostic = "原生成资源已核实停止";
    await waitFor(
      () =>
        expect(screen.getByText("原生成资源已核实停止")).toBeInTheDocument(),
      { timeout: 4500 },
    );
    expect(
      fetch.mock.calls.filter((c) => !String(c[0]).includes("?candidate_id="))
        .length,
    ).toBeGreaterThanOrEqual(2);
  },
  7000,
);
