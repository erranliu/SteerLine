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
import type { ProjectReadiness } from "../../shared/api/types";
import {
  identity,
  mountFeature,
  snapshot,
  testClient,
} from "../../test/workspace";
import { Dispatch } from "./dispatch";

afterEach(cleanup);

it("gates automatic dispatch on the shared project verdict and keeps ready stable during refresh", async () => {
  const api = new Api(identity, "csrf");
  const client = testClient();
  const key = projectQueries(api).projectReadiness().queryKey;
  const ready: ProjectReadiness = {
    status: "ready",
    commit: "base",
    total: 2,
    ready: 2,
    blockers: [],
  };
  let resolve!: (value: ProjectReadiness) => void;
  vi.spyOn(api, "get").mockImplementation((path) =>
    path === "/maintenance/readiness"
      ? (new Promise((finish) => {
          resolve = finish;
        }) as never)
      : (Promise.resolve({ profiles: [], environments: [] }) as never),
  );
  client.setQueryData(key, {
    ...ready,
    status: "not_ready",
    ready: 1,
    blockers: [{ category: "task_material", code: "task_material_unprepared", diagnostic: "任务 #99 的材料与测试安排未就绪" }],
  });
  const send = vi.fn().mockResolvedValue(true);
  mountFeature(<Dispatch />, api, client, true, { send });
  const toggle = screen.getByRole("switch", { name: "自动派发" });
  expect(toggle).toBeDisabled();
  fireEvent.click(toggle);
  expect(send).not.toHaveBeenCalled();
  await act(async () => {
    client.setQueryData(key, ready);
  });
  await waitFor(() => expect(toggle).toBeEnabled());
  expect(toggle).toHaveAttribute("aria-checked", "false");
  fireEvent.click(toggle);
  expect(send).toHaveBeenCalledWith(
    "set_dispatch_mode",
    "dispatch",
    1,
    { mode: "auto" },
    { expected_control_revision: 1 },
  );
  let refresh!: Promise<unknown>;
  await act(async () => {
    refresh = client.refetchQueries({ queryKey: key });
  });
  expect(toggle).toBeEnabled();
  await act(async () => {
    resolve({
      ...ready,
      status: "not_ready",
      blockers: [{ category: "worktree", code: "local_behind_remote", diagnostic: "本地尚未与目标分支同步" }],
    });
    await refresh;
  });
  await waitFor(() => expect(toggle).toBeDisabled());
  client.clear();
});

it("allows turning automatic dispatch off while the project is unready", async () => {
  const api = new Api(identity, "csrf");
  const client = testClient();
  vi.spyOn(api, "get").mockResolvedValue({
    status: "not_ready",
    blockers: [],
    profiles: [],
    environments: [],
  } as never);
  const observation = snapshot(api);
  observation.connection.snapshot.page!.control.dispatch_mode = "auto";
  const send = vi.fn().mockResolvedValue(true);
  mountFeature(<Dispatch />, api, client, true, {
    send,
    snapshot: observation,
  });
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
  client.clear();
});
