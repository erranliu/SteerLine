import { useWorkspace } from "../../app/context";
import {
  CopyExecutionInfo,
  type ExecutionReference,
} from "../activities/execution-info";

export type DetectionRecord = {
  execution_ref?: ExecutionReference;
  id: string;
  revision: number;
  profile_id: string;
  environment_key: string;
  created_at?: string;
  state: string;
  status: string;
  settled: boolean;
  blocked?: boolean;
  timed_out?: boolean;
  diagnostic: string;
};
const recordState: { [key: string]: string } = {
  ready: "已通过",
  failed: "未通过",
  cancelled: "已停止",
  running: "检测中",
  queued: "等待检测",
  stopping: "正在停止",
  unverified: "结果待核实",
};
export const recordLabel = (record: DetectionRecord) =>
  record.timed_out
    ? "检测超时"
    : record.blocked
      ? "检测阻断"
      : recordState[record.state] || "结果待核实";

export function DetectionID({
  id,
  execution,
}: {
  id: string;
  execution?: ExecutionReference;
}) {
  const { identity } = useWorkspace();
  return (
    <div className="detection-id">
      <CopyExecutionInfo
        key={id}
        info={{
          title: "环境检测",
          project: identity.repository_identity,
          requestID: execution?.request_id || id,
          record: { id, label: "检测记录" },
          runner: execution || {},
          runnerUnverified: !execution,
        }}
      />
    </div>
  );
}
