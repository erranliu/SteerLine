import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, testClient } from "../../test/workspace";
import { OperationResult } from "./operation-result";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
it.each([true, false])(
  "binds a historical publication receipt to its exact record (matches=%s)",
  async (matches) => {
    const api = new Api(identity, "csrf", "home");
    const read = vi.spyOn(api, "get").mockResolvedValue({
      request_id: "original-publish",
      project_id: identity.project_id,
      instance_id: "old-instance",
      action: "roadmap_publish",
      target_id: matches ? "old-record" : "another-record",
      status: "rejected",
      diagnostic: "原发布被拒绝",
      roadmap: {
        id: matches ? "old-record" : "another-record",
        run_id: "generation-run",
      },
    });
    const submit = vi.spyOn(api, "submit");
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    const client = testClient();
    mountFeature(
      <OperationResult requestID="original-publish" targetID="old-record" />,
      api,
      client,
    );
    if (matches) await screen.findByText("原发布被拒绝");
    else await screen.findByText("无法核实该请求与当前记录的关联");
    fireEvent.click(screen.getByRole("button", { name: "复制执行信息" }));
    await screen.findByRole("status");
    const copied = writeText.mock.calls[0][0];
    expect(copied).toContain("original-publish");
    expect(copied).not.toContain("generation-run");
    expect(copied.includes("old-record")).toBe(matches);
    expect(read).toHaveBeenCalledWith("/commands/original-publish");
    expect(submit).not.toHaveBeenCalled();
    client.clear();
  },
);

it("shows an original source-level parse receipt while an older record is still current", async () => {
  const api = new Api(identity, "csrf", "home");
  vi.spyOn(api, "get").mockResolvedValue({
    request_id: "new-parse",
    project_id: identity.project_id,
    instance_id: "instance",
    action: "roadmap_parse",
    target_id: "source",
    status: "outcome_unknown",
    diagnostic: "原解析提交待核实",
  });
  const client = testClient();
  mountFeature(
    <OperationResult
      requestID="new-parse"
      targetID="older-record"
      sourceID="source"
    />,
    api,
    client,
  );
  expect(await screen.findByText("原解析提交待核实")).toBeVisible();
  expect(
    screen.queryByText("无法核实该请求与当前记录的关联"),
  ).not.toBeInTheDocument();
  client.clear();
});
