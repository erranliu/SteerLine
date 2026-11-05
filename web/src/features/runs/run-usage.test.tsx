import { afterEach, expect, it, vi } from "vitest";
import { useState } from "react";
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { projectQueries } from "../../shared/api/queries";
import type { RunUsageView } from "../../shared/api/run-usage";
import { identity, mountFeature, run, testClient } from "../../test/workspace";
import { formatAmount, RunUsage, usagePollInterval } from "./run-usage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});
function view(): RunUsageView {
  return {
    run_id: run.run_id,
    execution_id: run.execution_id,
    usage: {
      snapshot_revision: 7,
      status: "partial",
      scope: "main_session",
      tokens: {
        input_total: 210000,
        input_uncached: 10000,
        cache_read: 200000,
        cache_write: 0,
        output: 15453,
        reasoning: null,
        total: 225453,
      },
      finalized: false,
      counted_responses: 11,
      issues: [
        { category: "usage", code: "USAGE_NOT_FINALIZED", retryable: true },
      ],
      observed_at: "2026-10-22T06:32:08Z",
      access_cleanup_status: "active",
    },
    cost: {
      status: "partial",
      usage_revision: 7,
      pricing_policy: "default",
      pricing_policy_version: "2026-10-22",
      pricing_policy_digest: "digest",
      pricing_source: "独立合成测试价",
      matches: [
        {
          rule_id: "r",
          provider: "synthetic",
          model: "fixture",
          records: 10,
          amount: "0.05327400",
        },
      ],
      currency: "USD",
      covered_estimated_amount: "0.05327400",
      total_estimated_amount: null,
      fully_priced_records: 10,
      partially_priced_records: 0,
      record_count: 11,
      settings_generation: 1,
      unpriced_records: 1,
      assumptions: [],
      issues: ["PRICE_NOT_FOUND"],
    },
  };
}
it("shows partial amount, nullable tokens and source time in a compact disclosure", async () => {
  const api = new Api(identity, "csrf", "home");
  const client = testClient();
  vi.spyOn(api, "get").mockResolvedValue(view());
  mountFeature(<RunUsage run={run} />, api, client);
  expect(await screen.findByText("已计价部分 ≈ USD 0.0533")).toBeVisible();
  expect(screen.getByText("Token 225,453 · 已核实部分")).toBeVisible();
  expect(screen.getByText("10 / 11")).not.toBeVisible();
  fireEvent.click(screen.getByText("已计价部分 ≈ USD 0.0533"));
  expect(screen.getByText("10 / 11")).toBeVisible();
  expect(screen.getByText("200,000 / 0")).toBeVisible();
  expect(screen.getByText("部分模型缺少适用价格")).toBeVisible();
  expect(screen.getByText("独立合成测试价")).toBeVisible();
  expect(screen.getByText(/synthetic \/ fixture/)).toBeVisible();
  expect(document.querySelector("time")?.dateTime).toBe("2026-10-22T06:32:08Z");
  expect(screen.getByText(/按配置价表估算，非实际账单/)).toBeVisible();
  client.clear();
});
it("keeps tokens without a price table; unknown and proven zero remain distinct", async () => {
  const api = new Api(identity, "csrf", "home");
  const client = testClient();
  const sample = view();
  sample.cost = null;
  sample.cost_reason = "PRICE_POLICY_NOT_CONFIGURED";
  const get = vi.spyOn(api, "get").mockResolvedValue(structuredClone(sample));
  const mounted = mountFeature(<RunUsage run={run} />, api, client);
  expect(await screen.findByText("Token 225,453 · 已核实部分")).toBeVisible();
  expect(screen.getByRole("link", { name: /未配置价表/ })).toBeVisible();
  sample.usage!.tokens = null;
  sample.usage!.status = "unavailable";
  get.mockResolvedValue(structuredClone(sample));
  await act(() => client.invalidateQueries());
  expect(await screen.findByText("Token —")).toBeVisible();
  sample.usage!.status = "available";
  sample.usage!.finalized = true;
  sample.usage!.tokens = {
    input_total: 0,
    input_uncached: 0,
    cache_read: 0,
    cache_write: 0,
    output: 0,
    reasoning: null,
    total: 0,
  };
  sample.cost = view().cost!;
  sample.cost.status = "estimated";
  sample.cost.total_estimated_amount = "0.00000000";
  get.mockResolvedValue(structuredClone(sample));
  await act(() => client.invalidateQueries());
  expect(await screen.findByText("费用估算 ≈ USD 0.0000")).toBeVisible();
  expect(screen.getByText("Token 0")).toBeVisible();
  mounted.unmount();
  client.clear();
});
it("polls visible partial data, retains the last amount on failure, and stops after finalization", async () => {
  vi.useFakeTimers();
  const visibility = vi
    .spyOn(document, "visibilityState", "get")
    .mockReturnValue("visible");
  const api = new Api(identity, "csrf", "home");
  const client = testClient();
  const sample = view();
  const get = vi.spyOn(api, "get").mockResolvedValue(sample);
  mountFeature(<RunUsage run={run} />, api, client);
  await act(() => vi.advanceTimersByTimeAsync(1));
  await act(() => vi.advanceTimersByTimeAsync(5100));
  expect(get).toHaveBeenCalledTimes(2);
  get.mockRejectedValue(new Error("offline"));
  await act(() => vi.advanceTimersByTimeAsync(5100));
  expect(screen.getByText("已计价部分 ≈ USD 0.0533")).toBeVisible();
  expect(screen.getByText(/用量刷新失败/)).toBeVisible();
  visibility.mockReturnValue("hidden");
  fireEvent(document, new Event("visibilitychange"));
  const calls = get.mock.calls.length;
  await act(() => vi.advanceTimersByTimeAsync(15000));
  expect(get).toHaveBeenCalledTimes(calls);
  sample.usage!.status = "available";
  sample.usage!.finalized = true;
  sample.cost!.status = "estimated";
  sample.cost!.total_estimated_amount = "0.05327400";
  get.mockResolvedValue(structuredClone(sample));
  visibility.mockReturnValue("visible");
  fireEvent(document, new Event("visibilitychange"));
  await act(() => vi.advanceTimersByTimeAsync(5100));
  expect(screen.getByText("费用估算 ≈ USD 0.0533")).toBeVisible();
  const finalized = get.mock.calls.length;
  await act(() => vi.advanceTimersByTimeAsync(15000));
  expect(get).toHaveBeenCalledTimes(finalized);
  client.clear();
});
it("rejects foreign identities or mixed snapshots", async () => {
  const api = new Api(identity, "csrf", "home");
  const client = testClient();
  const get = vi.spyOn(api, "get");
  get.mockResolvedValue({ ...view(), execution_id: "foreign" });
  await expect(
    client.fetchQuery(projectQueries(api).runUsage(run)),
  ).rejects.toThrow("快照已变化");
  const mixed = view();
  mixed.cost!.usage_revision++;
  get.mockResolvedValue(mixed);
  await expect(
    client.fetchQuery(projectQueries(api).runUsage(run)),
  ).rejects.toThrow("快照已变化");
  expect(
    projectQueries(api).runUsage({ ...run, current_turn_id: "new-turn" })
      .queryKey,
  ).not.toEqual(projectQueries(api).runUsage(run).queryKey);
  client.clear();
});
it("formats exact decimal amounts and keeps incomplete coverage polling", () => {
  expect(formatAmount("0.00000001", "USD")).toBe("< USD 0.0001");
  expect(formatAmount("999999999999999999.99995000", "USD")).toBe(
    "USD 1000000000000000000.0000",
  );
  expect(usagePollInterval(view())).toBe(5000);
  const unsupported = view();
  unsupported.usage!.status = "unavailable";
  unsupported.usage!.issues = [
    {
      category: "usage",
      code: "PRODUCTION_BINDING_UNSUPPORTED",
      retryable: false,
    },
  ];
  expect(usagePollInterval(unsupported)).toBe(false);
  expect(usagePollInterval({ ...view(), reason: "USAGE_UNSUPPORTED" })).toBe(
    false,
  );
});

