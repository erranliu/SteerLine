export interface Identity {
  protocol: string;
  project_id: string;
  repository_identity: string;
  instance_id: string;
  ready: boolean;
  status: string;
  capabilities: string[];
  diagnostic?: string;
  sampled_at: string;
}
export interface Profile {
  id: string;
  name: string;
  host: string;
  agent: string;
  unavailable_reason: string;
  details: string[];
}
export interface Presentation {
  default_profile_id: string;
  profiles: Profile[];
}
export interface Task {
  task_id: string;
  title: string;
  status: string;
  version: number;
  spec_path: string;
  dependency_ready: boolean;
  startup_blockers: string[];
  predecessors: {
    task_id: string;
    delivered: boolean;
    status: string;
    blocker: string;
  }[];
  successors: string[];
  workflow_runs: {
    workflow_run_id: string;
    task_id: string;
    candidate_number: number;
  }[];
  delivery?: { pr_url: string; workflow_run_id: string; merged_at: string };
  stop_pending?: string[];
}
export interface TaskPage {
  tasks: Task[];
  next_cursor?: string;
  has_more: boolean;
  graph_revision: number;
  database_revision: number;
  database_id: string;
  sampled_at: string;
}
export interface GraphPage {
  nodes: {
    task_id: string;
    title: string;
    status: string;
    dependency_ready: boolean;
    blockers: { task_id: string; blocker: string }[];
    startup_blockers: string[];
  }[];
  edges: { predecessor: string; successor: string }[];
  has_more: boolean;
  next_cursor?: string;
  graph_revision: number;
  database_revision: number;
  database_id: string;
  boundary: {
    truncated: boolean;
    omitted_node_count: number;
    reasons: string[];
  };
}
export interface Decision {
  id: string;
  attempt_id: string;
  version: number;
  status: string;
  request: {
    interaction_kind: string;
    blocks_progress: boolean;
    question: string;
    context: string;
    allow_free_text: boolean;
    has_native_questions: boolean;
    options: { ID: string; Label: string; Detail: string }[];
  };
}
export interface Candidate {
  task_status: string;
  delivery_outcome?: string;
  task_id: string;
  domain_task_id: string;
  run_id: string;
  title: string;
  execution_id: string;
  attempt_id: string;
  session_id: string;
  session_generation: string;
  current_turn_id: string;
  candidate_number: number;
  control_revision: number;
  recovery_version: number;
  recovery_required: boolean;
  execution_status: string;
  workflow_stage: string;
  workflow_status: string;
  stage_diagnostic?: string;
  agent: string;
  host: string;
  decision?: Decision;
  execution_profile?: Profile;
  pr: {
    url?: string;
    state: string;
    query_error?: string;
    merge_commit?: string;
  };
  worktree_path?: string;
  worktree_branch?: string;
  error?: string;
  summary?: string;
  environment_blocker?: {
    id: string;
    turn_id: string;
    version: number;
    fact: { Reason: string; Detail: string };
  };
}
export interface ServicePage {
  service: { instance_id: string; ready: boolean; capabilities: string[] };
  runs: Candidate[];
  control: {
    revision: number;
    dispatch_mode: string;
    dispatch_paused: boolean;
    blockers: { Reason: string }[];
  };
}
export interface Snapshot {
  connection: {
    state: string;
    can_control: boolean;
    identity_matched: boolean;
    config_matched: boolean;
    snapshot: { page?: ServicePage; sampled_at: string; error: string };
    diagnostic: string;
  };
  decision_reasons: Record<string, string>;
  native_task_error: string;
  history_error: string;
}
export interface Envelope {
  request_id: string;
  project_id: string;
  instance_id: string;
  action: string;
  target_id: string;
  expected_version: number;
  expected_control_revision?: number;
  expected_graph_revision?: number;
  payload: Record<string, unknown>;
}
export interface Receipt {
  request_id: string;
  project_id: string;
  instance_id: string;
  action: string;
  target_id: string;
  expected_version: number;
  original?: Envelope;
  status: "submitted" | "accepted" | "applied" | "rejected" | "outcome_unknown";
  code?: string;
  diagnostic?: string;
  sampled_at: string;
}
export interface StoredRequest {
  envelope: Envelope;
  created_at: string;
  response?: Receipt;
  error?: string;
}
export interface BodyRef {
  execution_id: string;
  attempt_id: string;
  item_key: string;
  revision: number;
  sha256: string;
  bytes: number;
  source: string;
  completeness: string;
}
export interface Conversation {
  events: {
    event_id?: number;
    type: string;
    body?: BodyRef;
    diagnostic?: string;
  }[];
  next_cursor?: string;
  has_more: boolean;
}
export interface FinalReply {
  workflow_id: string;
  execution_id: string;
  attempt_id: string;
  bodies: {
    item_key: string;
    revision: number;
    text: string;
    complete: boolean;
    completeness: string;
  }[];
  page: Conversation;
}
export interface Activity {
  page: {
    activity: {
      seq: number;
      ts: string;
      title: string;
      detail: string;
      kind: string;
      status: string;
    }[];
    diagnostics: string[];
  };
}

export interface LastMessageView {
  run_id: string;
  attempt_id: string;
  message: {
    run_id: string;
    attempt_id: string;
    session_id: string;
    turn_id: string;
    kind: "assistant" | "tool";
    text: string;
    observed_at: string;
  } | null;
}

export interface ApplicationIdentity {
  protocol: string;
  home_id: string;
  instance_id: string;
  ready: boolean;
}
export interface BrowserSession {
  csrf_token: string;
  identity: ApplicationIdentity;
  diagnostic?: string;
}
export interface ConfiguredProject {
  runtime_state?: "starting" | "ready" | "failed" | "stopping" | "stopped";
  instance_id?: string;
  config_id: string;
  name: string;
  repository: string;
  project_id?: string;
  online: boolean;
  conflict?: string;
  diagnostic?: string;
}
