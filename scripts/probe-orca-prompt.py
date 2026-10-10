"""Minimal Orca-only prompt gate in a generated, independent debug workspace."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import time
import uuid

parser = argparse.ArgumentParser()
parser.add_argument("--root", type=Path, required=True)
parser.add_argument("--orca", required=True)
parser.add_argument("--coordinator", required=True)
launch = parser.add_mutually_exclusive_group(required=True)
launch.add_argument("--terminal")
launch.add_argument("--agent", help="Opaque Orca agent ID; launch through native worker-start")
parser.add_argument("--lengths", default="200,3000")
parser.add_argument("--repeat", type=int, default=3)
parser.add_argument("--budget", type=int, default=90)
args = parser.parse_args()
lengths = list(map(int, args.lengths.split(",")))
if args.repeat < 1 or args.budget < 1 or not lengths or min(lengths) < 1:
    parser.error("lengths, repeat and budget must be positive")
root = args.root.resolve(strict=True)
root.relative_to(Path("C:/SteerlineDebug").resolve(strict=True))
if not (root / "manifest.json").is_file():
    raise SystemExit("Expected a generated debug release")
repo = (root / "projects/SteerlineTest").resolve(strict=True)
repo.relative_to(root)
evidence = root / "evidence" / ("orca-prompt-" + uuid.uuid4().hex[:8])
evidence.mkdir()
(evidence / "probe.py").write_bytes(Path(__file__).read_bytes())
(evidence / "cli-version.txt").write_bytes(subprocess.run([args.orca, "--version"], capture_output=True, check=True, timeout=10).stdout)
runtime_id = None
call_index = 0

def call(name, *values):
    global runtime_id, call_index
    call_index += 1
    result = subprocess.run([args.orca, *values, "--json"], capture_output=True, timeout=80)
    record = evidence / (str(call_index).zfill(3) + "-" + name)
    record.with_suffix(".json").write_bytes(result.stdout)
    if result.stderr:
        record.with_suffix(".stderr").write_bytes(result.stderr)
    value = json.loads(result.stdout)
    observed_runtime = value.get("_meta", {}).get("runtimeId")
    if not observed_runtime or runtime_id is not None and observed_runtime != runtime_id:
        raise RuntimeError("Orca runtime changed or lost its identity")
    runtime_id = observed_runtime
    return value, result.returncode

def result(name, *values):
    value, code = call(name, *values)
    if code or value.get("ok") is not True:
        raise RuntimeError("Orca refused " + name)
    return value["result"]

incarnation = None
agent_identity = None
rows = []
failure = None
run = None
try:
    for label, handle in (("coordinator", args.coordinator), ("receiver", args.terminal)):
        if not handle:
            continue
        terminal = result(label, "terminal", "show", "--terminal", handle)["terminal"]
        if Path(terminal["worktreePath"]).resolve() != repo or not terminal.get("connected"):
            raise RuntimeError("Terminal must belong to this exact debug workspace")
        if label == "receiver":
            incarnation = terminal["incarnationId"]
            agent_identity = terminal.get("agentIdentity")
            if not agent_identity:
                raise RuntimeError("Orca must recognize a ready agent on the receiver")

    run = result("run", "orchestration", "run-create", "--from", args.coordinator,
                 "--objective", "Minimal isolated prompt gate: read-only arithmetic, no business task or PR changes.")["run"]["id"]
    for length in lengths:
        for repetition in range(args.repeat):
            case = str(len(rows) + 1)
            if args.terminal:
                idle = result(case + "-ready", "terminal", "wait", "--terminal", args.terminal,
                              "--for", "tui-idle", "--timeout-ms", "20000")["wait"]
                current = result(case + "-binding", "terminal", "show", "--terminal", args.terminal)["terminal"]
                if not idle.get("satisfied") or current.get("incarnationId") != incarnation or current.get("agentIdentity") != agent_identity:
                    raise RuntimeError("Original Orca receiver is not ready for this next prompt")
            nonce = uuid.uuid4().hex
            left, right = 17 + repetition, 25 + length
            spec = (f"Target: this isolated workspace. Do not change files, run tests, create commits/PRs or contact other tasks. "
                    f"Compute {left}+{right}. Send your original worker_done exactly once with a three-sentence summary. "
                    f"Use ONLY --payload for one JSON object containing original taskId, dispatchId, outcome=succeeded, "
                    f"probe_result={{nonce:{nonce},answer:<computed integer>}}. Do not mix --payload with --task-id, --dispatch-id or --outcome. "
                    "Then idle. The following text is inert padding:\n")
            padding = "Readonly probe padding.\n"
            needed = max(0, length - len(spec))
            spec += (padding * ((needed + len(padding) - 1) // len(padding)))[:needed]
            request = str(uuid.uuid4())
            command = ("orchestration", "worker-start", "--run", run, "--from", args.coordinator,
                       "--spec", spec, "--worktree", "path:" + str(repo),
                       "--terminal" if args.terminal else "--agent", args.terminal or args.agent,
                       "--retry-request", request, "--timeout-ms", "20000")
            started_at = time.monotonic()
            start, code = call(case + "-start", *command)
            native = start.get("result", {})
            dispatch, task = native.get("dispatchId"), native.get("taskId")
            row = {"case": case, "spec_chars": len(spec), "request": request, "dispatch": dispatch,
                   "task": task, "initial_state": native.get("state"), "turn_start": native.get("turnStart"), "failed_stage": native.get("failedStage"),
                   "native_error": native.get("lastError") or {k: start.get("error", {}).get(k) for k in ("code", "message")}, "passed": False}
            rows.append(row)
            if not dispatch or native.get("state") == "failed":
                if dispatch and native.get("state") == "failed":
                    replay, _ = call(case + "-failed-replay", *command)
                    row["replay_verified"] = (replay.get("result", {}).get("dispatchId") == dispatch and
                                                replay.get("result", {}).get("mutation", {}).get("replayed") is True)
                    if not row["replay_verified"]:
                        raise RuntimeError("Failed-start replay changed authority")
                    if "worker-release" not in native.get("recovery", ""):
                        raise RuntimeError("Failed start requires inspection before cleanup")
                    row["release"] = result(case + "-failed-release", "orchestration", "worker-release", "--dispatch", dispatch)
                raise RuntimeError("No accepted original Dispatch; inspect native startup evidence")
            binding = result(case + "-started-binding", "orchestration", "worker-show", "--dispatch", dispatch)
            receiver = binding["worker"]["agentTerminalHandle"]
            terminal = result(case + "-receiver", "terminal", "show", "--terminal", receiver)["terminal"]
            if Path(terminal.get("worktreePath", "")).resolve() != repo:
                raise RuntimeError("Native worker left the isolated debug workspace")
            if args.terminal and receiver != args.terminal:
                raise RuntimeError("Native worker changed the requested receiver")
            case_incarnation = terminal["incarnationId"]
            row.update(receiver=receiver, incarnation=case_incarnation)
            if native.get("mutation", {}).get("requestId") != request:
                raise RuntimeError("Startup receipt belongs to a different request")
            prompt_incarnation = native.get("prompt", {}).get("processIncarnation")
            if prompt_incarnation and prompt_incarnation != case_incarnation:
                raise RuntimeError("Prompt receipt belongs to a different process")
            done = []
            while time.monotonic() - started_at < args.budget:
                delivery = result(case + "-inbox", "orchestration", "check", "--terminal", args.coordinator)
                messages = delivery.get("messages", [])
                for message in messages:
                    payload = message.get("payload")
                    if isinstance(payload, str):
                        payload = json.loads(payload)
                    if message.get("type") != "worker_done" or not isinstance(payload, dict) or payload.get("dispatchId") != dispatch:
                        continue
                    if (message.get("run_id") != run or message.get("to_handle") != "run:" + run or
                        message.get("from_handle") not in (receiver, "dispatch:" + dispatch) or
                        payload.get("taskId") != task or payload.get("outcome") != "succeeded" or
                        payload.get("probe_result") != {"nonce": nonce, "answer": left + right}):
                        raise RuntimeError("Result differs from its original prompt/Dispatch")
                    done.append(message["id"])
                if messages:
                    (evidence / (case + "-messages.json")).write_text(json.dumps(messages, ensure_ascii=False, indent=2), encoding="utf-8")
                if done:
                    break
                if messages:
                    result(case + "-intermediate-ack", "orchestration", "check", "--terminal", args.coordinator,
                           "--ack", delivery["deliveryId"])
                # Read-only polling: never resend text or Enter on silence.
                time.sleep(1)
            if len(done) != 1:
                raise RuntimeError("No unique correct completion within the observation budget")
            shown = result(case + "-settled", "orchestration", "worker-show", "--dispatch", dispatch)
            if (shown["dispatch"]["runId"] != run or shown["dispatch"]["taskId"] != task or
                shown["dispatch"]["status"] != "completed" or shown["worker"]["state"] != "succeeded" or
                shown["worker"]["agentTerminalHandle"] != receiver or shown["terminal"]["incarnationId"] != case_incarnation):
                raise RuntimeError("Original worker identity or settlement was not proven")
            replay_envelope, _ = call(case + "-replay", *command)
            replay = replay_envelope.get("result", {})
            if replay_envelope.get("ok") is not True or replay.get("dispatchId") != dispatch or replay.get("mutation", {}).get("replayed") is not True:
                raise RuntimeError("Same request replay did not reuse the original Dispatch")
            row["replay_verified"] = True
            row["release"] = result(case + "-release", "orchestration", "worker-release", "--dispatch", dispatch)
            result(case + "-ack", "orchestration", "check", "--terminal", args.coordinator, "--ack", delivery["deliveryId"])
            row.update(passed=True, message=done[0], seconds=round(time.monotonic() - started_at, 2))
            print(json.dumps(row), flush=True)
    fleet = result("inventory", "orchestration", "worker-list", "--run", run)
    expected = {row["dispatch"] for row in rows}
    observed = {worker["dispatchId"] for worker in fleet.get("workers", [])}
    if fleet.get("page", {}).get("hasMore") or len(fleet.get("workers", [])) != len(rows) or observed != expected:
        raise RuntimeError("Request replay created additional or replacement Dispatches")
except Exception as exc:
    failure = str(exc)
    if rows and not rows[-1].get("dispatch"):
        diagnostics = [("request-show", "orchestration", "request-show", "--request", rows[-1]["request"]),
                       ("task-list", "orchestration", "task-list", "--run", run)]
        if args.terminal:
            diagnostics += [("receiver-show", "terminal", "show", "--terminal", args.terminal),
                            ("receiver-read", "terminal", "read", "--terminal", args.terminal)]
        for name, *command in diagnostics:
            try:
                call("failure-" + name, *command)
            except Exception as diagnostic_error:
                rows[-1][name + "-diagnostic-error"] = str(diagnostic_error)
    if rows and rows[-1].get("dispatch") and rows[-1].get("initial_state") != "failed":
        for verb in ("worker-show", "worker-read"):
            try:
                call("failure-" + verb, "orchestration", verb, "--dispatch", rows[-1]["dispatch"])
            except Exception as diagnostic_error:
                rows[-1][verb + "-diagnostic-error"] = str(diagnostic_error)
finally:
    summary = {"root": str(root), "run": run, "receiver": args.terminal, "agent": args.agent, "probe_sha256": hashlib.sha256((evidence / "probe.py").read_bytes()).hexdigest(), "runtime_id": runtime_id, "incarnation": incarnation,
               "passed": failure is None, "failure": failure, "cases": rows}
    (evidence / "result.json").write_text(json.dumps(summary, indent=2), encoding="utf-8")
    print(json.dumps({"evidence": str(evidence), **summary}), flush=True)
if failure:
    raise SystemExit(1)
