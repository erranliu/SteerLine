import type { ReactNode } from "react";
import { render } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { WorkspaceContext, type Workspace } from "../app/context";
import { Api } from "../shared/api/client";
import type {
  Candidate,
  GraphPage,
  Identity,
  Snapshot,
  TaskPage,
} from "../shared/api/types";

export const identity = {
  project_id: "project",
  instance_id: "instance",
  repository_identity: "/repo",
  ready: true,
} as Identity;
export const run = {
  run_id: "run",
  execution_id: "execution",
  attempt_id: "attempt",
  task_id: "1",
  domain_task_id: "1",
  task_status: "in_progress",
  workflow_status: "running",
  workflow_stage: "implement.execute",
  candidate_number: 1,
  title: "候选任务",
  agent: "codex",
  pr: { state: "none" },
} as Candidate;
export const tasks: TaskPage = {
  tasks: [
    {
      task_id: "1",
      title: "缓存任务",
      status: "open",
      version: 1,
      spec_path: "spec.md",
      dependency_ready: true,
      startup_blockers: [],
      predecessors: [],
      successors: [],
      workflow_runs: [],
    },
  ],
  graph_revision: 1,
  database_revision: 1,
  database_id: "db",
  sampled_at: "2026-10-15T00:00:00Z",
  has_more: false,
};
export const graph: GraphPage = {
  nodes: [
    {
      task_id: "1",
      title: "缓存任务",
      status: "open",
      dependency_ready: true,
      blockers: [],
      startup_blockers: [],
    },
  ],
  edges: [],
  has_more: false,
  graph_revision: 1,
  database_revision: 1,
  database_id: "db",
  boundary: { truncated: false, omitted_node_count: 0, reasons: [] },
};
export function snapshot(api: Api, runs: Candidate[] = []): Snapshot {
  return {
    connection: {
      diagnostic: "",
      state: "connected",
      can_control: true,
      identity_matched: true,
      config_matched: true,
      snapshot: {
        sampled_at: "2026-10-15T00:00:00Z",
        error: "",
        page: {
          service: {
            instance_id: api.identity.instance_id,
            ready: true,
            capabilities: ["pause_dispatch", "set_dispatch_mode", "start_task"],
          },
          runs,
          control: {
            revision: 1,
            dispatch_mode: "manual",
            dispatch_paused: false,
            blockers: [],
          },
        },
      },
    },
    decision_reasons: {},
    native_task_error: "",
    history_error: "",
  };
}
export function testClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity },
      mutations: { retry: false },
    },
  });
}
export function mountFeature(
  ui: ReactNode,
  api: Api,
  client: QueryClient,
  online = true,
  overrides: Partial<Workspace> = {},
) {
  return render(
    <QueryClientProvider client={client}>
      <WorkspaceContext.Provider
        value={{
          api,
          identity: api.identity,
          online,
          snapshot: snapshot(api, [run]),
          busy: false,
          send: async () => true,
          refresh: () => {},
          ...overrides,
        }}
      >
        <MemoryRouter>{ui}</MemoryRouter>
      </WorkspaceContext.Provider>
    </QueryClientProvider>,
  );
}
