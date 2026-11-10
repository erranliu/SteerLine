import type { UsagePolicy } from "./usage-collection";
import { ApiError, json } from "./client";
export type Settings = Record<string, unknown>;
export interface Selection {
  adapter: string;
  target: { kind: string; environment?: string };
  agent: string;
  model?: string;
  reasoning_effort?: string;
  access_mode: string;
}
export interface ProfileEdit {
  profile_id: string;
  base_profile_id?: string;
  selection: Selection;
  unset: string[];
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
  continuation_suggestion?: { model: string; reasoning_effort: string } | null;
  developer?: { skip_environment_readiness?: boolean };
  version: number;
  runner: { executable: string; version: string; sha256: string };
  default_execution_profile: string;
  execution_profiles: Profile[];
  max_generate_concurrency?: number;
  usage_policy?: UsagePolicy;
}
export interface Combination {
  host: string;
  adapter: string;
  agent: string;
  supported: boolean;
  efforts: string[];
  workflows: ConfigSnapshot["workflows"];
}
export interface RuntimeEnvironment {
  adapter: string;
  target: Profile["execution"]["target"];
  profile_ids: string[];
}
export interface ConfigSnapshot {
  developer_available?: boolean;
  environments: RuntimeEnvironment[];
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
  profile_views: Record<
    string,
    { view?: Selection; editable: boolean; reason?: string }
  >;
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
    compose: (
      input: {
        saved_digest: string;
        base_profile_id?: string;
        workflow: Profile["workflow"];
        selection: Selection;
        unset: string[];
      },
      signal?: AbortSignal,
    ) =>
      json<{ execution: Profile["execution"]; view: Selection }>(
        path + "/compose",
        { method: "POST", headers, body: JSON.stringify(input), signal },
      ),
    validate: (config: ExecutionConfig, digest: string, edits: ProfileEdit[]) =>
      json<{ config: ExecutionConfig }>(path + "/validate", {
        method: "POST",
        headers,
        body: JSON.stringify({
          saved_digest: digest,
          config,
          profile_edits: edits,
        }),
      }),
    save: (
      config: ExecutionConfig,
      digest: string,
      edits: ProfileEdit[] = [],
    ) =>
      json<ConfigSnapshot>(path, {
        method: "PUT",
        headers: { ...headers, "If-Match": '"' + digest + '"' },
        body: JSON.stringify({
          saved_digest: digest,
          config,
          profile_edits: edits,
        }),
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

export async function saveExecutionConfig(
  api: ReturnType<typeof executionConfigAPI>,
  config: ExecutionConfig,
  digest: string,
  edits: ProfileEdit[] = [],
) {
  try {
    return await api.save(config, digest, edits);
  } catch (error) {
    if (error instanceof ApiError && error.status >= 400 && error.status < 500)
      throw error;
    const current = await api.read();
    if (stable(current.config) !== stable(config)) throw error;
    return current;
  }
}
