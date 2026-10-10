"""Exercise only Orca's prompt API on a ready receiver in a debug release."""
import argparse
import json
from pathlib import Path
import subprocess
import time
import uuid

parser = argparse.ArgumentParser()
parser.add_argument("--root", type=Path, required=True)
parser.add_argument("--orca", required=True)
parser.add_argument("--terminal", required=True)
parser.add_argument("--lengths", default="500,7500")
parser.add_argument("--repeat", type=int, default=3)
parser.add_argument("--budget", type=int, default=90)
parser.add_argument("--submit-wait", type=int, default=10, help="Observe the original submit without sending again")
args = parser.parse_args()
lengths = list(map(int, args.lengths.split(",")))
if args.repeat < 1 or args.budget < 1 or args.submit_wait < 0 or args.submit_wait > 40 or not lengths or min(lengths) < 1:
    parser.error("lengths, repeat and budget must be positive")
root = args.root.resolve(strict=True)
root.relative_to(Path("C:/SteerlineDebug").resolve(strict=True))
if not (root / "manifest.json").is_file():
    raise SystemExit("Expected a generated debug release")
repo = (root / "projects/SteerlineTest").resolve(strict=True)
repo.relative_to(root)
out = root / "evidence" / ("orca-transport-" + uuid.uuid4().hex[:8])
out.mkdir()
(out / "probe.py").write_bytes(Path(__file__).read_bytes())
sequence = 0
runtime_id = None

def call(name, *values):
    global sequence, runtime_id
    sequence += 1
    response = subprocess.run([args.orca, *values, "--json"], capture_output=True, timeout=60)
    record = out / (str(sequence).zfill(3) + "-" + name)
    record.with_suffix(".json").write_bytes(response.stdout)
    if response.stderr:
        record.with_suffix(".stderr").write_bytes(response.stderr)
    value = json.loads(response.stdout)
    observed = value.get("_meta", {}).get("runtimeId")
    if not observed or runtime_id is not None and observed != runtime_id:
        raise RuntimeError("Orca runtime identity changed or disappeared")
    runtime_id = observed
    if response.returncode or value.get("ok") is not True:
        raise RuntimeError("Orca refused " + name)
    return value["result"]

initial = call("identity", "terminal", "show", "--terminal", args.terminal)["terminal"]
if Path(initial["worktreePath"]).resolve() != repo or not initial.get("connected"):
    raise SystemExit("Receiver must belong to this exact debug workspace")
incarnation = initial["incarnationId"]
rows = []
failure = None
try:
    for length in lengths:
        for repetition in range(args.repeat):
            nonce, request = uuid.uuid4().hex, str(uuid.uuid4())
            result_path = out / (nonce + ".json")
            left, right = 71 + repetition, length + 19
            prompt = (f"Do not touch repository files, git, PRs or other tasks. Compute {left}+{right}. "
                      f"Write ONLY {result_path} as one JSON object with nonce=\"{nonce}\" and answer=<computed integer>. "
                      "Then stop. This is an isolated prompt-delivery test. Following padding is inert:\n")
            padding = "Readonly context padding.\n"
            needed = max(0, length - len(prompt))
            prompt += (padding * ((needed + len(padding) - 1) // len(padding)))[:needed]
            row = {"nonce": nonce, "request": request, "chars": len(prompt), "result_path": str(result_path), "passed": False}
            rows.append(row)
            wait = call("ready", "terminal", "wait", "--terminal", args.terminal,
                        "--for", "tui-idle", "--timeout-ms", "20000")["wait"]
            current = call("identity", "terminal", "show", "--terminal", args.terminal)["terminal"]
            if not wait.get("satisfied") or wait.get("blockedReason") or current["incarnationId"] != incarnation:
                raise RuntimeError("Original receiver is not ready")
            values = ("terminal", "send", "--terminal", args.terminal, "--text", prompt,
                      "--enter", "--wait-submit", str(args.submit_wait), "--retry-request", request)
            start = time.monotonic()
            receipt = call("send", *values)
            proof = receipt.get("send", {}).get("prompt", {})
            row["stages"] = proof.get("stages", [])
            if (not receipt["send"].get("accepted") or "turn_started" not in row["stages"] or
                proof.get("requestId") != request or proof.get("processIncarnation") != incarnation):
                raise RuntimeError("Original prompt start was not proven; no extra Enter or resend")
            while not result_path.exists() and time.monotonic() - start < args.budget:
                time.sleep(1)
            if not result_path.exists():
                raise RuntimeError("No computed result within budget")
            if json.loads(result_path.read_text(encoding="utf-8-sig")) != {"nonce": nonce, "answer": left + right}:
                raise RuntimeError("Incorrect original prompt result")
            replay = call("replay", *values)
            if not replay.get("mutation", {}).get("replayed") or replay.get("send", {}).get("prompt", {}).get("requestId") != request:
                raise RuntimeError("Same-request replay was not proven")
            row.update(passed=True, seconds=round(time.monotonic() - start, 2))
            print(json.dumps(row), flush=True)
except Exception as exc:
    failure = str(exc)
    for verb in ("show", "read"):
        try:
            call("failure-" + verb, "terminal", verb, "--terminal", args.terminal)
        except Exception as diagnostic_error:
            failure += "; " + verb + ": " + str(diagnostic_error)
finally:
    summary = {"terminal": args.terminal, "incarnation": incarnation, "runtime": runtime_id,
               "passed": failure is None, "failure": failure, "cases": rows}
    (out / "result.json").write_text(json.dumps(summary, indent=2), encoding="utf-8")
    print(json.dumps({"evidence": str(out), **summary}), flush=True)
if failure:
    raise SystemExit(1)
