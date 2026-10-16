export interface CheckExecution {
  adapter: string;
  target: { kind: string; environment?: string };
  settings: Record<string, unknown>;
}
export interface CheckCatalog {
  repository: string;
  profiles: { id: string; name: string; execution: CheckExecution }[];
  unavailable_reason: string;
  capabilities?: {
    contract: string;
    targets: {
      adapter: string;
      target: string;
      mechanism: string;
      platform?: string;
      reason?: string;
      access_modes: string[];
    }[] | null;
  };
}
export interface CheckRecord {
  id: string;
  check_id: string;
  created_at: string;
  diagnostic?: string;
  runner: { executable: string; version: string; sha256: string };
  resource_digest: string;
  input: {
    repository: { url: string; base_branch: string; base_revision: string };
    execution: CheckExecution;
    publication?: { repository: string };
  };
  operations: {
    request_id: string;
    action: string;
    status: string;
    diagnostic?: string;
    runner_request: { input: { expected_revision?: number } };
  }[];
  report?: {
    check_id: string;
    revision: number;
    input_digest: string;
    updated_at: string;
    status: string;
    result: string;
    cleanup_status: string;
    diagnostic?: string;
    allowed_actions: string[];
    actual: Record<string, string | number>;
    items: {
      id: string;
      status: string;
      stage: string;
      required: boolean;
      evidence: { source: string; facts?: Record<string, string> }[];
    }[];
    resources: { pr_url?: string; writer_stopped?: boolean; local_removed?: boolean };
  };
}
