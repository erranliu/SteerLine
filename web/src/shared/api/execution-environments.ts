import { useQuery } from "@tanstack/react-query";
import {
  executionConfigAPI,
  type Catalog,
  type Profile,
} from "./execution-configs";

// The remote machine directory belongs to the application home, not a project.
export function useExecutionEnvironments(
  home: string,
  csrf: string,
  enabled = true,
) {
  return useQuery({
    queryKey: [home, "execution-environments"],
    queryFn: ({ signal }) =>
      executionConfigAPI(csrf).discover(
        { kind: "environments", host: "orca_environment" },
        signal,
      ),
    enabled,
    retry: false,
  });
}

export function executionTargetName(
  target: Profile["execution"]["target"],
  environments?: Catalog["environments"],
) {
  if (target.kind === "local") return "本机";
  return (
    environments?.find(
      (e) => e.id === target.environment || e.name === target.environment,
    )?.name ??
    target.environment ??
    "远端机器"
  );
}
