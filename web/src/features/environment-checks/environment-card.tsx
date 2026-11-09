import { useState } from "react";
import { Button } from "../../shared/ui/button";
import { environmentStage, type Environment } from "./use-readiness";

export type EnvironmentCommand = (
  action: string,
  target: string,
  version: number,
  payload: { [key: string]: unknown },
) => Promise<void>;

export function DetectionID({ id }: { id: string }) {
  const [feedback, setFeedback] = useState("");
  return (
    <div className="detection-id">
      <span>
        检测 ID：<code>{id}</code>
      </span>
      <Button
        size="sm"
        variant="ghost"
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(id);
            setFeedback("已复制");
          } catch {
            setFeedback("复制失败，请手动复制检测 ID");
          }
        }}
      >
        复制
      </Button>
      {feedback && <span role="status">{feedback}</span>}
    </div>
  );
}

export function EnvironmentCard({
  environment: e,
  project,
  defaultProfile,
  preferred,
  onProfileChange,
  confirmingStart,
  statusVerified,
  disabled,
  stopDisabled,
  activity,
  command,
}: {
  environment: Environment;
  project: string;
  defaultProfile: string;
  preferred?: string;
  onProfileChange: (profile: string) => void;
  confirmingStart: boolean;
  statusVerified: boolean;
  disabled: boolean;
  stopDisabled: boolean;
  activity?: {
    id: string;
    revision: number;
    settled: boolean;
    created_at?: string;
  };
  command: EnvironmentCommand;
}) {
  const profile =
    preferred === "" || e.profiles.some((p) => p.id === preferred)
      ? preferred
      : e.profiles.length === 1
        ? e.profiles[0].id
        : e.profiles.some((p) => p.id === defaultProfile)
          ? defaultProfile
          : "";
  const [timeout, setTimeout] = useState(3600);
  const [reason, setReason] = useState("");
  const chosen = e.profiles.find((p) => p.id === profile);
  const unsettled =
    !!e.activity_id && (!e.activity_settled || activity?.settled === false);
  const activityID = e.activity_id;
  return (
    <article
      className="environment-card"
      id={`environment-${e.environment_key || e.profiles[0]?.id}`}
      tabIndex={-1}
    >
      <h2 aria-live="polite">
        {confirmingStart
          ? "正在确认启动"
          : (environmentStage[statusVerified ? e.stage : "unverified"] ??
            environmentStage.unverified)}
      </h2>
      {!confirmingStart &&
        statusVerified &&
        e.stage !== "ready" &&
        e.reason && <p>{e.reason}</p>}
      {!confirmingStart && activity?.created_at && (
        <p className="muted">
          最近检测：{new Date(activity.created_at).toLocaleString("zh-CN")}
        </p>
      )}
      {!confirmingStart && activityID && (
        <DetectionID key={activityID} id={activityID} />
      )}
      {e.profiles.length > 1 && (
        <label>
          检测所用方案
          <select
            aria-label="检测所用方案"
            value={profile}
            onChange={(event) => onProfileChange(event.target.value)}
          >
            <option value="">选择本次检测方案</option>
            {e.profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {[p.name, p.model, p.reasoning_effort]
                  .filter(Boolean)
                  .join(" · ")}
              </option>
            ))}
          </select>
        </label>
      )}
      <div className="actions">
        {unsettled && !confirmingStart ? (
          <Button
            disabled={stopDisabled || !activity || activity.settled}
            onClick={() =>
              activity &&
              void command(
                "environment_stop",
                activity.id,
                activity.revision,
                {},
              )
            }
          >
            停止检测
          </Button>
        ) : (
          <Button
            disabled={
              disabled ||
              confirmingStart ||
              !statusVerified ||
              e.stage === "unverified" ||
              !e.environment_key ||
              !chosen ||
              !Number.isInteger(timeout) ||
              timeout < 1
            }
            onClick={() =>
              void command("environment_start", project, 1, {
                environment_key: e.environment_key,
                profile_id: profile,
                round_timeout_seconds: timeout,
                expected_current_revision: e.current_revision,
              })
            }
          >
            {e.stage === "ready" || activityID ? "重新检测" : "开始检测"}
          </Button>
        )}
      </div>
      {!confirmingStart && unsettled && (
        <p className="muted">
          {e.stage === "blocked"
            ? "本次检测尚未结束，可停止检测后重新发起。"
            : "本次检测结束前不能再次检测。"}
        </p>
      )}
      {!confirmingStart && e.stage === "ready" && (
        <p className="muted">重新检测会使当前就绪状态失效。</p>
      )}
      <details>
        <summary>更多设置</summary>
        <label>
          本次检测总超时（秒）
          <input
            aria-label="本次检测总超时（秒）"
            type="number"
            min={1}
            value={timeout}
            onChange={(event) => setTimeout(Number(event.target.value))}
          />
        </label>
        <label>
          环境失效原因
          <input
            aria-label="环境失效原因"
            value={reason}
            maxLength={4096}
            onChange={(event) => setReason(event.target.value)}
          />
        </label>
        <Button
          variant="ghost"
          disabled={
            disabled || confirmingStart || !e.environment_key || !reason.trim()
          }
          onClick={() =>
            void command("environment_invalidate", project, 1, {
              environment_key: e.environment_key,
              expected_current_revision: e.current_revision,
              reason,
            })
          }
        >
          标记失效
        </Button>
        <p className="muted">仅影响当前项目；不会停止正在进行的检测。</p>
      </details>
    </article>
  );
}
