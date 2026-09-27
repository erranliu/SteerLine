import datetime
import hashlib
import json
import pathlib


root = pathlib.Path(__file__).parent
sources = {
    "start": ["home-worktree.json", "final-worker-show.json"],
    "observe": ["final-worker-show.json", "final-worker-list.json"],
    "workspace_inspection": ["home-repo.json", "home-worktree.json", "host-git-read.json"],
    "output_reading": ["worker-read-summary.json"],
    "message_inbox": ["final-mail.json", "final-task-list.json"],
    "resource_settlement": ["release-summary.json", "final-terminal-list.json"],
}
capabilities = {}
for name, filenames in sources.items():
    capabilities[name] = [
        {"file": filename, "sha256": hashlib.sha256((root / filename).read_bytes()).hexdigest()}
        for filename in filenames
    ]
manifest = {
    "version": 1,
    "captured_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
    "project": "github:erranliu/steerline",
    "center_orca_version": "1.4.209",
    "home_orca_version": "1.4.210",
    "remote_codex_version": "codex-cli 0.157.1",
    "remote_repository_id": "c67f8c64-d472-449b-82e3-24df822d0fdb",
    "remote_worktree_id": "c67f8c64-d472-449b-82e3-24df822d0fdb::C:/Users/liuming/orca/workspaces/SteerLine/prod-capability-20260928",
    "successful_dispatch": "ctx_3ceb7b58383c",
    "failed_reused_terminal_dispatch": "ctx_5dfac74462a6",
    "capabilities": capabilities,
    "limits": "The first reused-terminal Dispatch could send status but not worker_done and was explicitly stopped. The successful proof uses worker-start --agent and normal worker-release; it is an Orca project capability probe, not a SteerLine native Task on the production repository.",
}
(root / "capability-manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
print("manifest:", len(capabilities), "required capabilities,", sum(map(len, capabilities.values())), "content-bound source receipts")
