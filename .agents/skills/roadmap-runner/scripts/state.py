"""Internal, offline state journal for roadmap-runner (no session or Git APIs)."""

import argparse
import copy
import json
import os
from pathlib import Path
import sys
import tempfile
from contextlib import contextmanager
from datetime import datetime, timezone


MODEL = "gpt-6-luna"
THINKING = "xhigh"
TASK_PHASES = {"pending", "creating", "running", "pr_ready", "reviewing",
               "ready_to_merge", "merged", "blocked", "needs_user_decision"}
RUN_FIELDS = {"phase", "automation", "blocker", "decisions", "finished_at", "authorization"}
TASK_FIELDS = {"phase", "session", "pr", "review", "validation", "merge_gate", "plan",
               "merge", "observed_merge", "blocker", "decisions"}
AUTH = {"create_session": "create_sessions", "send_prompt": "send_prompts",
        "comment_pr": "comment_prs", "merge_pr": "merge_prs",
        "create_heartbeat": "schedule", "pause_heartbeat": "schedule"}


class StateError(ValueError):
    pass


def require(condition, message):
    if not condition:
        raise StateError(message)


def now():
    return datetime.now(timezone.utc).isoformat()


def read_json(path):
    return json.loads(Path(path).read_text(encoding="utf-8-sig"))


def task_by_id(state, task_id):
    for task in state["tasks"]:
        if task["id"] == task_id:
            return task
    raise StateError(f"unknown task: {task_id}")


def predecessors_merged(state, task_id):
    for task in state["tasks"]:
        if task["id"] == task_id:
            return True
        if task["phase"] != "merged":
            return False
    raise StateError(f"unknown task: {task_id}")


def merge_ready(task):
    pr = task.get("pr") or {}
    head, base = pr.get("head"), pr.get("base_commit")
    require(head and base and pr.get("url"), "PR identity/head/base missing")
    review = task.get("review") or {}
    verification = task.get("validation") or {}
    gate = task.get("merge_gate") or {}
    require(all(item.get("head") == head for item in (review, verification, gate)),
            "review, validation and merge gate must match PR head")
    require(review.get("base_commit") == base and gate.get("base_commit") == base,
            "review and merge gate must match PR base")
    require(review.get("comment_url"), "PR review record missing")
    require(isinstance(review.get("findings"), list) and review.get("scope"),
            "review scope/findings missing (use [] for no findings)")
    require(verification.get("status") in {
                "passed", "passed_with_manual_pending", "accepted_exception"}
            and verification.get("commands"), "required validation not satisfied")
    if verification["status"] == "passed_with_manual_pending":
        require(verification.get("non_manual_status") == "passed",
                "non-manual validation must pass before deferring manual checks")
        pending = verification.get("manual_pending")
        require(isinstance(pending, list) and pending,
                "registered manual checks required")
        for item in pending:
            require(isinstance(item, dict) and all(
                isinstance(item.get(key), str) and item[key].strip()
                for key in ("check_id", "reason", "roadmap_path")),
                "manual check identity, reason and roadmap location required")
    if verification["status"] == "accepted_exception":
        require(verification.get("authorization_source"), "exception authorization missing")
    require(gate.get("required_checks") in {"passed", "none"}
            and gate.get("mergeable") is True and gate.get("checked_at"),
            "required checks or mergeability not confirmed")


