import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";

export function freshDualPackage() {
  const root = resolve(process.env.STEERLINE_WEB_PACKAGE ?? "");
  if (!root.toLowerCase().startsWith("c:\\steerlinedebug\\"))
    throw new Error(
      "fresh generated package under C:\\SteerlineDebug required",
    );
  const manifest = JSON.parse(
    readFileSync(join(root, "manifest.json"), "utf8"),
  );
  if (
    manifest.status !== "ready" ||
    manifest.web_projects !== 2 ||
    !manifest.web_binary_sha256 ||
    existsSync(join(root, "scheme.json"))
  )
    throw new Error("fresh dual-project Web package required");
  return { root, manifest };
}

export function packageCommand(env: NodeJS.ProcessEnv) {
  return (exe: string, args: string[]) =>
    execFileSync(exe, args, {
      env,
      encoding: "utf8",
      timeout: 45000,
      windowsHide: true,
      stdio: "pipe",
    }).trim();
}
