export interface UsageTokens {
  input_total: number | null;
  input_uncached: number | null;
  cache_read: number | null;
  cache_write: number | null;
  output: number | null;
  reasoning: number | null;
  total: number | null;
}
export interface RunUsageView {
  run_id: string;
  execution_id: string;
  reason?: string;
  cost_reason?: string;
  usage: null | {
    snapshot_revision: number;
    status: "pending" | "available" | "partial" | "unavailable";
    scope: string;
    tokens: UsageTokens | null;
    finalized: boolean;
    counted_responses: number;
    issues: string[];
    access_cleanup_status: string;
    observed_at: string;
  };
  cost: null | {
    status: "estimated" | "partial" | "unavailable";
    usage_revision: number;
    pricing_policy: string;
    pricing_policy_version: string;
    pricing_policy_digest: string;
    currency: string;
    covered_estimated_amount: string | null;
    total_estimated_amount: string | null;
    priced_responses: number;
    unpriced_responses: number;
    assumptions: string[] | null;
    issues: string[] | null;
  };
}
