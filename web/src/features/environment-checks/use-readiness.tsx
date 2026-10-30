import { useQuery } from "@tanstack/react-query";
import { NavLink } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { usePageVisible } from "../../shared/query-visibility";

export type EnvironmentProfile = {
  id: string;
  name: string;
  environment_key: string;
  status: string;
  stage: string;
  reason: string;
  current_revision: number;
  activity_id?: string;
};
export type Environment = {
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
  profiles: EnvironmentProfile[];
  environments: Environment[];
  generators: { id: string; name: string }[];
  default_profile_id: string;
};
export const environmentStage: Record<string, string> = {
  unprepared: "未准备",
  awaiting_user: "待用户准备/验收",
  verifying: "验收中",
  ready: "已就绪",
  blocked: "阻断",
  stale: "已失效",
};

export function useEnvironmentReadiness() {
  const { api, identity, online } = useWorkspace();
  const visible = usePageVisible();
  return useQuery({
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
}
export function EnvironmentSummary() {
  const { identity } = useWorkspace();
  const readiness = useEnvironmentReadiness();
  const environments = readiness.data?.environments;
  return (
    <NavLink to={`/projects/${identity.project_id}/environment`}>
      {environments
        ? `环境已准备 ${environments.filter((e) => e.status === "ready").length}/${environments.length}`
        : "环境准备状态待核实"}
    </NavLink>
  );
}
