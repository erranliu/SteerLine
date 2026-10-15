import { queryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "./client";
import type {
  Activity,
  Candidate,
  Envelope,
  GraphPage,
  Presentation,
  Receipt,
  Snapshot,
  TaskPage,
} from "./types";

export function projectQueries(api: Api) {
  const scope = [
    api.home,
    api.identity.project_id,
    api.identity.instance_id,
  ] as const;
  const defaults = { staleTime: 1000, retry: false } as const;
  return {
    snapshot: () =>
      queryOptions({
        ...defaults,
        queryKey: [...scope, "snapshot"],
        queryFn: ({ signal }) => api.get<Snapshot>("/snapshot", signal),
      }),
    presentation: () =>
      queryOptions({
        ...defaults,
        staleTime: 60000,
        queryKey: [...scope, "presentation"],
        // Shared bootstrap reads survive page observer changes. Api.get still
        // aborts them when their Workspace/project lifetime ends.
        queryFn: () => api.get<{ presentation: Presentation }>("/presentation"),
      }),
    tasks: (cursor = "", limit = 100) =>
      queryOptions({
        ...defaults,
        queryKey: [...scope, "tasks", { cursor, limit }],
        queryFn: () =>
          api.get<TaskPage>(
            "/tasks?" + new URLSearchParams({ limit: String(limit), cursor }),
          ),
      }),
    task: (id: string) =>
      queryOptions({
        ...defaults,
        queryKey: [...scope, "task", id],
        queryFn: ({ signal }) =>
          api.get<TaskPage>("/tasks/" + encodeURIComponent(id), signal),
      }),
    graph: (cursor = "", limit = 100) =>
      queryOptions({
        ...defaults,
        queryKey: [...scope, "graph", { cursor, limit }],
        queryFn: ({ signal }) =>
          api.get<GraphPage>(
            "/graph?" + new URLSearchParams({ limit: String(limit), cursor }),
            signal,
          ),
      }),
    activity: (
      run: Pick<Candidate, "run_id" | "execution_id" | "attempt_id">,
    ) =>
      queryOptions({
        ...defaults,
        queryKey: [
          ...scope,
          "activity",
          run.run_id,
          run.execution_id,
          run.attempt_id,
        ],
        queryFn: ({ signal }) =>
          api.get<Activity>(
            "/runs/" +
              encodeURIComponent(run.run_id) +
              "/activity?" +
              new URLSearchParams({
                execution_id: run.execution_id,
                attempt_id: run.attempt_id,
                recent: "true",
              }),
            signal,
          ),
      }),
    final: (run: Pick<Candidate, "run_id" | "execution_id" | "attempt_id">) =>
      queryOptions({
        ...defaults,
        queryKey: [
          ...scope,
          "final",
          run.run_id,
          run.execution_id,
          run.attempt_id,
        ],
        queryFn: ({ signal }) => api.final(run, signal),
      }),
  };
}

const taskResources = new Set(["snapshot", "tasks", "task", "graph"]);
const dynamicResources = new Set([...taskResources, "activity", "final"]);
const taskActions = new Set([
  "set_dispatch_mode",
  "pause_dispatch",
  "resume_dispatch",
  "start_task",
  "start_candidate",
]);

export function invalidateCommand(
  client: QueryClient,
  home: string,
  envelope: Envelope,
) {
  const { project_id, instance_id, action } = envelope;
  const affectsOutput =
    action === "answer_decision" || action === "stop_candidate";
  const runID =
    action === "stop_candidate" ? envelope.target_id : envelope.payload.run_id;
  return client.invalidateQueries({
    predicate: ({ queryKey: key }) => {
      if (key[0] !== home) return false;
      if (key[1] === "local-requests") return true;
      if (
        key[1] !== project_id ||
        key[2] !== instance_id ||
        action === "open_native_session"
      )
        return false;
      const resource = String(key[3]);
      if (taskActions.has(action)) return taskResources.has(resource);
      if (affectsOutput && typeof runID === "string" && runID) {
        return (
          taskResources.has(resource) ||
          ((resource === "activity" || resource === "final") &&
            key[4] === runID)
        );
      }
      return dynamicResources.has(resource);
    },
  });
}

// Sample timestamps change on every read; only a changed outcome needs a refresh.
export function receiptChanged(previous: Receipt | undefined, result: Receipt) {
  return (
    !previous ||
    previous.status !== result.status ||
    previous.code !== result.code ||
    previous.diagnostic !== result.diagnostic
  );
}

export function refreshProject(client: QueryClient, api: Api) {
  return Promise.all([
    client.invalidateQueries({
      queryKey: [api.home, api.identity.project_id, api.identity.instance_id],
    }),
    client.invalidateQueries({ queryKey: [api.home, "local-requests"] }),
  ]);
}
