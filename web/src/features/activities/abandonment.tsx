import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import type { Envelope } from "../../shared/api/types";
import { invalidateCommand } from "../../shared/api/queries";
import { Button } from "../../shared/ui/button";
import { ended, type Activity } from "./model";
import { rememberAbandonedActivities } from "./abandonment-marks";

const stopNotes: Record<string, string> = {
  environment_detection:
    "尚未结算时会请求停止检测。停止结算仍可在原检测详情核实。",
  roadmap:
    "必要时会请求停止本记录的内容生成。发布、合并和入图继续沿原记录核实。",
  main_agent_advisory: "尚未结算时会取消建议生成。原候选和待答问题仍可处理。",
  refactoring:
    "活动仍在进行时会请求停止维护。原运行、发布和 PR 可在行动详情核实。",
  baseline_check: "原检查继续，可通过原请求核实结果。",
  task_candidate: "候选继续参与和运行，可在原候选详情查看或处理。",
};

export function AbandonActivity({
  activity,
  onApplied,
}: {
  activity: Activity;
  onApplied: () => void;
}) {
  const { api, identity, online, submit, requests } = useWorkspace();
  const client = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const [original, setOriginal] = useState<Envelope>();
  if (
    activity.provisional ||
    activity.abandoned_at ||
    !Object.hasOwn(stopNotes, activity.source) ||
    (!confirming && ended(activity) && !activity.needs_attention)
  )
    return null;
  const matchesTarget = (envelope: Envelope) =>
    envelope.action === "activity_abandon" &&
    envelope.target_id === activity.activity_id &&
    envelope.project_id === identity.project_id &&
    envelope.instance_id === identity.instance_id;
  const outstanding =
    original && matchesTarget(original)
      ? original
      : requests?.find(
          (row) =>
            matchesTarget(row.envelope) &&
            !["applied", "rejected"].includes(row.response?.status ?? ""),
        )?.envelope;

  async function abandon() {
    if (!online || sending) return;
    const envelope =
      outstanding ??
      api.envelope(
        "activity_abandon",
        activity.activity_id,
        activity.source_revision ?? 0,
        {},
      );
    setSending(true);
    setError("");
    try {
      const result = outstanding
        ? await api.reconcile(envelope)
        : await submit(envelope);
      if (result?.status === "applied") {
        rememberAbandonedActivities(
          client,
          [api.home, identity.project_id, identity.instance_id],
          [
            {
              ...activity,
              abandoned_at: result.abandoned_at ?? result.sampled_at,
            },
          ],
        );
        setOriginal(undefined);
        onApplied();
        if (outstanding) void invalidateCommand(client, api.home, envelope);
      } else if (result?.status === "rejected") {
        setOriginal(undefined);
        setError(result.diagnostic || "放弃活动被拒绝");
      } else {
        setOriginal(envelope);
        setError("放弃结果待核实，请查询原请求。");
      }
    } catch (reason) {
      setOriginal(
        (reason as { requestUnsent?: boolean })?.requestUnsent
          ? undefined
          : envelope,
      );
      setError(String(reason));
    } finally {
      setSending(false);
    }
  }

  return confirming ? (
    <div role="group" aria-label="确认放弃活动">
      <p>
        放弃「{activity.title}」并移入历史。{stopNotes[activity.source]}
      </p>
      {error && <p role="alert">{error}</p>}
      <Button
        size="sm"
        variant="ghost"
        disabled={sending}
        onClick={() => setConfirming(false)}
      >
        返回
      </Button>
      <Button
        size="sm"
        variant="outline"
        disabled={!online || sending}
        onClick={() => void abandon()}
      >
        {sending ? "正在处理…" : outstanding ? "核实放弃结果" : "确认放弃"}
      </Button>
    </div>
  ) : (
    <Button
      size="sm"
      variant="ghost"
      disabled={!online}
      onClick={() => setConfirming(true)}
    >
      {outstanding ? "放弃结果待核实" : "放弃活动"}
    </Button>
  );
}
