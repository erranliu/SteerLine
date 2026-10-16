// @vitest-environment node
import { expect, it } from "vitest";
import type { Candidate } from "../../shared/api/types";
import { nativeSessionReason } from "./native-session";

const run = {
  host: "local",
  execution_id: "run",
  attempt_id: "attempt",
  session_id: "session",
  session_generation: "generation",
} as Candidate;
const caps = ["open_native_session", "open_native_session:codex-app-server"];

it("enables the original bound Codex session only when Runner advertises it", () => {
  expect(nativeSessionReason(run, true, caps)).toBe("");
  expect(nativeSessionReason(run, true, [])).toContain("未提供 Codex");
  expect(nativeSessionReason(run, true, ["open_native_session"])).toContain(
    "未提供 Codex",
  );
  expect(nativeSessionReason({ ...run, host: "orca" }, true, caps)).toContain(
    "未提供 Orca",
  );
});

it("distinguishes offline service from missing native identities", () => {
  expect(nativeSessionReason(run, false, caps)).toContain("服务离线");
  for (const field of [
    "execution_id",
    "attempt_id",
    "session_id",
    "session_generation",
  ]) {
    expect(nativeSessionReason({ ...run, [field]: "" }, true, caps)).toContain(
      "绑定不完整",
    );
  }
});