def validate(state):
    require(state.get("schema_version") == 1, "unsupported state schema")
    require(isinstance(state.get("revision"), int) and state["revision"] >= 0,
            "invalid revision")
    for field in ("run_id", "repository", "repo_root", "project_id",
                  "coordinator_thread_id", "roadmap", "default_branch"):
        require(isinstance(state.get(field), str) and state[field].strip(),
                f"missing run identity: {field}")
    require(state.get("phase") in {"active", "paused", "needs_user_decision", "complete"},
            "invalid run phase")
    require(isinstance(state.get("authorization"), dict)
            and state["authorization"].get("source"), "human authorization source missing")
    require(isinstance(state.get("interval_minutes"), int)
            and state["interval_minutes"] > 0, "invalid interval")
    tasks = state.get("tasks")
    require(isinstance(tasks, list) and tasks, "ordered tasks required")
    ids = set()
    for task in tasks:
        require(isinstance(task, dict), "invalid task")
        task_id = task.get("id")
        require(isinstance(task_id, str) and task_id and task_id not in ids,
                "task IDs must be nonempty and unique")
        ids.add(task_id)
        require(task.get("spec"), f"{task_id}: Spec missing")
        require(task.get("phase") in TASK_PHASES, f"{task_id}: invalid phase")
        session = task.get("session") or {}
        if session and not task.get("adopted"):
            require(session.get("model") == MODEL and session.get("thinking") == THINKING,
                    f"{task_id}: execution model must be Luna xhigh")
        if task["phase"] == "running":
            require(session.get("thread_id"), f"{task_id}: real thread ID required")
        started = session or task.get("pr") or task["phase"] not in {
            "pending", "blocked", "needs_user_decision"}
        if started and not task.get("adopted"):
            require(predecessors_merged(state, task_id),
                    f"{task_id}: predecessor not merged")
        if task.get("adopted"):
            require(session.get("thread_id"), "adoption requires existing thread evidence")
        if task["phase"] in {"pr_ready", "reviewing", "ready_to_merge", "merged"}:
            pr = task.get("pr") or {}
            require(pr.get("url") and pr.get("head") and pr.get("base_commit"),
                    f"{task_id}: PR evidence missing")
            require(pr.get("base_branch") == state["default_branch"],
                    f"{task_id}: wrong PR target branch")
        if task["phase"] in {"reviewing", "ready_to_merge", "merged"}:
            require(predecessors_merged(state, task_id), "review/merge order violated")
        if task["phase"] in {"ready_to_merge", "merged"}:
            merge_ready(task)
        if task["phase"] == "merged":
            merged = task.get("merge") or {}
            require(merged.get("head") == task["pr"]["head"]
                    and merged.get("commit") and merged.get("merged_at"),
                    "actual merged head/commit/time required")
    require(state["phase"] != "complete" or all(t["phase"] == "merged" for t in tasks),
            "cannot complete before every selected task is merged")
    actions = state.get("actions")
    require(isinstance(actions, dict), "actions journal missing")
    for action in actions.values():
        require(action.get("kind") in AUTH and action.get("status") in {
            "pending", "confirmed", "not_applied"}, "invalid action")
    require(sum(a["status"] == "pending" and a["kind"] != "pause_heartbeat"
                for a in actions.values()) <= 1,
            "multiple unresolved external actions")
    require(sum(a["status"] == "pending" and a["kind"] == "pause_heartbeat"
                for a in actions.values()) <= 1, "multiple unresolved heartbeat pauses")


