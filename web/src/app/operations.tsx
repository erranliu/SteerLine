import type { StoredRequest } from "../shared/api/types";
import { Button } from "../shared/ui/button";
import { Status, short } from "../shared/ui/status";
import { Link } from "react-router-dom";
import {
  CopyExecutionInfo,
  requestExecutionInfo,
} from "../features/activities/execution-info";
import { requestDestination } from "../features/activities/model";
export function Operations({
  requests,
  reconcile,
  reconciling,
  online,
  compact = false,
}: {
  requests: StoredRequest[];
  reconcile: (row: StoredRequest) => void;
  reconciling: boolean;
  online: boolean;
  compact?: boolean;
}) {
  if (compact) {
    const rows = requests
      .filter((r) => r.envelope.action.startsWith("environment_"))
      .sort((a, b) => a.created_at.localeCompare(b.created_at));
    const latest = rows.at(-1);
    const pending = rows.filter(
      (r) => !["applied", "rejected"].includes(r.response?.status ?? ""),
    );
    const displayed = rows.filter((r) => r === latest || pending.includes(r));
    const actions: Record<string, string> = {
      environment_start: "开始检测",
      environment_stop: "停止检测",
      environment_invalidate: "标记失效",
    };
    return displayed.length ? (
      <section
        className="environment-operation-results"
        aria-label="操作结果"
        aria-live="polite"
      >
        {displayed.map((row) => {
          const unresolved = pending.includes(row);
          const confirmingStart =
            unresolved && row.envelope.action === "environment_start";
          return (
            <div key={row.envelope.request_id}>
              <p>
                {actions[row.envelope.action] || "环境操作"}：
                {row.response?.status === "applied"
                  ? "操作成功"
                  : row.response?.status === "rejected"
                    ? "操作未成功，请刷新后重试"
                    : confirmingStart
                      ? "正在确认启动"
                      : "结果待核实"}
                {unresolved && (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={!online || reconciling}
                    onClick={() => reconcile(row)}
                  >
                    重新查询
                  </Button>
                )}
              </p>
              {!confirmingStart &&
                row.response?.status !== "applied" &&
                (row.response?.diagnostic || row.error) && (
                  <p className="muted">
                    {row.response?.diagnostic || row.error}
                  </p>
                )}
              <CopyExecutionInfo
                info={requestExecutionInfo(row.envelope, row.response)}
              />
              {requestDestination(row.envelope, row.response) && (
                <Link to={requestDestination(row.envelope, row.response)!}>
                  查看对应记录
                </Link>
              )}
            </div>
          );
        })}
      </section>
    ) : null;
  }
  return requests.length ? (
    <details className="operations">
      <summary>
        操作回执{" "}
        <span>
          {
            requests.filter(
              (r) =>
                !r.response ||
                ["submitted", "accepted", "outcome_unknown"].includes(
                  r.response.status,
                ),
            ).length
          }{" "}
          项待核实
        </span>
      </summary>
      {requests
        .slice()
        .reverse()
        .slice(0, 12)
        .map((row) => (
          <div className="operation" key={row.envelope.request_id}>
            <div>
              <strong>{row.envelope.action}</strong>
              <span>
                目标 {short(row.envelope.target_id)} · 请求{" "}
                {short(row.envelope.request_id)}
              </span>
              {row.error && <p className="muted">{row.error}</p>}
              {!["applied", "rejected"].includes(row.response?.status ?? "") &&
                Date.now() - Date.parse(row.created_at) >= 30000 && (
                  <p className="notice amber">
                    结果尚未确认 · 已等待{" "}
                    {Math.floor(
                      (Date.now() - Date.parse(row.created_at)) / 1000,
                    )}{" "}
                    秒；保留原请求，继续核实。
                  </p>
                )}
              {row.checked_at && (
                <p className="muted">
                  最后核实：{new Date(row.checked_at).toLocaleString("zh-CN")}
                </p>
              )}
              <CopyExecutionInfo
                info={requestExecutionInfo(row.envelope, row.response)}
              />
              {requestDestination(row.envelope, row.response) && (
                <Link to={requestDestination(row.envelope, row.response)!}>
                  查看对应记录
                </Link>
              )}
              {row.response?.diagnostic && (
                <p className="muted">{row.response.diagnostic}</p>
              )}
            </div>
            <Status value={row.response?.status ?? "outcome_unknown"} />
            {(!row.response ||
              !["applied", "rejected"].includes(row.response.status)) && (
              <Button
                size="sm"
                disabled={!online || reconciling}
                onClick={() => reconcile(row)}
              >
                查询原请求
              </Button>
            )}
          </div>
        ))}
    </details>
  ) : null;
}
