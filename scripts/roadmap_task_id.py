#!/usr/bin/env python3
"""Reserve monotonically increasing roadmap task IDs across Git worktrees."""

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path


REF = "refs/steerline/roadmap-task-ids"
ZERO_OID = "0" * 40
ROW = re.compile(r"^\|\s*(\d+)\s*\|\s*([^|]+?)\s*\|", re.MULTILINE)
SPEC_LINK = re.compile(r"\[Spec\]\(([^)]+)\)")
PLAN_LINK = re.compile(r"\[Plan\]\(([^)]+)\)")
TASK_HEADER = re.compile(r"任务 ID：(\d+)；任务名称：([^。；\r\n]+)")


def git(root: Path, *args: str, input_text: str | None = None, check: bool = True):
    result = subprocess.run(
        ["git", *args], cwd=root, input=input_text, text=True,
        encoding="utf-8", capture_output=True,
    )
    if check and result.returncode:
        raise RuntimeError(result.stderr.strip() or f"git {' '.join(args)} failed")
    return result


def repository_root() -> Path:
    return Path(git(Path.cwd(), "rev-parse", "--show-toplevel").stdout.strip())


def registry(root: Path) -> dict[int, str]:
    records = {}
    directory = root / "docs" / "roadmap" / "task-ids"
    for path in directory.glob("*.json"):
        record = json.loads(path.read_text(encoding="utf-8"))
        task_id, name = record.get("id"), record.get("name")
        if type(task_id) is not int or task_id < 0 or path.stem != str(task_id):
            raise ValueError(f"invalid task ID in {path}")
        if not isinstance(name, str) or not name.strip() or name != name.strip():
            raise ValueError(f"invalid task name in {path}")
        if task_id in records:
            raise ValueError(f"duplicate task ID {task_id}")
        records[task_id] = name
    return records


def ref_oid(root: Path) -> str | None:
    result = git(root, "rev-parse", "--verify", "--quiet", REF, check=False)
    if result.returncode == 1:
        return None
    if result.returncode:
        raise RuntimeError(result.stderr.strip())
    return result.stdout.strip()


def load_state(root: Path, oid: str | None, records: dict[int, str]) -> dict:
    if oid is None:
        state = {"version": 1, "next_id": max(records, default=-1) + 1,
                 "reservations": {str(k): v for k, v in records.items()}}
    else:
        state = json.loads(git(root, "cat-file", "blob", oid).stdout)
    reservations = state.get("reservations")
    next_id = state.get("next_id")
    if state.get("version") != 1 or not isinstance(reservations, dict):
        raise ValueError("invalid roadmap task ID ref state")
    if type(next_id) is not int or next_id < 0:
        raise ValueError("invalid next_id in roadmap task ID ref")
    for key, name in reservations.items():
        if not key.isdecimal() or str(int(key)) != key or not name:
            raise ValueError("invalid reservation in roadmap task ID ref")
    if any(int(key) >= next_id for key in reservations):
        raise ValueError("roadmap task ID ref would reuse an allocated ID")
    for task_id, name in records.items():
        if reservations.get(str(task_id)) != name:
            raise ValueError(f"registry entry {task_id} differs from shared Git ref")
    return state


def update_ref(root: Path, state: dict, old_oid: str | None) -> bool:
    payload = json.dumps(state, ensure_ascii=False, sort_keys=True, separators=(",", ":")) + "\n"
    new_oid = git(root, "hash-object", "-w", "--stdin", input_text=payload).stdout.strip()
    result = git(root, "update-ref", REF, new_oid, old_oid or ZERO_OID, check=False)
    return result.returncode == 0


def initialize_or_reserve(root: Path, records: dict[int, str], name: str | None) -> int | None:
    for _ in range(32):
        old_oid = ref_oid(root)
        state = load_state(root, old_oid, records)
        if name is None and old_oid is not None:
            return None
        task_id = None
        if name is not None:
            task_id = state["next_id"]
            if task_id in records:
                raise ValueError(f"task ID {task_id} already exists in this worktree")
            state["reservations"][str(task_id)] = name
            state["next_id"] = task_id + 1
        if update_ref(root, state, old_oid):
            return task_id
    raise RuntimeError("could not update shared task ID ref after concurrent allocations")