@contextmanager
def locked(path):
    path.parent.mkdir(parents=True, exist_ok=True)
    lock_path = path.with_name(path.name + ".lock")
    try:
        fd = os.open(lock_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError:
        raise StateError("state locked; inspect owner, do not auto-expire") from None
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as stream:
            json.dump({"pid": os.getpid(), "created_at": now()}, stream)
        yield
    finally:
        lock_path.unlink()


def write_atomic(path, state):
    validate(state)
    # Never put live coordinator state inside any recorded repository/worktree.
    roots = [state["repo_root"]] + [
        (t.get("session") or {}).get("worktree") for t in state["tasks"]]
    for root in filter(None, roots):
        require(not path.is_relative_to(Path(root).resolve()),
                "live state must be outside repository/worktrees")
    fd, temporary = tempfile.mkstemp(prefix=path.name + ".", suffix=".tmp", dir=path.parent)
    try:
        with os.fdopen(fd, "w", encoding="utf-8", newline="\n") as stream:
            json.dump(state, stream, ensure_ascii=True, indent=2)
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def initialize(path, config):
    with locked(path):
        require(not path.exists(), "state already exists; resume it")
        state = copy.deepcopy(config)
        state.update(schema_version=1, revision=0, phase="active", actions={}, updated_at=now())
        state.setdefault("interval_minutes", 20)
        for task in state.get("tasks", []):
            task.setdefault("phase", "pending")
        write_atomic(path, state)
        return state


def apply_patch(state, patch):
    require(isinstance(patch, dict) and set(patch) <= {"run", "tasks"}, "invalid patch keys")
    run = patch.get("run", {})
    require(isinstance(run, dict) and set(run) <= RUN_FIELDS, "run identity is immutable")
    if "authorization" in run:
        require(isinstance(run["authorization"], dict) and run["authorization"].get("source"),
                "new human authorization source required")
        state.setdefault("authorization_history", []).append({
            "replaced_at": now(), "authorization": copy.deepcopy(state["authorization"])})
    state.update(run)
    tasks = patch.get("tasks", {})
    require(isinstance(tasks, dict), "task patches must be keyed by task ID")
    for task_id, changes in tasks.items():
        task = task_by_id(state, task_id)
        require(isinstance(changes, dict) and set(changes) <= TASK_FIELDS,
                "task identity/order is immutable or patch key unknown")
        require(task["phase"] != "merged", "merged task evidence is immutable")
        if "plan" in changes:
            require(not task.get("plan") and isinstance(changes["plan"], str) and changes["plan"],
                    "only an initially unbound Plan can be bound; existing mapping is immutable")
        old_pr = task.get("pr") or {}
        new_pr = changes.get("pr") or {}
        candidate_changed = bool(old_pr and new_pr and any(
            old_pr.get(key) != new_pr.get(key) for key in ("url", "head", "base_branch", "base_commit")))
        if candidate_changed:
            task.setdefault("review_history", []).append({
                "invalidated_at": now(), "pr": copy.deepcopy(old_pr),
                **{k: copy.deepcopy(task.get(k)) for k in ("review", "validation", "merge_gate")}})
        task.update(changes)
        if candidate_changed:
            for key in ("review", "validation", "merge_gate"):
                task.pop(key, None)
            # Record the new candidate first; certify it in a separate transaction.
            task["phase"] = "pr_ready"


def reserve(state, key, kind, task_id, request):
    require(key and key not in state["actions"], "action key already used")
    pending = [a for a in state["actions"].values() if a["status"] == "pending"]
    if kind == "pause_heartbeat":
        require(not any(a["kind"] == kind for a in pending), "heartbeat pause already pending")
    else:
        require(not pending, "unknown external action; reconcile before another action")
    require(state["authorization"].get(AUTH[kind]) is True, "action lacks user authorization")
    require(isinstance(request, dict) and request, "exact request summary required")
    if kind == "pause_heartbeat":
        require(task_id is None, "heartbeat is a run action")
    else:
        require(state["phase"] == "active", "run is not active")
    if kind in {"create_session", "send_prompt", "comment_pr", "merge_pr"}:
        task = task_by_id(state, task_id)
        require(task["phase"] not in {"merged", "blocked", "needs_user_decision"},
                "task is not actionable")
        if kind != "send_prompt" or not task.get("adopted"):
            require(predecessors_merged(state, task_id), "predecessor not merged")
        if kind in {"create_session", "send_prompt"}:
            require(request.get("model") == MODEL and request.get("thinking") == THINKING,
                    "external session request must explicitly use Luna xhigh")
        if kind == "create_session":
            require(task["phase"] == "pending" and not task.get("session")
                    and not task.get("pr"), "task already started")
            task["phase"] = "creating"
        if kind == "send_prompt":
            require((task.get("session") or {}).get("thread_id"), "real thread ID required")
        if kind == "merge_pr":
            require(task["phase"] == "ready_to_merge", "task not independently reviewed")
            merge_ready(task)
            require(request.get("expected_head") == task["pr"]["head"], "wrong expected head")
        if kind == "comment_pr":
            require(request.get("head") == (task.get("pr") or {}).get("head")
                    and request.get("head"), "comment candidate head missing/mismatched")
    else:
        require(task_id is None, "heartbeat is a run action")
    state["actions"][key] = {"kind": kind, "task_id": task_id, "request": request,
                              "status": "pending", "reserved_at": now()}


def settle(state, key, status, evidence):
    action = state["actions"].get(key)
    require(action and action["status"] == "pending", "no pending action with this key")
    require(status in {"confirmed", "not_applied"} and isinstance(evidence, dict) and evidence,
            "external outcome evidence required")
    kind = action["kind"]
    if kind == "create_session":
        task = task_by_id(state, action["task_id"])
        session = task.get("session") or {}
        if status == "confirmed":
            require(session.get("client_thread_id") or session.get("thread_id"),
                    "persist returned session identity before confirming creation")
        else:
            require(not session and not task.get("pr"), "existing session contradicts not_applied")
            task["phase"] = "pending"
    if status == "confirmed" and kind == "merge_pr":
        task = task_by_id(state, action["task_id"])
        require(task["phase"] == "merged"
                and task["merge"]["head"] == action["request"]["expected_head"],
                "persist platform merged evidence before confirming merge")
    if status == "confirmed" and kind in {"create_heartbeat", "pause_heartbeat"}:
        automation = state.get("automation") or {}
        require(automation.get("id"), "persist automation identity first")
        if kind == "pause_heartbeat":
            require(automation.get("status") == "PAUSED", "pause not yet verified")
    action.update(status=status, evidence=evidence, settled_at=now())


def transact(path, expected, operation):
    with locked(path):
        state = read_json(path)
        validate(state)
        require(state["revision"] == expected, "revision changed; reread and reconcile")
        operation(state)
        state.update(revision=state["revision"] + 1, updated_at=now())
        write_atomic(path, state)
        return state


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    for name in ("init", "show", "apply", "reserve", "settle"):
        command = commands.add_parser(name)
        command.add_argument("--state", type=Path, required=True)
        if name != "show":
            command.add_argument("--input", type=Path, required=True)
        if name not in {"init", "show"}:
            command.add_argument("--expect", type=int, required=True)
        if name in {"reserve", "settle"}:
            command.add_argument("--key", required=True)
        if name == "reserve":
            command.add_argument("--kind", choices=list(AUTH), required=True)
            command.add_argument("--task")
        if name == "settle":
            command.add_argument("--status", choices=["confirmed", "not_applied"], required=True)
    args = parser.parse_args()
    path = args.state.resolve()
    try:
        if args.command == "show":
            state = read_json(path)
            validate(state)
        elif args.command == "init":
            state = initialize(path, read_json(args.input))
        else:
            payload = read_json(args.input)
            if args.command == "apply":
                operation = lambda state: apply_patch(state, payload)
            elif args.command == "reserve":
                operation = lambda state: reserve(state, args.key, args.kind, args.task, payload)
            else:
                operation = lambda state: settle(state, args.key, args.status, payload)
            state = transact(path, args.expect, operation)
        print(json.dumps(state, ensure_ascii=True))
    except (StateError, OSError, ValueError, TypeError, KeyError, AttributeError) as error:
        print(json.dumps({"error": str(error)}, ensure_ascii=True), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
