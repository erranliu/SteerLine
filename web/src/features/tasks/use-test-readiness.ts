import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { projectQueries } from "../../shared/api/queries";
import { usePageVisible } from "../../shared/query-visibility";

export const testPreparationNames: Record<string, string> = {
  unprepared: "材料与测试安排待准备",
  preparing: "正在读取和校验材料",
  ready: "测试安排已就绪",
  pending: "材料与测试安排待准备",
  queued: "测试安排排队中",
  accepted: "测试安排生成中",
  generated: "正在校验测试安排",
  unknown: "原生成结果待核实",
  failed: "测试安排生成失败",
  invalid: "材料或测试安排无效",
  rejected: "测试安排生成被拒绝",
  cancelled: "测试生成已取消",
  configuration_required: "需要配置测试生成",
};

export function useTestReadiness(id: string) {
  const { api, online } = useWorkspace();
  const visible = usePageVisible();
  const query = useQuery({
    ...projectQueries(api).taskReadiness(id),
    enabled: online && !!id,
    refetchInterval: online && visible ? 2000 : false,
  });
  const verified = online && query.isSuccess;
  const ready = verified && query.data?.status === "ready";
  const text = !online
    ? "测试安排状态待核实（服务未连接）"
    : query.isError
      ? "测试安排查询失败，等待重新核实"
      : !query.data
        ? "正在读取测试安排状态…"
        : testPreparationNames[query.data.status] || "测试安排状态待核实";
  return {
    ready,
    text,
    reason: ready ? "" : text,
    diagnostic: query.isError
      ? String(query.error)
      : verified
        ? query.data?.diagnostic
        : undefined,
  };
}
