import type { Candidate } from "../../shared/api/types";

export function nativeSessionReason(
  run: Candidate,
  online: boolean,
  caps: string[],
): string {
  if (!online) return "服务离线，连接后可查看原会话";
  const adapter = run.host === "orca" ? "orca-terminal" : "codex-app-server";
  if (
    !caps.includes("open_native_session") ||
    !caps.includes("open_native_session:" + adapter)
  ) {
    return `当前 Runner 未提供 ${run.host === "orca" ? "Orca" : "Codex"} 原会话查看能力`;
  }
  if (
    !run.execution_id ||
    !run.attempt_id ||
    !run.session_id ||
    !run.session_generation
  ) {
    return "原会话尚未就绪或绑定不完整";
  }
  return "";
}
