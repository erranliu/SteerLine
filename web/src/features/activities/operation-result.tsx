import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import type { Receipt } from "../../shared/api/types";
import { Status } from "../../shared/ui/status";
import {
  CopyExecutionInfo,
  operationNames,
  requestExecutionInfo,
} from "./execution-info";

// Historical receipts describe the exact request, independently of current workflow state.
export function OperationResult({
  requestID,
  targetID,
  sourceID,
}: {
  requestID: string;
  targetID: string;
  sourceID?: string;
}) {
  const { api, identity, online } = useWorkspace();
  const result = useQuery({
    queryKey: [
      api.home,
      identity.project_id,
      identity.instance_id,
      "operation-result",
      requestID,
    ],
    queryFn: () =>
      api.get<Receipt>("/commands/" + encodeURIComponent(requestID)),
    enabled: online && !!requestID,
    retry: false,
    refetchInterval: (query) =>
      online &&
      !["applied", "rejected"].includes(query.state.data?.status || "")
        ? 5000
        : false,
  });
  if (!requestID) return null;
  const r = result.data;
  const related = (r?.roadmap || r?.environment) as { id?: string } | undefined;
  const matches =
    r?.request_id === requestID &&
    r.project_id === identity.project_id &&
    (r.target_id === targetID ||
      related?.id === targetID ||
      (["roadmap_parse", "roadmap_task_generate"].includes(r.action) &&
        r.target_id === sourceID) ||
      (r.action === "roadmap_prepare_static" &&
        r.target_id === identity.project_id));
  return (
    <section aria-label="指定操作结果" className="notice">
      <h3>
        本次操作 ·{" "}
        {matches ? operationNames[r.action] || r.action : "读取原请求"}
      </h3>
      {matches ? (
        <>
          <Status value={r.status} />
          {r.diagnostic && <p className="notice amber">{r.diagnostic}</p>}
          <CopyExecutionInfo
            key={requestID}
            info={{
              ...requestExecutionInfo(r.original || { ...r, payload: {} }, r),
              project: identity.repository_identity,
            }}
          />
        </>
      ) : (
        <>
          <p>
            {result.isError || r
              ? "无法核实该请求与当前记录的关联"
              : "正在读取原请求…"}
          </p>
          <CopyExecutionInfo
            info={{
              title: "原操作待核实",
              project: identity.repository_identity,
              requestID,
            }}
          />
        </>
      )}
    </section>
  );
}
