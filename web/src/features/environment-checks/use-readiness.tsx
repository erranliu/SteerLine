import { useQuery } from "@tanstack/react-query";
import { NavLink } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { usePageVisible } from "../../shared/query-visibility";

export type EnvironmentProfile = {
  id: string;
  name: string;
  model?: string;
  reasoning_effort?: string;
  environment_key: string;
  status: string;
  stage: string;
  reason: string;
  current_revision: number;
  activity_id?: string;
};
export type Environment = {
  target: { kind: string; environment?: string };
  adapter: string;
  agent: string;
  activity_settled: boolean;
  script_path?: string;
  environment_key: string;
  name: string;
  profiles: EnvironmentProfile[];
  status: string;
  stage: string;
  reason: string;
  current_revision: number;
  activity_id?: string;
  source_commit?: string;
  document_path: string;
  document?: string;
  last_passed_activity_id?: string;
};
export type EnvironmentOptions = {
  environment_readiness_required?: boolean;
  profiles: EnvironmentProfile[];
  environments: Environment[];
  default_profile_id: string;
};
export const environmentStage: Record<string, string> = {
  unprepared: "未准备",
  awaiting_user: "待准备与检测",
  verifying: "检测中",
  timed_out: "检测超时",
  ready: "已就绪",
  blocked: "检测阻断",
  stale: "已失效",
};

export function useEnvironmentReadiness() {
  const { api, identity, online } = useWorkspace();
  const visible = usePageVisible();
  const configuration = useQuery({
    queryKey: [
      api.home,
      identity.project_id,
      identity.instance_id,
      "environment",
      "configuration",
    ],
    queryFn: ({ signal }) =>
      api.get<EnvironmentOptions>(
        "/environment/options?configuration=true",
        signal,
      ),
    enabled: online,
    retry: false,
    // Loaded configuration is frozen for this project instance.
    staleTime: Infinity,
  });
  const query = useQuery({
    queryKey: [
      api.home,
      identity.project_id,
      identity.instance_id,
      "environment",
      "options",
    ],
    queryFn: ({ signal }) =>
      api.get<EnvironmentOptions>("/environment/options", signal),
    enabled: online,
    retry: false,
    refetchInterval: visible ? 15000 : false,
  });
  const verified = online && query.isSuccess && !query.isError;
  const configurationVerified =
    online && configuration.isSuccess && !configuration.isError;
  const required =
    (configurationVerified
      ? configuration.data?.environment_readiness_required
      : undefined) ??
    (verified ? query.data?.environment_readiness_required : undefined) ??
    true;
  return {
    ...query,
    data: query.data ?? configuration.data,
    observed: !!query.data,
    verified,
    required,
    refetch: (...args: Parameters<typeof query.refetch>) => {
      void configuration.refetch();
      return query.refetch(...args);
    },
  };
}

export function environmentLink(
  project: string,
  profile?: string,
  task?: string,
) {
  const params = new URLSearchParams();
  if (profile) params.set("profile", profile);
  if (task) params.set("task", task);
  return `/projects/${project}/environment${params.size ? "?" + params : ""}`;
}

export function EnvironmentObservation() {
  const readiness = useEnvironmentReadiness();
  if (readiness.verified) return null;
  return (
    <p role="status" className="notice amber">
      {readiness.observed
        ? "上次已知状态，当前待核实"
        : "项目运行环境状态待核实"}
      {!!readiness.dataUpdatedAt &&
        ` · 最后成功观察 ${new Date(readiness.dataUpdatedAt).toLocaleString()}`}
    </p>
  );
}

export function EnvironmentSummary() {
  const { identity } = useWorkspace();
  const readiness = useEnvironmentReadiness();
  const environments = readiness.data?.environments;
  return (
    <NavLink to={`/projects/${identity.project_id}/environment`}>
      {!readiness.observed ? (
        "项目运行环境状态待核实"
      ) : (
        <>
          {!readiness.verified && "上次结果 · 当前待核实 · "}
          {environments?.length
            ? `项目运行环境已就绪 ${environments.filter((e) => e.status === "ready").length}/${environments.length}`
            : "尚无开发执行方案"}
          {!readiness.required && " · 环境就绪阻塞已关闭"}
        </>
      )}
    </NavLink>
  );
}
