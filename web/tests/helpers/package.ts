import { randomUUID } from "node:crypto";
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
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

const psQuote = (value: string) => "'" + value.replaceAll("'", "''") + "'";
const block = (ms: number) =>
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);

// Node's Windows Job owns its child tree. WMI starts this verification
// command in an independent process context. The generated
// script contains only fixture paths, never credentials or production settings.
function outsideJob(
  exe: string,
  args: string[],
  env: NodeJS.ProcessEnv,
  wait: boolean,
) {
  const home = env.STEERLINE_HOME!;
  const root = resolve(home, "..");
  const id = randomUUID();
  const script = join(root, "evidence", "native-" + id + ".ps1");
  const result = join(root, "evidence", "native-" + id + ".json");
  const output = join(root, "evidence", "native-" + id + ".log");
  const lines = [
    "$ErrorActionPreference='Stop'",
    "[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false);$OutputEncoding=[Console]::OutputEncoding",
    "$PSDefaultParameterValues=@{'Out-File:Encoding'='utf8'}",
    "$env:STEERLINE_HOME=" + psQuote(home),
    "$env:STEERLINE_E2E_CONTROLLED_STATE=" +
      psQuote(env.STEERLINE_E2E_CONTROLLED_STATE!),
    "$env:CODEX_HOME=" + psQuote(join(home, "codex")),
    "$env:GH_CONFIG_DIR=" + psQuote(join(home, "gh")),
    "$env:TEMP=" + psQuote(join(home, "temp")),
    "$env:TMP=" + psQuote(join(home, "temp")),
    "$env:GIT_CONFIG_NOSYSTEM='1'",
    "$env:GIT_CONFIG_GLOBAL='NUL'",
    "$env:GIT_TERMINAL_PROMPT='0'",
    "$taskExit=1",
    "try { $ErrorActionPreference='Continue'; & " +
      psQuote(exe) +
      " " +
      args.map(psQuote).join(" ") +
      " *> " +
      psQuote(output) +
      "; $taskExit=$LASTEXITCODE } finally { @{exit_code=$taskExit}|ConvertTo-Json -Compress|Set-Content -LiteralPath " +
      psQuote(result) +
      " -Encoding utf8 }",
  ];
  writeFileSync(script, "\ufeff" + lines.join("\r\n"));
  const shellArgs =
    'powershell.exe -NoProfile -ExecutionPolicy Bypass -File "' + script + '"';
  // WMI creates the owned verification launcher outside Node's kill-on-close
  // child Job. ShowWindow=0 keeps the independent launcher hidden.
  execFileSync(
    "powershell.exe",
    [
      "-NoProfile",
      "-Command",
      "$startup=New-CimInstance -ClassName Win32_ProcessStartup -ClientOnly -Property @{ShowWindow=[uint16]0};" +
        "$created=Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{CommandLine=" +
        psQuote(shellArgs) +
        ";CurrentDirectory=" +
        psQuote(root) +
        ";ProcessStartupInformation=$startup};" +
        "if($created.ReturnValue -ne 0){throw ('native launcher creation failed: '+$created.ReturnValue)}",
    ],
    { windowsHide: true, timeout: 20000, stdio: "pipe" },
  );
  if (!wait) return result;
  const deadline = Date.now() + 195000;
  while (!existsSync(result) && Date.now() < deadline) block(100);
  if (!existsSync(result))
    throw new Error("native coordinator outcome unverified: " + output);
  const fact = JSON.parse(readFileSync(result, "utf8").replace(/^\uFEFF/, ""));
  const text = existsSync(output)
    ? readFileSync(output, "utf8").replace(/^\uFEFF/, "")
    : "";
  if (fact.exit_code !== 0)
    throw new Error("native coordinator failed: " + text);
  return text.trim();
}
export function packageCommand(env: NodeJS.ProcessEnv) {
  return (exe: string, args: string[]) => {
    if (
      (exe.toLowerCase().endsWith("steerline.exe") && args[0] === "start") ||
      (args[0] === "debug" && args[1] === "stop")
    )
      return outsideJob(exe, args, env, true);
    if (exe.toLowerCase() === "powershell.exe") {
      const at = args.indexOf("-Command");
      if (at >= 0) {
        args = [...args];
        args[at + 1] =
          "[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false);" +
          args[at + 1];
      }
    }
    return execFileSync(exe, args, {
      env,
      encoding: "utf8",
      timeout: 140000,
      windowsHide: true,
      stdio: "pipe",
    }).trim();
  };
}

const delay = (ms: number) => new Promise((done) => setTimeout(done, ms));

// Sole owner of fresh validation, scheme freezing, debug helpers and cleanup.
export function preparePackageRuntime() {
  const { root, manifest } = freshDualPackage();
  const home = join(root, "home"),
    binary = join(root, "bin", "steerline.exe"),
    web = join(root, "bin", "steerline-web.exe"),
    helper = join(root, "bin", "debug-helper.exe");
  const env = {
    ...process.env,
    STEERLINE_HOME: home,
    STEERLINE_E2E_CONTROLLED_STATE: join(root, "provider", "state.json"),
  };
  const command = packageCommand(env);
  const outcomes: string[] = [];
  command(helper, ["debug", "prepare", "--simulate"]);
  const scheme = JSON.parse(readFileSync(join(root, "scheme.json"), "utf8"));
  if (scheme.scheme !== "simulate" || !scheme.ready)
    throw new Error("scheme freeze did not complete");
  const launch = (exe: string, args: string[]) => {
    const outcome = outsideJob(exe, args, env, false);
    outcomes.push(outcome);
    return outcome;
  };
  const start = async () => {
    launch(helper, ["debug", "run", "--simulate", "--no-open"]);
    const deadline = Date.now() + 180000;
    let diagnostic = "";
    while (Date.now() < deadline) {
      if (outcomes[0] && existsSync(outcomes[0]))
        throw new Error(
          "debug owner exited before readiness; inspect native launch logs under " +
            root,
        );
      try {
        const status = JSON.parse(command(binary, ["status", "--json"]));
        diagnostic = JSON.stringify(status);
        const ready = join(root, "runtime-launcher", "ready.json");
        if (
          status.backend.state === "ready" &&
          status.web.state === "ready" &&
          status.projects.length === 2 &&
          status.projects.every((p: any) => p.runtime_state === "ready") &&
          existsSync(ready)
        ) {
          const ownerRecord = JSON.parse(
            readFileSync(join(root, "runtime-launcher", "owner.json"), "utf8"),
          );
          if (
            JSON.parse(readFileSync(ready, "utf8")).token === ownerRecord.token
          )
            return status;
        }
      } catch (error) {
        diagnostic = String(error);
      }
      await delay(100);
    }
    throw new Error("required projects did not become ready: " + diagnostic);
  };
  const stop = async () => {
    const errors: string[] = [];
    try {
      command(helper, ["debug", "stop", "--simulate"]);
    } catch (error) {
      errors.push(String(error));
    }
    const deadline = Date.now() + 120000;
    while (Date.now() < deadline && outcomes.some((path) => !existsSync(path)))
      await delay(100);
    for (const path of outcomes)
      if (!existsSync(path))
        errors.push("owned launcher did not exit: " + path);
    return errors;
  };
  return {
    root,
    manifest,
    home,
    binary,
    web,
    helper,
    env,
    command,
    launch,
    start,
    stop,
  };
}
