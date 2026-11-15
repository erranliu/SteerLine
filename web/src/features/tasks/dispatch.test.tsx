import { afterEach, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { projectQueries } from "../../shared/api/queries";
import type { DispatchObservation } from "../../shared/api/types";
import {
  identity,
  mountFeature,
  snapshot,
  testClient,
} from "../../test/workspace";
import { Dispatch, DispatchStatus } from "./dispatch";

afterEach(cleanup);

const ready: DispatchObservation = {
  mode: "auto",
  revision: 1,
  state: "ready",
  default_profile_id: "default",
  default_profile_name: "默认方案",
  environment_readiness_required: true,
};

it("allows enabling automatic dispatch before the readiness observation has returned", () => {
  const api = new Api(identity, "csrf");
  const client = testClient();
  const read = vi.spyOn(api, "get").mockReturnValue(new Promise(() => {}));
  const send = vi.fn().mockResolvedValue(true);
  mountFeature(<Dispatch />, api, client, true, { send });
  const toggle = screen.getByRole("switch", { name: "自动派发" });
  expect(toggle).toBeEnabled();
  expect(toggle).toHaveAttribute("aria-checked", "false");
  expect(read.mock.calls.map(([path]) => path)).toEqual([
    "/dispatch",
    "/settings/worktrees",
  ]);
  fireEvent.click(toggle);
  expect(send).toHaveBeenCalledWith(
    "set_dispatch_mode",
    "dispatch",
    1,
    { mode: "auto" },
    { expected_control_revision: 1 },
  );
  client.clear();
});

it("saves and restores the worktree directory in project settings", async () => {
  const api = new Api(identity, "csrf");
  let settings = {
    directory: "",
    effective: "C:/state",
    default_directory: "C:/state",
  };
  vi.spyOn(api, "get").mockImplementation(
    async (path) =>
      (path === "/settings/worktrees" ? settings : ready) as never,
  );
  const submit = vi
    .spyOn(api, "submit")
    .mockImplementation(async (envelope) => {
      const directory = envelope.payload.directory as string;
      settings = {
        ...settings,
        directory,
        effective: directory ? directory + "/project" : "C:/state",
      };
      return { status: "applied" } as never;
    });
  mountFeature(<Dispatch />, api, testClient());
  expect(
    screen.getByRole("heading", { name: "项目设置", level: 1 }),
  ).toBeVisible();
  const directory = screen.getByLabelText("目录");
  await waitFor(() => expect(directory).toBeEnabled());
  fireEvent.change(directory, { target: { value: "D:/worktrees" } });
  fireEvent.click(screen.getByRole("button", { name: "保存目录" }));
  await screen.findByText("D:/worktrees/project");
  expect(submit.mock.calls[0][0]).toMatchObject({
    action: "maintenance_set_worktree_directory",
    target_id: identity.project_id,
    payload: { directory: "D:/worktrees" },
  });
  const restore = screen.getByRole("button", { name: "恢复默认目录" });
  await waitFor(() => expect(restore).toBeEnabled());
  fireEvent.click(restore);
  await screen.findByText("C:/state");
  expect(submit.mock.calls[1][0].payload).toEqual({ directory: "" });
});

it("uses the same server verdict in settings and the header and can turn off while waiting", async () => {
  const api = new Api(identity, "csrf");
  const client = testClient();
  const waiting: DispatchObservation = {
    ...ready,
    state: "waiting",
    blocker: {
      category: "project",
      code: "project_not_ready",
      diagnostic: "任务 #99 的材料与测试安排未就绪",
    },
  };
  vi.spyOn(api, "get").mockResolvedValue(waiting);
  const observation = snapshot(api);
  observation.connection.snapshot.page!.control.dispatch_mode = "auto";
  const send = vi.fn().mockResolvedValue(true);
  mountFeature(
    <>
      <DispatchStatus />
      <Dispatch />
    </>,
    api,
    client,
    true,
    {
      send,
      snapshot: observation,
    },
  );
  await screen.findByText(waiting.blocker!.diagnostic, { exact: false });
  expect(
    screen.getByRole("link", { name: "任务自动派发：已开启，等待就绪" }),
  ).toBeVisible();
  expect(screen.getByRole("link", { name: "前往项目状态" })).toHaveAttribute(
    "href",
    "/projects/project/status",
  );
  const toggle = screen.getByRole("switch", { name: "自动派发" });
  expect(toggle).toBeEnabled();
  expect(toggle).toHaveAttribute("aria-checked", "true");
  fireEvent.click(toggle);
  expect(send).toHaveBeenCalledWith(
    "set_dispatch_mode",
    "dispatch",
    1,
    { mode: "manual" },
    { expected_control_revision: 1 },
  );

  const key = projectQueries(api).dispatch(1).queryKey;
  await act(async () => client.setQueryData(key, ready));
  expect(
    await screen.findByRole("link", { name: "任务自动派发：已开启，可派发" }),
  ).toBeVisible();
  await act(async () => client.setQueryData(key, { ...ready, revision: 2 }));
  expect(
    await screen.findByRole("link", {
      name: "任务自动派发：已开启，状态待核实",
    }),
  ).toBeVisible();
  await act(async () =>
    client.setQueryData(key, {
      ...waiting,
      blocker: {
        category: "environment",
        code: "environment_not_ready",
        diagnostic: "默认环境尚未检测",
      },
    }),
  );
  expect(
    await screen.findByRole("link", { name: "前往环境准备" }),
  ).toHaveAttribute("href", "/projects/project/environment?profile=default");
  client.clear();
});