it("refreshes finalized data when another tab changes prices or the effective collection policy", async () => {
  vi.useFakeTimers();
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
  let generation = 1;
  let policyRevision = "instance-1/policy-1";
  vi.spyOn(globalThis, "fetch").mockImplementation(
    async (input) =>
      new Response(
        JSON.stringify(
          String(input).includes("billing-config")
            ? { generation }
            : { usage_policy_revision: policyRevision },
        ),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
  );
  const api = new Api(identity, "csrf", "home");
  const client = testClient();
  const sample = view();
  sample.usage!.status = "available";
  sample.usage!.finalized = true;
  sample.cost!.status = "estimated";
  sample.cost!.total_estimated_amount = "0.100000000000000000";
  const get = vi.spyOn(api, "get").mockImplementation(async () => ({
    ...structuredClone(sample),
    cost: { ...sample.cost!, settings_generation: generation },
  }));
  mountFeature(<RunUsage run={run} />, api, client);
  await act(() => vi.advanceTimersByTimeAsync(100));
  const finalizedCalls = get.mock.calls.length;
  await act(() => vi.advanceTimersByTimeAsync(5100));
  expect(get).toHaveBeenCalledTimes(finalizedCalls);
  generation = 2;
  sample.cost!.total_estimated_amount = "0.200000000000000000";
  await act(() => vi.advanceTimersByTimeAsync(5100));
  expect(screen.getByText("费用估算 ≈ USD 0.2000")).toBeVisible();
  expect(get.mock.calls.length).toBeGreaterThan(finalizedCalls);
  const repricedCalls = get.mock.calls.length;
  policyRevision = "instance-2/policy-2";
  await act(() => vi.advanceTimersByTimeAsync(5100));
  expect(get.mock.calls.length).toBeGreaterThan(repricedCalls);
  const keys = client
    .getQueryCache()
    .findAll()
    .map((q) => q.queryKey);
  expect(
    keys.some((key) => key.includes(2) && key.includes(policyRevision)),
  ).toBe(true);
  client.clear();
});

it("rejects an old price generation instead of publishing it under a new cache key", async () => {
  const api = new Api(identity, "csrf", "home");
  const client = testClient();
  vi.spyOn(api, "get").mockResolvedValue(view());
  await expect(
    client.fetchQuery(projectQueries(api).runUsage(run, 2, "new-policy")),
  ).rejects.toThrow();
  expect(
    client.getQueryData(
      projectQueries(api).runUsage(run, 2, "new-policy").queryKey,
    ),
  ).toBeUndefined();
  client.clear();
});

it("retains the original price version on reprice failure without carrying it to a different candidate", async () => {
  vi.useFakeTimers();
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
  let generation = 1;
  vi.spyOn(globalThis, "fetch").mockImplementation(
    async (input) =>
      new Response(
        JSON.stringify(
          String(input).includes("billing-config")
            ? { generation }
            : { usage_policy_revision: "policy" },
        ),
        { status: 200 },
      ),
  );
  const api = new Api(identity, "csrf", "home");
  const client = testClient();
  const original = view();
  original.usage!.status = "available";
  original.usage!.finalized = true;
  original.cost!.status = "estimated";
  original.cost!.total_estimated_amount = "0.1";
  const get = vi.spyOn(api, "get").mockResolvedValue(original);
  function Selection() {
    const [selected, setSelected] = useState(run);
    return (
      <>
        <button
          onClick={() => setSelected({ ...run, run_id: "different-candidate" })}
        >
          切换测试候选
        </button>
        <RunUsage run={selected} />
      </>
    );
  }
  mountFeature(<Selection />, api, client);
  await act(() => vi.advanceTimersByTimeAsync(100));
  expect(screen.getByText("费用估算 ≈ USD 0.1000")).toBeVisible();
  get.mockRejectedValue(new Error("reprice offline"));
  generation = 2;
  await act(() => vi.advanceTimersByTimeAsync(5100));
  expect(screen.getByText("费用估算 ≈ USD 0.1000")).toBeVisible();
  expect(screen.getByText(/暂显示原价表版本/)).toBeVisible();
  expect(
    client.getQueryData(
      projectQueries(api).runUsage(run, 2, "policy").queryKey,
    ),
  ).toBeUndefined();
  fireEvent.click(screen.getByRole("button", { name: "切换测试候选" }));
  await act(() => vi.advanceTimersByTimeAsync(100));
  expect(screen.queryByText("费用估算 ≈ USD 0.1000")).toBeNull();
  client.clear();
});
