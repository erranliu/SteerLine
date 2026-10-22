import { json } from "./client";
export interface Settings {
  executable?: string;
  model?: string;
  reasoning_effort?: string;
  access_mode?: string;
  agent?: string;
  transport?: { executable?: string; [key: string]: unknown };
  agent_launch?: {
    executable?: string;
    model?: string;
    reasoning_effort?: string;
    [key: string]: unknown;
  };
  [key: string]: unknown;
}
export interface Profile {
  id: string;
  name: string;
  execution: {
    adapter: string;
    target: { kind: string; environment?: string };
    settings: Settings;
  };
  workflow: {
    id: string;
    version: number;
    audit_model?: { model: string; reasoning_effort?: string };
    [key: string]: unknown;
  };
}
export interface ExecutionConfig {
  version: number;
  runner: { executable: string; version: string; sha256: string };
  default_execution_profile: string;
  execution_profiles: Profile[];
  max_generate_concurrency?: number;
  pricing_policy?: string;
  usage_policy?: Record<string, unknown>;
}
export interface Combination {
  host: string;
  adapter: string;
  agent: string;
  supported: boolean;
  efforts: string[];
  workflows: ConfigSnapshot["workflows"];
}
export interface ConfigSnapshot {
  config: ExecutionConfig;
  saved_digest: string;
  writable: boolean;
  reason?: string;
  workflows: {
    id: string;
    version: number;
    supported_adapters: string[];
    audit_model_required: boolean;
    prompt_steps?: string[];
  }[];
  efforts: string[];
  combinations: Combination[];
  local_helper: { available: boolean; reason?: string };
}
export interface Catalog {
  state:
    "not_requested" | "in_progress" | "ready" | "unavailable" | "unsupported";
  freshness: "fresh" | "cached" | "expired";
  origin: string;
  fetched_at?: string;
  context_key: string;
  context_unverified: boolean;
  scope?: string;
  diagnostic?: string;
  environments?: { id: string; name: string }[];
  agents?: {
    id: string;
    directory: string;
  }[];
  models?: {
    id: string;
    label: string;
    provider?: string;
    is_default: boolean;
    default_effort?: string;
    efforts: { value: string; label: string }[];
  }[];
}
const path = "/api/web/execution-config";
export function executionConfigAPI(csrf: string) {
  const headers = { "Content-Type": "application/json", "X-CSRF-Token": csrf };
  return {
    read: (signal?: AbortSignal) => json<ConfigSnapshot>(path, { signal }),
    discover: (
      query: {
        host: string;
        environment?: string;
        profile_id?: string;
        agent?: string;
        refresh?: boolean;
      } & (
        | { kind: "environments"; adapter?: never }
        | { kind: "agents" | "models"; adapter: string }
      ),
      signal?: AbortSignal,
    ) =>
      json<Catalog>(path + "/discover", {
        method: "POST",
        headers,
        body: JSON.stringify(query),
        signal,
      }),
    validate: (config: ExecutionConfig) =>
      json<{ config: ExecutionConfig }>(path + "/validate", {
        method: "POST",
        headers,
        body: JSON.stringify(config),
      }),
    save: (config: ExecutionConfig, digest: string) =>
      json<ConfigSnapshot>(path, {
        method: "PUT",
        headers: { ...headers, "If-Match": '"' + digest + '"' },
        body: JSON.stringify(config),
      }),
  };
}
export function stable(value: unknown): string {
  if (Array.isArray(value)) return "[" + value.map(stable).join(",") + "]";
  if (value && typeof value === "object")
    return (
      "{" +
      Object.keys(value)
        .sort()
        .map(
          (key) =>
            JSON.stringify(key) +
            ":" +
            stable((value as Record<string, unknown>)[key]),
        )
        .join(",") +
      "}"
    );
  return JSON.stringify(value);
}
