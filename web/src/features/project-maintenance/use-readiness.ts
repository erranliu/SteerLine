import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { projectQueries } from "../../shared/api/queries";
import { usePageVisible } from "../../shared/query-visibility";

export function useProjectReadiness() {
  const { api, online } = useWorkspace();
  const visible = usePageVisible();
  const query = useQuery({
    ...projectQueries(api).projectReadiness(),
    enabled: online,
    refetchInterval: online && visible ? 5000 : false,
  });
  const verified = online && query.isSuccess;
  const ready = verified && query.data?.status === "ready";
  const text = !online
    ? "项目未就绪（服务未连接）"
    : query.isError
      ? "项目未就绪（状态查询失败）"
      : !query.data
        ? "项目未就绪（正在核实）"
        : ready
          ? "项目已就绪"
          : "项目未就绪";
  return {
    ready,
    text,
    reason: ready ? "" : text,
    diagnostic: query.isError
      ? String(query.error)
      : verified
        ? query.data?.blockers.join("；")
        : undefined,
  };
}
