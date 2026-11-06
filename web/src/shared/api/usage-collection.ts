import { json } from "./client";
export interface UsageGrant {
  environment: string;
  host_id: string;
  session_roots: string[];
}
export interface UsagePolicy {
  enabled: boolean;
  remote_log_access: { mode: "disabled" | "temporary"; grants: UsageGrant[] };
}
export interface UsageRequirement {
  run_id: string;
  source_ref: string;
  environment: string;
  host_id: string;
  session_root: string;
  observed_at: string;
}
export interface UsageCollection {
  status: string;
  instance_id: string;
  usage_policy: UsagePolicy;
  usage_policy_digest: string;
  usage_policy_revision: string;
  requirements: UsageRequirement[];
  diagnostic?: string;
}
export const defaultUsagePolicy = (): UsagePolicy => ({
  enabled: true,
  remote_log_access: { mode: "disabled", grants: [] },
});
export const usageCollection = (signal?: AbortSignal, statusOnly = false) =>
  json<UsageCollection>(
    "/api/web/usage-collection" + (statusOnly ? "/status" : ""),
    { signal },
  );
