import { useState } from "react";
import type { StoredRequest } from "../shared/api/types";
import { Button } from "../shared/ui/button";
import { Status, short } from "../shared/ui/status";
export function Operations({
  requests,
  reconcile,
  reconciling,
  online,
}: {
  requests: StoredRequest[];
  reconcile: (row: StoredRequest) => void;
  reconciling: boolean;
  online: boolean;
}) {
  const [copyFeedback, setCopyFeedback] = useState("");
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
      {copyFeedback && <p role="status">{copyFeedback}</p>}
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
              <button
                className="muted"
                onClick={async () => {
                  try {
                    await navigator.clipboard.writeText(
                      row.envelope.request_id,
                    );
                    setCopyFeedback("已复制原请求 ID");
                  } catch {
                    setCopyFeedback(
                      "复制失败，请手动复制：" + row.envelope.request_id,
                    );
                  }
                }}
              >
                复制原请求 ID
              </button>
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
