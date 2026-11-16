import { useWorkspace } from "../../app/context";
import {
  CopyExecutionInfo,
  type ExecutionReference,
} from "../activities/execution-info";
import { environmentStage } from "./use-readiness";

export type DetectionRecord = {
  execution_ref?: ExecutionReference;
  id: string;
  revision: number;
  profile_id: string;
  environment_key: string;
  created_at?: string;
  stage: string;
  status: string;
  settled: boolean;
  closing_action?: string;
  diagnostic: string;
  requests?: { request_id: string; action: string; activity_id?: string }[];
  rounds?: {
    number: number;
    submission: string;
    run_id?: string;
    submission_diagnostic?: string;
  }[];
};
export const recordLabel = (record: DetectionRecord) => {
  if (!record.settled) {
    return isDetectionClosing(record)
      ? environmentStage.closing
      : environmentStage.verifying;
  }
  const stage = record.stage;
  return stage === "ready"
    ? "检测通过"
    : environmentStage[stage] || environmentStage.unverified;
};

export const isDetectionClosing = (record?: DetectionRecord) =>
  record?.stage === "closing" && !record.settled;

export function DetectionSubmissionHistory({
  record,
}: {
  record: DetectionRecord;
}) {
  return record.rounds?.map((round) => {
    const original = round.submission_diagnostic;
    if (!original || original === record.diagnostic) return null;
    const outcome =
      round.submission === "accepted" || round.run_id
        ? "已受理"
        : round.submission === "rejected"
          ? "已拒绝"
          : round.submission === "cancelled"
            ? "已取消"
            : undefined;
    if (!outcome) return null;
    const history = original.startsWith("request_outcome_unknown:")
      ? `首次提交时未能确认回执，随后确认${outcome}。`
      : `首次提交记录：${original}（随后确认${outcome}）`;
    return (
      <p className="muted" key={round.number}>
        {history}
      </p>
    );
  });
}

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