def rename_task(root: Path, records: dict[int, str], task_id: int, name: str) -> None:
    if task_id not in records:
        raise ValueError(f"task ID {task_id} has no identity file in this worktree")
    for _ in range(32):
        old_oid = ref_oid(root)
        if old_oid is None:
            raise ValueError("shared Git ref is absent; run init first")
        state = load_state(root, old_oid, records)
        state["reservations"][str(task_id)] = name
        if update_ref(root, state, old_oid):
            path = root / "docs" / "roadmap" / "task-ids" / f"{task_id}.json"
            try:
                path.write_text(
                    json.dumps({"id": task_id, "name": name}, ensure_ascii=False, indent=2) + "\n",
                    encoding="utf-8",
                )
            except OSError as exc:
                raise RuntimeError(f"shared ref renamed {task_id}, but {path} needs repair: {exc}") from exc
            return
    raise RuntimeError("could not rename task after concurrent ref updates")


def document_tasks(path: Path) -> dict[int, tuple[str, str, str]]:
    found = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        match = ROW.match(line)
        if match is None:
            continue
        raw_id, raw_name = match.groups()
        task_id = int(raw_id)
        name = raw_name.strip()
        if task_id in found:
            raise ValueError(f"duplicate task ID {task_id} in {path}")
        spec = SPEC_LINK.search(line)
        plan = PLAN_LINK.search(line)
        if spec is None or plan is None:
            raise ValueError(f"task {task_id} in {path} requires its own Spec and Plan links")
        for target in (spec.group(1), plan.group(1)):
            if not (path.parent / target).is_file():
                raise ValueError(f"task {task_id} in {path} links to missing file {target}")
        found[task_id] = (name, spec.group(1), plan.group(1))
    return found


def check_documents(root: Path, records: dict[int, str]) -> None:
    roadmap = document_tasks(root / "docs" / "roadmap" / "roadmap.md")
    execution_map = document_tasks(root / "docs" / "roadmap" / "execution-map.md")
    if roadmap != execution_map:
        raise ValueError("roadmap.md and execution-map.md have different active IDs, names, or Spec/Plan links")
    specs = [spec for _, spec, _ in roadmap.values()]
    plans = [plan for _, _, plan in roadmap.values()]
    if len(specs) != len(set(specs)) or len(plans) != len(set(plans)):
        raise ValueError("multiple active tasks share the same Spec or Plan")
    for task_id, (name, spec, plan) in roadmap.items():
        if records.get(task_id) != name:
            raise ValueError(f"active task {task_id} differs from task-ids/{task_id}.json")
        for target in (spec, plan):
            file = root / "docs" / "roadmap" / target
            header = TASK_HEADER.search(file.read_text(encoding="utf-8")[:2000])
            if header is None or int(header.group(1)) != task_id or header.group(2) != name:
                raise ValueError(f"task {task_id} identity differs from {file}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("init", help="initialize shared Git ref from tracked task IDs")
    reserve = commands.add_parser("reserve", help="reserve a new ID and create its identity file")
    reserve.add_argument("name", help="stable task name")
    rename = commands.add_parser("rename", help="change a task name while keeping its ID")
    rename.add_argument("id", type=int)
    rename.add_argument("name")
    commands.add_parser("check", help="check ref, identity files, and active roadmap rows")
    args = parser.parse_args()
    root = repository_root()
    records = registry(root)
    if args.command in {"reserve", "rename"}:
        name = args.name.strip()
        if not name or "\n" in name or "|" in name:
            raise ValueError("task name must be nonempty and contain no newline or pipe")
    if args.command == "reserve":
        task_id = initialize_or_reserve(root, records, name)
        path = root / "docs" / "roadmap" / "task-ids" / f"{task_id}.json"
        try:
            path.parent.mkdir(parents=True, exist_ok=True)
            with path.open("x", encoding="utf-8", newline="\n") as output:
                json.dump({"id": task_id, "name": name}, output, ensure_ascii=False, indent=2)
                output.write("\n")
        except OSError as exc:
            raise RuntimeError(f"ID {task_id} was reserved; repair {path} or leave it unused: {exc}") from exc
        print(f"reserved {task_id}: {name} ({path.relative_to(root)})")
    elif args.command == "rename":
        rename_task(root, records, args.id, name)
        print(f"renamed {args.id}: {name}; update roadmap tables and Spec/Plan headers before check")
    elif args.command == "init":
        initialize_or_reserve(root, records, None)
        print(f"shared Git ref initialized; next ID: {load_state(root, ref_oid(root), records)['next_id']}")
    else:
        if ref_oid(root) is None:
            raise ValueError("shared Git ref is absent; run init first")
        state = load_state(root, ref_oid(root), records)
        check_documents(root, records)
        print(f"checked {len(records)} identity files; next ID: {state['next_id']}")


if __name__ == "__main__":
    try:
        main()
    except (OSError, RuntimeError, ValueError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        sys.exit(1)
