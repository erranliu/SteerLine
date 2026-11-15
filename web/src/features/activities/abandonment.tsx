import { useState } from "react";
import {
  useQueries,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import type { Envelope } from "../../shared/api/types";
import { invalidateCommand } from "../../shared/api/queries";
import { Button } from "../../shared/ui/button";
import { ended, type Activity } from "./model";

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

export function rememberAbandonedActivities(
  client: QueryClient,
  scope: string[],
  activities: Pick<Activity, "source" | "source_id" | "abandoned_at">[],
) {
  const marks = Object.fromEntries(
    activities.flatMap((a) =>
      a.abandoned_at ? [[a.source + "/" + a.source_id, a.abandoned_at]] : [],
    ),
  );
  if (Object.keys(marks).length) {
    client.setQueryData<Record<string, string>>(
      [...scope, "abandoned-activity-marks"],
      (old) => ({ ...old, ...marks }),
    );
  }
}

export function useAbandonedActivities(source = "", ids: string[] = []) {
  const { api, identity, online } = useWorkspace();
  const client = useQueryClient();
  const scope = [api.home, identity.project_id, identity.instance_id];
  const localKey = [...scope, "abandoned-activity-marks"];
  const unique = [...new Set(ids.filter(Boolean))].sort();
  const batches = Array.from(
    { length: Math.ceil(unique.length / 100) },
    (_, i) => unique.slice(i * 100, i * 100 + 100),
  );
  const [local, ...remote] = useQueries({
    queries: [
      {
        queryKey: localKey,
        enabled: false,
        queryFn: async () => ({}) as Record<string, string>,
      },
      ...batches.map((batch) => ({
        queryKey: [...scope, "abandoned-activities", source, batch],
        queryFn: async ({ signal }: { signal: AbortSignal }) => {
          const params = new URLSearchParams({
            mode: "abandoned",
            source,
            source_ids: batch.join(","),
          });
          const marks = await api.get<Record<string, string>>(
            "/activities?" + params,
            signal,
          );
          if (marks && !Array.isArray(marks) && typeof marks === "object") {
            const found = Object.fromEntries(
              batch.flatMap((id) =>
                typeof marks[id] === "string"
                  ? [[source + "/" + id, marks[id]]]
                  : [],
              ),
            );
            if (Object.keys(found).length)
              client.setQueryData<Record<string, string>>(localKey, (old) => ({
                ...old,
                ...found,
              }));
          }
          return marks;
        },
        enabled: online,
        retry: false,
        staleTime: 5000,
      })),
    ],
  });
  return {
    isAbandoned: (id?: string) => !!id && !!local.data?.[source + "/" + id],
    mark: (name: string, id: string) => local.data?.[name + "/" + id],
    pending: online && remote.some((query) => query.isPending),
  };
}

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
  const outstanding =
    original ??
    requests?.find(
      (row) =>
        row.envelope.action === "activity_abandon" &&
        row.envelope.target_id === activity.activity_id &&
        row.envelope.project_id === identity.project_id &&
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
