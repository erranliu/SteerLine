"""Verify one saved P13 ingress fault against its isolated center SQLite.

Usage: python verify_evidence.py KIND ORIGINAL MUTATED STATE_DIR
KIND is missing_check or wrong_commit. Prints a compact JSON finding.
"""

import json
import sqlite3
import sys
from copy import deepcopy
from pathlib import Path


def main() -> None:
    kind, original_path, mutated_path, state_path = sys.argv[1:]
    assert kind in {"missing_check", "wrong_commit"}
    original = json.loads(Path(original_path).read_text(encoding="utf-8-sig"))
    mutated = json.loads(Path(mutated_path).read_text(encoding="utf-8-sig"))
    original_messages = original["result"]["messages"]
    mutated_messages = mutated["result"]["messages"]
    assert len(original_messages) == len(mutated_messages) >= 1
    assert [m["id"] for m in original_messages] == [m["id"] for m in mutated_messages]
    target_indexes = [
        index for index, message in enumerate(original_messages)
        if message["type"] == "status"
        and json.loads(message["payload"]).get("step_result", {}).get("result", {}).get("step_id") == "verify"
    ]
    assert len(target_indexes) == 1
    index = target_indexes[0]
    source_message = original_messages[index]
    changed_message = mutated_messages[index]
    assert source_message["id"] == changed_message["id"]
    assert source_message["type"] == changed_message["type"] == "status"

    source_payload = json.loads(source_message["payload"])
    changed_payload = json.loads(changed_message["payload"])
    source_result = source_payload["step_result"]["result"]
    changed_result = changed_payload["step_result"]["result"]
    assert source_result["step_id"] == changed_result["step_id"] == "verify"
    assert len(source_result["checks"]) == 1
    assert source_result["checks"][0]["status"] == "passed"
    assert source_result["checks"][0]["exit_code"] == 0
    assert source_result["checks"][0]["passed"] is True

    restored_payload = deepcopy(changed_payload)
    if kind == "missing_check":
        assert changed_result["checks"] == []
        restored_payload["step_result"]["result"]["checks"] = source_result["checks"]
        altered_field = "step_result.result.checks"
    else:
        assert len(changed_result["checks"]) == 1
        assert changed_result["checks"][0]["actual_commit"] == "0" * 40
        restored_payload["step_result"]["result"]["checks"][0]["actual_commit"] = source_result["checks"][0]["actual_commit"]
        altered_field = "step_result.result.checks[0].actual_commit"
    assert restored_payload == source_payload
    restored_response = deepcopy(mutated)
    restored_response["result"]["messages"][index]["payload"] = source_message["payload"]
    assert restored_response == original

    state_dir = Path(state_path)
    runtime = json.loads((state_dir / "runtime.json").read_text(encoding="utf-8"))
    connection = sqlite3.connect("file:" + str(state_dir / "steerline.db") + "?mode=ro", uri=True)
    try:
        delivery_count = connection.execute("select count(*) from task_deliveries").fetchone()[0]
        task_status = connection.execute("select status from tasks where task_id='91024'").fetchone()[0]
    finally:
        connection.close()
    workflow = runtime["workflow_runs"][0]
    execution = runtime["executions"][0]
    run = runtime["runs"][0]
    verify_attempts = [a for a in runtime["attempts"] if a["step_id"] == "verify"]
    assert len(verify_attempts) == 1 and verify_attempts[0]["status"] == "failed"
    assert workflow["phase"] == "blocked"
    assert execution["current_step"] == "verify" and not execution.get("verified_revision")
    assert not run.get("pr", {}).get("url") and delivery_count == 0
    assert task_status == "in_progress"
    assert source_result["checks"][0]["actual_commit"] == execution["candidate_revision"]

    print(json.dumps({
        "kind": kind,
        "altered_field": altered_field,
        "only_target_field_changed": True,
        "message_id": source_message["id"],
        "workflow_run_id": workflow["workflow_run_id"],
        "execution_id": execution["execution_id"],
        "verify_attempt_id": verify_attempts[0]["attempt_id"],
        "orca_run_id": run["target"]["orchestration_run"],
        "verify_dispatch_id": run["target"]["dispatch"],
        "candidate_revision": execution["candidate_revision"],
        "original_actual_commit": source_result["checks"][0]["actual_commit"],
        "mutated_actual_commit": changed_result["checks"][0]["actual_commit"] if kind == "wrong_commit" else None,
        "workflow_phase": workflow["phase"],
        "verify_status": verify_attempts[0]["status"],
        "verified_revision": execution.get("verified_revision"),
        "task_status": task_status,
        "delivery_count": delivery_count,
        "pr_url": run.get("pr", {}).get("url"),
    }, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
