import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
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
        const config = JSON.parse(String(init.body));
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
