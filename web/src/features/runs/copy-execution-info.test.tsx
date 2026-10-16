import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import type { Candidate, Presentation } from "../../shared/api/types";
import { projectQueries } from "../../shared/api/queries";
import { identity, mountFeature, run, testClient } from "../../test/workspace";
import { CopyExecutionInfo } from "./copy-execution-info";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const location = {
  application_home: "C:\\调试 home\\SteerLine",
  repository: "C:\\项目 repo",
  runner: { executable: "C:\\工具\\steerrunner.exe" },
};
const candidate = {
  ...run,
  run_id: "candidate-full-id",
  execution_id: "runner-full-id",
  attempt_id: "attempt-full-id",
  runner_state_dir: "C:\\slr\\original-state-directory",
};

function mount(
  execution_location?: Presentation["execution_location"],
  selected: Candidate = candidate,
) {
  const client = testClient();
  const api = new Api(identity, "csrf", "home-id-is-not-a-path");
  if (execution_location) {
    client.setQueryData(projectQueries(api).presentation().queryKey, {
      presentation: {
        execution_location,
        profiles: [],
        default_profile_id: "",
      },
    });
  }
  // Cached location remains usable when disconnected.
  mountFeature(<CopyExecutionInfo run={selected} />, api, client, false);
  return { client };
}

it("copies exact paths and full identities for each execution owner", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal("navigator", { clipboard: { writeText } });
  const { client } = mount(location);
  fireEvent.click(
    screen.getByRole("button", { name: "复制 SteerLine 执行信息" }),
  );
  await waitFor(() =>
    expect(screen.getByRole("status")).toHaveTextContent("已复制 SteerLine"),
  );
  expect(writeText).toHaveBeenLastCalledWith(
    [
      "SteerLine",
      `Home: ${location.application_home}`,
      `Repository: ${location.repository}`,
      "Task ID: 1",
      "Candidate Run ID: candidate-full-id",
    ].join("\n"),
  );

  fireEvent.click(
    screen.getByRole("button", { name: "复制 SteerRunner 执行信息" }),
  );
  await waitFor(() =>
    expect(screen.getByRole("status")).toHaveTextContent("已复制 SteerRunner"),
  );
  expect(writeText).toHaveBeenLastCalledWith(
    [
      "SteerRunner",
      `Executable: ${location.runner.executable}`,
      `State directory: ${candidate.runner_state_dir}`,
      "Runner Run ID: runner-full-id",
      "Attempt ID: attempt-full-id",
    ].join("\n"),
  );
  client.clear();
});

it("copies the configured Runner location before a Run or Attempt is generated", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal("navigator", { clipboard: { writeText } });
  const { client } = mount(location, {
    ...candidate,
    execution_id: "",
    attempt_id: "",
  });
  fireEvent.click(
    screen.getByRole("button", { name: "复制 SteerRunner 执行信息" }),
  );
  await waitFor(() =>
    expect(screen.getByRole("status")).toHaveTextContent("已复制"),
  );
  expect(writeText).toHaveBeenLastCalledWith(
    [
      "SteerRunner",
      `Executable: ${location.runner.executable}`,
      `State directory: ${candidate.runner_state_dir}`,
      "Runner Run ID: 尚未生成",
      "Attempt ID: 尚未生成",
    ].join("\n"),
  );
  client.clear();
});

it("only shows the Runner button when Runner is configured", () => {
  const { client } = mount({ ...location, runner: undefined });
  expect(
    screen.getByRole("button", { name: "复制 SteerLine 执行信息" }),
  ).toBeEnabled();
  expect(
    screen.queryByRole("button", { name: "复制 SteerRunner 执行信息" }),
  ).not.toBeInTheDocument();
  client.clear();
});

it("keeps the SteerLine button disabled until location is available", () => {
  const { client } = mount();
  expect(
    screen.getByRole("button", { name: "复制 SteerLine 执行信息" }),
  ).toBeDisabled();
  expect(
    screen.queryByRole("button", { name: "复制 SteerRunner 执行信息" }),
  ).not.toBeInTheDocument();
  client.clear();
});

it.each([false, true])(
  "reports clipboard failure and allows retry (unsupported=%s)",
  async (unsupported) => {
    const writeText = vi.fn().mockRejectedValue(new Error("permission denied"));
    vi.stubGlobal("navigator", unsupported ? {} : { clipboard: { writeText } });
    const { client } = mount(location);
    fireEvent.click(
      screen.getByRole("button", { name: "复制 SteerLine 执行信息" }),
    );
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("复制失败"),
    );
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    const retry = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { clipboard: { writeText: retry } });
    fireEvent.click(
      screen.getByRole("button", { name: "复制 SteerLine 执行信息" }),
    );
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent("已复制"),
    );
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    client.clear();
  },
);
