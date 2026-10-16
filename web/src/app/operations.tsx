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
