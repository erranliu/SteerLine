import json
import pathlib
import subprocess


root = pathlib.Path(__file__).parent
worktree = "id:c67f8c64-d472-449b-82e3-24df822d0fdb::C:/Users/liuming/orca/workspaces/SteerLine/prod-capability-20260928"
run = "run_707f6533c393"
dispatch = "ctx_3ceb7b58383c"
calls = {
    "local-status.json": ["orca", "status", "--json"],
    "home-status.json": ["orca", "status", "--environment", "home", "--json"],
    "home-projects.json": ["orca", "project", "list", "--environment", "home", "--json"],
    "home-repo.json": ["orca", "repo", "show", "--environment", "home", "--repo", "id:c67f8c64-d472-449b-82e3-24df822d0fdb", "--json"],
    "home-worktree.json": ["orca", "worktree", "show", "--environment", "home", "--worktree", worktree, "--json"],
    "final-worker-show.json": ["orca", "orchestration", "worker-show", "--dispatch", dispatch, "--json"],
    "final-worker-list.json": ["orca", "orchestration", "worker-list", "--run", run, "--include-remote", "--json"],
    "final-task-list.json": ["orca", "orchestration", "task-list", "--run", run, "--json"],
    "final-mail.json": ["orca", "orchestration", "check", "--terminal", "term_a145f325-8694-41db-a47c-3db07702c1d3", "--run", run, "--all", "--json"],
    "final-terminal-list.json": ["orca", "terminal", "list", "--environment", "home", "--worktree", worktree, "--json"],
    "release-request.json": ["orca", "orchestration", "request-show", "--request", "8cab3e4d-7da4-4dc4-aee6-1bd5337fb9a5", "--json"],
}
for name, argv in calls.items():
    raw = subprocess.check_output(argv, timeout=30)
    if not json.loads(raw).get("ok"):
        raise RuntimeError(name)
    (root / name).write_bytes(raw)

raw = subprocess.check_output(["orca", "orchestration", "worker-read", "--dispatch", dispatch, "--source", "auto", "--limit", "50", "--json"], timeout=30)
document = json.loads(raw)
if not document.get("ok"):
    raise RuntimeError("worker-read")
result = document["result"]
summary = {
    "source_receipt_id": document["id"],
    "dispatch_id": dispatch,
    "source": result.get("source"),
    "source_identity": result.get("sourceIdentity"),
    "provider": result.get("provider"),
    "source_exact": result.get("sourceExact"),
    "archived": result.get("archived"),
    "content_complete": result.get("contentComplete"),
    "clipping": result.get("clipping"),
    "warnings": result.get("warnings"),
    "status": result.get("status"),
    "returned_message_count": result.get("transcript", {}).get("returnedMessageCount"),
    "remote_runtime_epoch": result.get("remoteRuntimeEpoch"),
}
(root / "worker-read-summary.json").write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")
print("captured", len(calls) + 1, "production capability receipts")
