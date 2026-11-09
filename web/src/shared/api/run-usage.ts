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
    issues: {
      category: string;
      code: string;
      retryable: boolean;
      source_ref?: string;
    }[];
    access_cleanup_status: string;
    observed_at: string;
  };
  cost: null | {
    status: "estimated" | "partial" | "unavailable";
    usage_revision: number;
    pricing_policy: string;
    pricing_policy_version: string;
    pricing_policy_digest: string;
    pricing_source: string;
    price_at: string;
    matches: {
      rule_id: string;
      step_id: string;
      configured_reasoning_effort: string;
      provider: string;
      model: string;
      records: number;
      amount: string;
    }[];
    currency: string;
    covered_estimated_amount: string | null;
    total_estimated_amount: string | null;
    fully_priced_records: number;
    partially_priced_records: number;
    unpriced_records: number;
    record_count: number;
    settings_generation: number;
    assumptions: string[] | null;
    issues: string[] | null;
  };
}
