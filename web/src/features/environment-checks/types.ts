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
    version: number;
    supported: {
      adapter: string;
      target_kind: string;
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
    capability_status: string;
    actual_context: Record<string, string>;
    required_checks: string[];
    items: {
      id: string;
      status: string;
      reason?: string;
      identity?: string;
      evidence: { source: string; text: string }[];
    }[];
    resources: { status: string; diagnostic?: string };
    controls: { can_stop: boolean; reason?: string };
    pr?: { url?: string };
  };
}
