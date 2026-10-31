import { useState } from "react";
import { Button } from "../../shared/ui/button";
import { environmentStage, type Environment } from "./use-readiness";

export type EnvironmentCommand = (
  action: string,
  target: string,
  version: number,
  payload: { [key: string]: unknown },
) => Promise<void>;

const nextStep: Record<string, string> = {
  unprepared: "查看准备要求，补齐或更新项目材料。",
  awaiting_user: "在实际执行账号下准备工具、变量和权限，然后开始验收。",
  verifying: "查看当前进度；原执行结算前不能再次验收。",
  ready: "可以用于本项目的新任务，仍需满足任务的其他启动条件。",
  blocked: "查看未通过或未核实项，处理具体原因后重新验收。",
  stale: "查看失效原因，处理环境或材料变化后重新验收。",
};

export function EnvironmentCard({
  environment: e,
  project,
  defaultProfile,
  preferred,
  disabled,
  command,
  onActivity,
}: {
  environment: Environment;
  project: string;
  defaultProfile: string;
  preferred: string;
  disabled: boolean;
  command: EnvironmentCommand;
  onActivity: (id: string) => void;
}) {
  const initial = e.profiles.some((p) => p.id === preferred)
    ? preferred
    : e.profiles.length === 1
      ? e.profiles[0].id
      : e.profiles.some((p) => p.id === defaultProfile)
        ? defaultProfile
        : "";
  const [profile, setProfile] = useState(initial);
  const [timeout, setTimeout] = useState(3600);
  const [reason, setReason] = useState("");
  const chosen = e.profiles.find((p) => p.id === profile);
  const unsettled = !!e.activity_id && !e.activity_settled;
  const title =
    e.adapter === "codex-app-server"
      ? "Codex 直连"
      : e.adapter === "orca-terminal"
        ? `Orca · ${e.agent || "Codex"}`
        : e.adapter;
  return (
    <article
      className="environment-card"
      id={`environment-${e.environment_key || e.profiles[0]?.id}`}
      tabIndex={-1}
    >
      <h3>
        {title} · {environmentStage[e.stage] ?? "状态待核实"}
      </h3>
      <p>
        共用方案：
        {e.profiles
          .map((p) => p.name + (p.id === defaultProfile ? "（默认）" : ""))
          .join("、")}
      </p>
      <p className="muted">
        环境 {e.environment_key?.slice(0, 10) || "不支持"} ·
        资格仅适用于当前项目
      </p>
      <p>{e.reason}</p>
      <p>下一步：{nextStep[e.stage] || "刷新并核实当前环境状态。"}</p>
      <div className="actions">
        <a href="#environment-materials">查看准备要求</a>
        {e.activity_id && (
          <Button variant="ghost" onClick={() => onActivity(e.activity_id!)}>
            查看当前验收
          </Button>
        )}
        {e.last_passed_activity_id && (
          <Button
            variant="ghost"
            onClick={() => onActivity(e.last_passed_activity_id!)}
          >
            查看历史通过报告
          </Button>
        )}
      </div>
      {unsettled && (
        <p role="status">原活动尚未结算，请查看进度或停止原活动。</p>
      )}
      <details open={preferred && preferred === initial ? true : undefined}>
        <summary>
          {e.stage === "ready" || e.activity_id ? "重新验收" : "开始验收"}
        </summary>
        <p>
          验收会调用所选模型、运行完整 L2 并创建真实验证
          PR；每次最多三轮。通过后供上述方案共用。重新开始即切断旧资格，失败不会恢复历史资格。
        </p>
        <label>
          验收执行方案
          <select
            aria-label="验收执行方案"
            value={profile}
            onChange={(event) => setProfile(event.target.value)}
          >
            <option value="">选择本次验收方案</option>
            {e.profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          每轮总超时（秒）
          <input
            aria-label="每轮总超时（秒）"
            type="number"
            min={1}
            value={timeout}
            onChange={(event) => setTimeout(Number(event.target.value))}
          />
        </label>
        <Button
          disabled={
            disabled ||
            unsettled ||
            !e.environment_key ||
            !chosen ||
            !Number.isInteger(timeout) ||
            timeout < 1
          }
          onClick={() =>
            void command("environment_start", project, 1, {
              profile_id: profile,
              round_timeout_seconds: timeout,
              expected_current_revision: e.current_revision,
            })
          }
        >
          提交验收
        </Button>
      </details>
      <details>
        <summary>标记失效</summary>
        <p>仅影响当前项目的这个执行环境，含上述共用方案；不会停止原执行。</p>
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
          disabled={disabled || !e.environment_key || !reason.trim()}
          onClick={() =>
            void command("environment_invalidate", project, 1, {
              environment_key: e.environment_key,
              expected_current_revision: e.current_revision,
              reason,
            })
          }
        >
          标记当前项目环境失效
        </Button>
      </details>
    </article>
  );
}
