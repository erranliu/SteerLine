import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, run, testClient } from "../../test/workspace";
import { CopyExecutionInfo } from "./copy-execution-info";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("copies a historical candidate without requiring current presentation or online queries", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal("navigator", { clipboard: { writeText } });
  const api = new Api(identity, "csrf", "home");
  const read = vi.spyOn(api, "get");
  const client = testClient();
  mountFeature(
    <CopyExecutionInfo
      run={{
        ...run,
        run_id: "old-candidate",
        execution_id: "old-run",
        attempt_id: "old-attempt",
      }}
    />,
    api,
    client,
    false,
  );
  fireEvent.click(screen.getByRole("button", { name: "复制执行信息" }));
  expect(await screen.findByRole("status")).toHaveTextContent("已复制执行信息");
  const text = writeText.mock.calls[0][0];
  expect(text).toContain("Task ID：1");
  expect(text).toContain("请求 / 候选 ID：old-candidate");
  expect(text).toContain("Runner Run ID：old-run");
  expect(text).toContain("Attempt ID：old-attempt");
  expect(read).not.toHaveBeenCalled();
  client.clear();
});
