import { afterEach, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { UsageSettings } from "./usage-settings";
import { configSnapshot } from "../../test/execution-config";
import { defaultUsagePolicy } from "../../shared/api/usage-collection";
const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((c) => c.clear());
  vi.unstubAllGlobals();
});
it("saves only the observed exact leaf and keeps actual remote authorization disabled until restart", async () => {
  let saved = structuredClone(configSnapshot);
  const writes: unknown[] = [];
  const root = "C:/Users/fixture/.codex/sessions/2026/11/05";
  const host = "sha256:" + "a".repeat(64);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url, init) => {
      if (String(url).includes("usage-collection"))
        return new Response(
          JSON.stringify({
            status: "ready",
            instance_id: "old",
            usage_policy: defaultUsagePolicy(),
            usage_policy_revision: "old",
            usage_policy_digest: "old",
            requirements: [
              {
                run_id: "r",
                source_ref: "s",
                environment: "env",
                host_id: host,
                session_root: root,
                observed_at: "2026-11-05T00:00:00Z",
              },
            ],
          }),
        );
      if (init?.method === "PUT") {
        const edit = JSON.parse(String(init.body));
        expect(edit.saved_digest).toBe(saved.saved_digest);
        expect(edit.profile_edits).toEqual([]);
        expect(new Headers(init.headers).get("If-Match")).toBe(
          '"' + saved.saved_digest + '"',
        );
        const config = edit.config;
        writes.push(config);
        saved = { ...saved, config, saved_digest: "new" };
      }
      return new Response(JSON.stringify(saved));
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <UsageSettings home="isolated" csrf="csrf" applicationConnected />
    </QueryClientProvider>,
  );
  await screen.findByText("运行中策略已生效");
  expect(
    screen.getByRole("button", { name: "授权此已核实日志叶目录" }),
  ).toBeDisabled();
  fireEvent.click(screen.getByLabelText("显式允许临时远端日志读取"));
  fireEvent.click(
    screen.getByRole("button", { name: "授权此已核实日志叶目录" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存采集设置" }));
  await screen.findByText("已保存，待正式重启");
  expect(writes).toEqual([
    {
      ...configSnapshot.config,
      usage_policy: {
        enabled: true,
        remote_log_access: {
          mode: "temporary",
          grants: [
            { environment: "env", host_id: host, session_roots: [root] },
          ],
        },
      },
    },
  ]);
  expect(
    screen.queryByRole("button", { name: /重启/ }),
  ).not.toBeInTheDocument();
});

it("does not let a pre-save read claim that changed collection settings are already effective", async () => {
  let saved = structuredClone(configSnapshot);
  let pending = false;
  let finish!: (value: Response) => void;
  let signal: AbortSignal | undefined;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url, init) => {
      if (String(url).includes("usage-collection"))
        return new Response(
          JSON.stringify({
            status: "ready",
            instance_id: "old",
            usage_policy: defaultUsagePolicy(),
            usage_policy_revision: "old",
            requirements: [],
          }),
        );
      if (init?.method === "PUT") {
        const edit = JSON.parse(String(init.body));
        expect(edit.saved_digest).toBe(saved.saved_digest);
        expect(edit.profile_edits).toEqual([]);
        expect(new Headers(init.headers).get("If-Match")).toBe(
          '"' + saved.saved_digest + '"',
        );
        saved = {
          ...saved,
          config: edit.config,
          saved_digest: "new",
        };
        return new Response(JSON.stringify(saved));
      }
      if (pending) {
        pending = false;
        signal = init?.signal as AbortSignal;
        return new Promise<Response>((resolve) => {
          finish = resolve;
        });
      }
      return new Response(JSON.stringify(saved));
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <UsageSettings home="race" csrf="csrf" applicationConnected />
    </QueryClientProvider>,
  );
  await screen.findByText("运行中策略已生效");
  const old = structuredClone(saved);
  pending = true;
  let read!: Promise<unknown>;
  await act(async () => {
    read = client.refetchQueries({ queryKey: ["race", "execution-config"] });
  });
  fireEvent.click(screen.getByLabelText("启用 Token 采集"));
  fireEvent.click(screen.getByRole("button", { name: "保存采集设置" }));
  await screen.findByText("已保存，待正式重启");
  expect(signal?.aborted).toBe(true);
  await act(async () => {
    finish(new Response(JSON.stringify(old)));
    await read;
  });
  expect(screen.getByLabelText("启用 Token 采集")).not.toBeChecked();
  expect(screen.getByText("已保存，待正式重启")).toBeVisible();
  fireEvent.click(screen.getByLabelText("启用 Token 采集"));
  fireEvent.click(screen.getByRole("button", { name: "保存采集设置" }));
  await screen.findByText("运行中策略已生效");
  expect(screen.queryByText(/采集设置已保存，待正式重启/)).toBeNull();
});
