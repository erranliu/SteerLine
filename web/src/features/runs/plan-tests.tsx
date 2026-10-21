import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { projectQueries } from "../../shared/api/queries";
import type { Candidate } from "../../shared/api/types";

export type TestResult = {
  test_id: string;
  status: string;
  message: string;
  log: string;
};
export type TestRow = {
  id: string;
  name: string;
  level: string;
  environment: string;
  command: string;
  check: string;
  status: string;
  message?: string;
  log?: string;
  history: TestResult[];
};
export type PlanTestsView = {
  commit?: string;
  status: string;
  diagnostic?: string;
  plan_path: string;
  total: number;
  passed: number;
  failed: number;
  pending: number;
  rows: TestRow[];
};
const labels: Record<string, string> = {
  passed: "通过",
  failed: "失败",
  incomplete: "未完成",
  pending: "待验证",
  stale: "待复验",
};
export function PlanTests({ run }: { run: Candidate }) {
  const { api, online } = useWorkspace();
  const result = useQuery({
    ...projectQueries(api).planTests(run),
    enabled: online,
    refetchInterval: online ? 5000 : false,
  });
  const view = result.data;
  return (
    <section className="settings-panel" aria-label="计划测试">
      <h3>计划测试</h3>
      {!online && (
        <p className="notice amber">离线 · 以下为最后观察，当前结果待核实</p>
      )}
      {result.error && (
        <p className="notice amber">测试查询失败：{String(result.error)}</p>
      )}
      {!view && !result.error && (
        <p className="muted">{online ? "正在读取测试安排…" : "尚无测试观察"}</p>
      )}
      {view?.status === "missing" && (
        <p className="muted">Plan 未关联测试安排</p>
      )}
      {view?.status === "invalid" && (
        <p className="notice amber">测试安排无效</p>
      )}
      {view?.diagnostic && <p className="muted">{view.diagnostic}</p>}
      {view?.status === "ready" && (
        <>
          <p className="muted">
            {view.plan_path}
            {view.commit && (
              <>
                {" "}
                · 固定版本 <code>{view.commit.slice(0, 12)}</code>
              </>
            )}
          </p>
          {view.total === 0 ? (
            <p>未安排自动测试</p>
          ) : (
            <p>
              共 {view.total} 项 ·{" "}
              {online && !result.error ? "通过" : "上次通过"} {view.passed} ·
              失败 {view.failed} · 待验证 {view.pending}
            </p>
          )}
          {view.rows.map((row) => (
            <details key={row.id} className="test-plan-row">
              <summary>
                {row.name} · {row.level} / {row.environment} ·{" "}
                {labels[row.status] || "待核实"}
              </summary>
              <p className="muted">{row.id}</p>
              <pre>{row.command}</pre>
              <p>{row.check}</p>
              {row.message && <p>{row.message}</p>}
              {row.log && (
                <p className="muted">
                  证据引用：<code>{row.log}</code>
                </p>
              )}
              {row.history.length > 0 && (
                <details>
                  <summary>历史结果（{row.history.length}）</summary>
                  {row.history.map((item, i) => (
                    <p key={i}>
                      {labels[item.status] || item.status} · {item.message}{" "}
                      <code>{item.log}</code>
                    </p>
                  ))}
                </details>
              )}
            </details>
          ))}
        </>
      )}
    </section>
  );
}
