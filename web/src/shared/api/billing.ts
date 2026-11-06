import { json } from "./client";
export interface PriceRule {
  id: string;
  provider: string;
  model: string;
  effective_from: string;
  effective_until: string | null;
  rates: {
    input_uncached_per_million: string;
    cache_read_per_million: string;
    cache_write_per_million: string;
    output_per_million: string;
  };
}
export interface PricePolicy {
  id?: string;
  version?: string;
  created_at?: string;
  source: string;
  source_url?: string;
  currency: string;
  basis: "manual_flat_token_estimate";
  rules: PriceRule[];
}
export interface BillingSnapshot {
  schema: string;
  generation: number;
  active_policy_version: string;
  policies: PricePolicy[];
  saved_digest: string;
  configured: boolean;
}
export interface BillingStatus {
  saved_digest: string;
  generation: number;
  active_policy_version: string;
}
const path = "/api/web/billing-config";
export const billingStatus = (signal?: AbortSignal) =>
  json<BillingStatus>(path + "/status", { signal });
export function billingAPI(csrf: string) {
  const headers = { "Content-Type": "application/json", "X-CSRF-Token": csrf };
  return {
    read: async (signal?: AbortSignal) => {
      const v = await json<BillingSnapshot>(path, { signal });
      if (
        v.schema !== "steerline-billing/v1" ||
        !Array.isArray(v.policies) ||
        typeof v.configured !== "boolean"
      )
        throw new Error("计费配置响应格式无效");
      return v;
    },
    save: (policy: PricePolicy, base: BillingSnapshot) =>
      json<BillingSnapshot>(path, {
        method: "PUT",
        headers: {
          ...headers,
          ...(base.configured
            ? { "If-Match": `"${base.saved_digest}"` }
            : { "If-None-Match": "*" }),
        },
        body: JSON.stringify(policy),
      }),
    activate: (version: string, base: BillingSnapshot) =>
      json<BillingSnapshot>(path + "/activate", {
        method: "POST",
        headers: { ...headers, "If-Match": `"${base.saved_digest}"` },
        body: JSON.stringify({ version }),
      }),
  };
}
