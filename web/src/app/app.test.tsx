import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor, act } from "@testing-library/react";
import {
  onlineManager,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { Identity } from "../shared/api/types";
import { App } from "./app";
import { Api } from "../shared/api/client";
import { saveRequest, listRequests } from "../shared/api/journal";
import type { Receipt } from "../shared/api/types";

afterEach(() => {
  cleanup();
  onlineManager.setOnline(true);
  vi.unstubAllGlobals();
});

it("runs only one reconciliation round while journal observations refresh and aborts it offline", async () => {
  const identity = {
    project_id: "poll-project",
    instance_id: "poll-instance",
    repository_identity: "repo",
    ready: true,
  } as Identity;
  const envelope = new Api(identity, "csrf").envelope("start_task", "1", 1, {});
  await saveRequest(envelope);
  let resolve!: (receipt: Receipt) => void;
  const reconcile = vi.spyOn(Api.prototype, "reconcile").mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) =>
      Response.json(
        url === "/auth/status"
          ? { identity, csrf_token: "csrf" }
          : url.endsWith("/snapshot")
            ? {
                connection: {
                  state: "connected",
                  can_control: true,
                  identity_matched: true,
                  config_matched: true,
                  snapshot: {
                    page: {
                      service: { instance_id: identity.instance_id },
                      runs: [],
                      control: {
                        revision: 1,
                        dispatch_mode: "manual",
                        dispatch_paused: false,
                      },
                    },
                  },
                },
              }
            : { presentation: { profiles: [], default_profile_id: "" } },
      ),
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/settings"]}>
        <App initial={{ identity, csrf_token: "csrf" }} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  await waitFor(() => expect(reconcile).toHaveBeenCalledTimes(1));
  const rows = await listRequests();
  for (let i = 1; i <= 5; i++)
    await act(async () => {
      client.setQueryData(["local-requests"], rows, {
        updatedAt: Date.now() + i,
      });
    });
  expect(reconcile).toHaveBeenCalledTimes(1);
  const signal = reconcile.mock.calls[0][1]!;
  act(() => onlineManager.setOnline(false));
  expect(signal.aborted).toBe(true);
  await act(async () => {
    resolve({
      ...envelope,
      original: envelope,
      status: "accepted",
      sampled_at: "now",
    });
  });
  expect(reconcile).toHaveBeenCalledTimes(1);
  client.clear();
});

it("disables commands when offline even while paused queries retain a successful snapshot", async () => {
  const identity = {
    project_id: "offline-project",
    instance_id: "original",
    repository_identity: "repo",
    ready: true,
  } as Identity;
  const snapshot = {
    connection: {
      state: "connected",
      can_control: true,
      identity_matched: true,
      config_matched: true,
      snapshot: {
        page: {
          service: {
            instance_id: "original",
            ready: true,
            capabilities: ["pause_dispatch", "set_dispatch_mode"],
          },
          runs: [],
          control: {
            revision: 1,
            dispatch_mode: "manual",
            dispatch_paused: false,
            blockers: [],
          },
        },
      },
    },
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) =>
      Response.json(
        url === "/auth/status"
          ? { identity, csrf_token: "csrf" }
          : url.endsWith("/snapshot")
            ? snapshot
            : { presentation: { profiles: [], default_profile_id: "" } },
      ),
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/settings"]}>
        <App initial={{ identity, csrf_token: "csrf" }} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  await waitFor(() =>
    expect(screen.getByText("服务已连接", { exact: true })).toBeVisible(),
  );
  expect(screen.getByRole("button", { name: "暂停派发" })).toBeEnabled();
  act(() => onlineManager.setOnline(false));
  expect(
    screen.getByText("当前不可提交业务操作", { exact: true }),
  ).toBeVisible();
  expect(screen.getByRole("button", { name: "暂停派发" })).toBeDisabled();
  expect(
    client.getQueryData([
      identity.project_id,
      identity.instance_id,
      "snapshot",
    ]),
  ).toEqual(snapshot);
  act(() => onlineManager.setOnline(true));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "暂停派发" })).toBeEnabled(),
  );
  client.clear();
});
