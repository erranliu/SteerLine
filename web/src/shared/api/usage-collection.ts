import { json } from "./client";

export interface UsagePolicy {
  enabled: boolean;
}
export interface UsageCollection {
  status: string;
  instance_id: string;
  usage_policy: UsagePolicy;
  usage_policy_digest: string;
  usage_policy_revision: string;
}
export const defaultUsagePolicy = (): UsagePolicy => ({ enabled: true });
export const usageCollection = (signal?: AbortSignal, statusOnly = false) =>
  json<UsageCollection>(
    "/api/web/usage-collection" + (statusOnly ? "/status" : ""),
    { signal },
  );
