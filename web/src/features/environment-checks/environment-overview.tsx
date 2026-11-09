import { useEffect, useState } from "react";
import { Button } from "../../shared/ui/button";
import { environmentStage, type Environment } from "./use-readiness";

const priority = (stage: string) =>
  stage === "ready" ? 2 : stage === "verifying" ? 1 : 0;

export function EnvironmentOverview({
  environments,
  verified,
  selected,
  name,
  confirming,
  onSelect,
}: {
  environments: Environment[];
  verified: boolean;
  selected: string;
  name: (environment: Environment) => string;
  confirming: (environment: Environment) => boolean;
  onSelect: (key: string) => void;
}) {
  const [order, setOrder] = useState<string[] | null>(null);
  useEffect(() => {
    if (order === null && verified && environments.length)
      setOrder(
        [...environments]
          .sort((a, b) => priority(a.stage) - priority(b.stage))
          .map((e) => e.environment_key),
      );
  }, [environments, verified, order]);
  // Freeze the first verified ordering. Polling must not move a focused row.
  const rows = [...environments].sort((a, b) => {
    if (!order) return verified ? priority(a.stage) - priority(b.stage) : 0;
    const index = (e: Environment) => {
      const found = order.indexOf(e.environment_key);
      return found < 0 ? order.length : found;
    };
    return index(a) - index(b);
  });
  const ready = environments.filter(
    (e) => e.status === "ready" && e.stage === "ready" && !confirming(e),
  ).length;
  const running = environments.filter(
    (e) => e.stage === "verifying" || confirming(e),
  ).length;
  const attention = environments.length - ready - running;
  return (
    <section aria-label="全部运行环境" className="environment-overview">
      <div className="environment-totals" role="status">
        {verified ? (
          <>
            <span className="environment-total-ready">
              <strong>
                {ready}/{environments.length}
              </strong>{" "}
              已就绪
            </span>
            <span>检测中 {running}</span>
            <span>需要处理 {attention}</span>
          </>
        ) : (
          <span>共 {environments.length} 个环境 · 当前状态待核实</span>
        )}
      </div>
      <table className="environment-table">
        <caption className="sr-only">所有环境的当前就绪情况</caption>
        <thead>
          <tr>
            <th scope="col">运行机 / 执行链路</th>
            <th scope="col">当前状态</th>
            <th scope="col">状态说明</th>
            <th scope="col">操作</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((e) => {
            const isConfirming = confirming(e);
            const stage = verified ? e.stage : "unverified";
            const label = isConfirming
              ? "正在确认启动"
              : environmentStage[stage] || environmentStage.unverified;
            const active = e.environment_key === selected;
            return (
              <tr
                key={e.environment_key || e.profiles[0]?.id}
                className={active ? "environment-row-selected" : undefined}
              >
                <td>
                  <strong>{name(e)}</strong>
                  <p className="environment-profiles">
                    关联方案：{e.profiles.map((p) => p.name).join("、") || "无"}
                  </p>
                </td>
                <td>
                  <span
                    className="environment-status"
                    data-stage={isConfirming ? "verifying" : stage}
                  >
                    {label}
                  </span>
                </td>
                <td>
                  <p className="environment-overview-reason">
                    {isConfirming
                      ? "正在核实原请求，请勿重复提交。"
                      : !verified
                        ? `上次状态：${environmentStage[e.stage] || "待核实"}`
                        : e.stage === "ready"
                          ? "当前准备要求已通过"
                          : e.reason || "查看详情了解当前检测情况"}
                  </p>
                  {verified &&
                    !isConfirming &&
                    e.activity_id &&
                    !e.activity_settled && (
                      <p className="muted">本次检测尚未结束</p>
                    )}
                </td>
                <td>
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-expanded={active}
                    aria-controls={active ? "environment-detail" : undefined}
                    disabled={!e.environment_key}
                    onClick={() => onSelect(active ? "" : e.environment_key)}
                  >
                    {active ? "收起详情" : "查看详情"}
                  </Button>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </section>
  );
}
