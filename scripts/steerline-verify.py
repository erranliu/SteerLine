"""Run the fixed Go checks for one native verify attempt."""

import datetime
import json
import pathlib
import subprocess
import sys
import tempfile
import uuid


def git(*args):
    result = subprocess.run(["git", *args], check=True, text=True, capture_output=True)
    return result.stdout.strip()


def main():
    context = json.load(sys.stdin)
    if context.get("schema_version") != "native-task/v1" or context.get("current_step") != "verify":
        raise RuntimeError("expected one fixed native verify attempt")
    step_input = context["step_input"]
    candidate = step_input["candidate_revision"]
    branch = git("branch", "--show-current")
    head = git("rev-parse", "HEAD")
    if branch != context["branch"] or head != candidate or git("status", "--porcelain"):
        raise RuntimeError("verify requires the fixed branch and a clean candidate commit")

    checks = []
    with tempfile.TemporaryDirectory(prefix="steerline-go-build-") as build_dir:
        commands = {
            "go_test": ["go", "run", "./cmd/test-feedback", "-workers=3", "-out", str(pathlib.Path(build_dir) / "test-feedback")],
            "go_vet": ["go", "vet", "./..."],
            "go_build": ["go", "build", "-o", build_dir, "./..."],
        }
        required = step_input["project_steps"]["verify"]["required_checks"]
        if required != list(commands):
            raise RuntimeError("verify checks differ from the fixed project contract")
        for name, command in commands.items():
            try:
                result = subprocess.run(command, text=True, stdout=subprocess.PIPE,
                                        stderr=subprocess.STDOUT, timeout=1500)
                if result.stdout:
                    print(result.stdout, file=sys.stderr)
                passed = result.returncode == 0
            except subprocess.TimeoutExpired:
                print(name + " timed out", file=sys.stderr)
                passed = False
            checks.append({"name": name, "status": "passed" if passed else "failed", "revision": candidate})

    actual_head = git("rev-parse", "HEAD")
    dirty = bool(git("status", "--porcelain"))
    if actual_head != candidate:
        raise RuntimeError("candidate commit changed during verification")
    workspace = {
        "repository": context["project_id"],
        "worktree": str(pathlib.Path.cwd().resolve()),
        "branch": branch,
        "base": context["base_commit"],
        "head": actual_head,
        "dirty": dirty,
        "verified_at": datetime.datetime.now(datetime.timezone.utc).isoformat().replace("+00:00", "Z"),
    }
    envelope = {
        "schema_version": 1,
        "project_identity": context["project_id"],
        "task_id": context["task_id"],
        "workflow_run_id": context["workflow_run_id"],
        "execution_id": context["execution_id"],
        "attempt_id": context["attempt_id"],
        "result": {
            "step_id": "verify",
            "succeeded": all(check["status"] == "passed" for check in checks) and not dirty,
            "result_id": uuid.uuid4().hex,
            "input_revision": step_input["input_revision"],
            "revision": candidate,
            "workspace": workspace,
            "checks": checks,
            "prerequisite_result_ids": step_input.get("prerequisite_result_ids", []),
        },
    }
    print(json.dumps(envelope, separators=(",", ":")))


if __name__ == "__main__":
    main()
