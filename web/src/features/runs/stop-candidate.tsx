import { useState } from "react";
import { Square } from "lucide-react";
import { useWorkspace } from "../../app/context";
import type { Candidate, Envelope, Receipt } from "../../shared/api/types";
import { requestByID } from "../../shared/api/journal";
import { Button } from "../../shared/ui/button";
import { stopReason } from "./state";

export function StopCandidate({
  run,
  pending,
  onRequest,
  onResult,
}: {
  run: Candidate;
  pending: boolean;
  onRequest: (envelope: Envelope) => void;
  onResult: (envelope: Envelope, receipt?: Receipt, unsent?: boolean) => void;
}) {
  const { api, online, busy, snapshot, submit } = useWorkspace();
  const [confirmation, setConfirmation] = useState<{
    run: Candidate;
    envelope: Envelope;
  }>();
  const [sending, setSending] = useState(false);
  const page = snapshot?.connection.snapshot.page;
  const reason = !online
    ? "服务离线，连接后可停止参与"
    : !page?.service.capabilities.includes("stop_candidate")
      ? "当前服务未提供停止参与能力"
      : pending
        ? "停止请求待核实，请在操作回执中查询原请求"
        : busy || sending
          ? "正在处理上一项操作"
          : stopReason(run, page.runs);
  const changed =
    confirmation &&
    (confirmation.envelope.project_id !== api.identity.project_id ||
      confirmation.envelope.instance_id !== api.identity.instance_id ||
      confirmation.envelope.target_id !== run.run_id ||
      confirmation.envelope.expected_version !== run.control_revision ||
      confirmation.envelope.expected_control_revision !==
        page?.control.revision);
  const confirmReason =
    reason || (changed ? "候选或服务状态已变化，请取消后重新确认" : "");
  return (
    <div>
      <Button
        variant="destructive"
        disabled={!!reason}
        title={reason}
        onClick={() =>
          setConfirmation({
            run: { ...run },
            envelope: api.envelope(
              "stop_candidate",
              run.run_id,
              run.control_revision,
              { task_id: run.domain_task_id || run.task_id },
              { expected_control_revision: page!.control.revision },
            ),
          })
        }
      >
        <Square size={13} />
        停止参与
      </Button>
      {reason && (
        <p className="muted" role="status">
          {reason}
        </p>
      )}
      {confirmation && (
        <div
          className="notice amber"
          role="dialog"
          aria-label="确认停止候选参与"
        >
          <strong>停止候选 C{confirmation.run.candidate_number}</strong>
          <p>
            项目 {confirmation.envelope.project_id} · 任务{" "}
            {String(confirmation.envelope.payload.task_id)} ·{" "}
            {confirmation.run.execution_profile?.name || confirmation.run.agent}
          </p>
          <p>
            候选 {confirmation.envelope.target_id} · 原执行{" "}
            {confirmation.run.execution_id || "尚未绑定"} · 服务{" "}
            {confirmation.envelope.instance_id}
          </p>
          <p>
            取消本候选的后续业务推进与交付资格；保留历史、worker、会话、PR
            和工作区，原 worker 可能继续已有工作。
          </p>
          {confirmReason && <p role="status">{confirmReason}</p>}
          <div className="actions">
            <Button
              disabled={sending}
              onClick={() => setConfirmation(undefined)}
            >
              取消
            </Button>
            <Button
              variant="destructive"
              disabled={!!confirmReason}
              onClick={async () => {
                if (confirmReason || sending) return;
                const envelope = confirmation.envelope;
                setSending(true);
                onRequest(envelope);
                try {
                  const receipt = await submit(envelope);
                  let unsent = false;
                  if (!receipt) {
                    try {
                      unsent = !(await requestByID(envelope.request_id));
                    } catch {
                      /* preserve unknown */
                    }
                  }
                  onResult(envelope, receipt, unsent);
                  if (receipt?.status !== "rejected" && !unsent)
                    setConfirmation(undefined);
                } finally {
                  setSending(false);
                }
              }}
            >
              确认停止
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
