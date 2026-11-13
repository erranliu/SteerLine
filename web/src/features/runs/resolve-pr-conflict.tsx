import { useState } from "react";
import { useWorkspace } from "../../app/context";
import type { Candidate } from "../../shared/api/types";
import { Button } from "../../shared/ui/button";

export function ResolvePRConflict({ run }: { run: Candidate }) {
  const { api, online, busy, snapshot, requests, send } = useWorkspace();
  const [sending, setSending] = useState(false);
  const [submittedRevision, setSubmittedRevision] = useState<number>();
  const page = snapshot?.connection.snapshot.page;
  const delivery = run.runner_delivery;
  if (
    !run.pr.url ||
    run.participation !== "active" ||
    run.stop_requested ||
    ["closed", "discarded", "completed"].includes(run.task_status || "") ||
    ["merged", "closed", "closed_unmerged"].includes(run.pr.state || "")
  )
    return null;
  const pending = requests?.some(
    (row) =>
      row.envelope.project_id === api.identity.project_id &&
      row.envelope.instance_id === api.identity.instance_id &&
      row.envelope.action === "resolve_pr_conflict" &&
      row.envelope.target_id === run.run_id &&
      !["applied", "rejected"].includes(row.response?.status || ""),
  );
  const activeMessage = delivery?.commands.some(
    (command) =>
      command.command === "message" &&
      !["completed", "failed", "rejected", "cancelled"].includes(
        command.action_status || command.receipt_state,
      ),
  );
  const reason = !online
    ? "服务离线"
    : pending
      ? "原请求待核实，请前往「项目状态」查询"
      : busy ||
          sending ||
          activeMessage ||
          submittedRevision === run.control_revision
        ? "正在处理上一项操作"
        : !page?.service.capabilities.includes("resolve_pr_conflict") ||
            !delivery?.controls.can_message
          ? "当前执行暂不能接收新指令"
          : "";
  return (
    <Button
      disabled={!!reason}
      title={reason}
      onClick={async () => {
        if (reason || sending) return;
        setSending(true);
        try {
          if (
            await send(
              "resolve_pr_conflict",
              run.run_id,
              run.control_revision,
              { task_id: run.domain_task_id || run.task_id },
              { expected_control_revision: page!.control.revision },
            )
          )
            setSubmittedRevision(run.control_revision);
        } finally {
          setSending(false);
        }
      }}
    >
      解决合并冲突
    </Button>
  );
}
