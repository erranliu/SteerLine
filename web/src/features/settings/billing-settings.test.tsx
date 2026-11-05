import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BillingSettings } from "./billing-settings";
import type { BillingSnapshot, PricePolicy } from "../../shared/api/billing";
const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((c) => c.clear());
  vi.unstubAllGlobals();
});
function mount(conflict = false) {
  let saved: BillingSnapshot = {
    schema: "steerline-billing/v1",
    generation: 0,
    active_policy_version: "",
    policies: [],
    saved_digest: "",
    configured: false,
  };
  const writes: { policy: PricePolicy; headers: Headers }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url, init) => {
      if (init?.method === "PUT") {
        const policy = JSON.parse(String(init.body));
        writes.push({ policy, headers: new Headers(init.headers) });
        if (conflict)
          return new Response(
            JSON.stringify({
              code: "configuration_changed",
              reason: "配置已变化",
            }),
            { status: 412 },
          );
        saved = {
          ...saved,
          configured: true,
          generation: 1,
          active_policy_version: "new",
          policies: [{ ...policy, id: "billing", version: "new" }],
          saved_digest: "new-etag",
        };
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
      <BillingSettings
        home="isolated-home"
        csrf="csrf"
        applicationConnected={false}
      />
    </QueryClientProvider>,
  );
  return writes;
}
it("creates a manual price version offline with explicit rates and a create-only precondition", async () => {
  const writes = mount();
  await screen.findByRole("button", { name: "添加模型费率" });
  fireEvent.change(screen.getByLabelText("价格来源"), {
    target: { value: "synthetic fixture" },
  });
  fireEvent.click(screen.getByRole("button", { name: "添加模型费率" }));
  for (const [label, value] of [
    ["Provider 1", "fixture"],
    ["实际模型 1", "model"],
    ["生效时间 1", "2026-11-01T00:00:00Z"],
    ["非缓存输入 1", "2"],
    ["缓存读取 1", "0.2"],
    ["缓存写入 1", "0"],
    ["输出（含推理） 1", "4"],
  ])
    fireEvent.change(screen.getByLabelText(label), { target: { value } });
  fireEvent.click(screen.getByRole("button", { name: "保存为新价表版本" }));
  await screen.findByText(/价表已保存并立即用于重算/);
  expect(writes).toHaveLength(1);
  expect(writes[0].headers.get("If-None-Match")).toBe("*");
  expect(writes[0].headers.get("X-CSRF-Token")).toBe("csrf");
  expect(writes[0].policy.rules[0].rates.cache_read_per_million).toBe("0.2");
  expect(screen.getByRole("combobox", { name: "价表版本" })).toHaveValue("new");
});
it("retains a conflicting draft until the user reloads", async () => {
  const writes = mount(true);
  await screen.findByLabelText("价格来源");
  fireEvent.change(screen.getByLabelText("价格来源"), {
    target: { value: "my draft" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存为新价表版本" }));
  await screen.findByRole("alert");
  expect(screen.getByLabelText("价格来源")).toHaveValue("my draft");
  expect(writes).toHaveLength(1);
  fireEvent.click(
    screen.getByRole("button", { name: "重新读取（放弃未保存修改）" }),
  );
  await waitFor(() =>
    expect(screen.getByLabelText("价格来源")).toHaveValue(""),
  );
});
