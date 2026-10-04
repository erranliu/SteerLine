"""Offline recovery/ordering tests; never touch real sessions, PRs or run records."""

import copy
from concurrent.futures import ThreadPoolExecutor
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import state as journal


class JournalTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.path = self.root / "outside" / "run.json"
        self.config = {
            "run_id": "test-run", "repository": "example/repo",
            "repo_root": str(self.root / "repo"), "project_id": "project",
            "coordinator_thread_id": "coordinator", "roadmap": str(self.root / "roadmap.md"),
            "default_branch": "main", "authorization": {
                "source": "user message", "create_sessions": True, "send_prompts": True,
                "merge_prs": True, "comment_prs": True, "schedule": True},
            "tasks": [{"id": "A", "spec": "a.spec.md", "plan": "a.plan.md"},
                      {"id": "B", "spec": "b.spec.md", "plan": "b.plan.md"}]}
        journal.initialize(self.path, self.config)

    def load(self):
        return journal.read_json(self.path)

    def transact(self, operation):
        return journal.transact(self.path, self.load()["revision"], operation)

    def reserve(self, key="A:create", kind="create_session", task_id="A", request=None):
        request = request or {"model": journal.MODEL, "thinking": journal.THINKING,
                              "prompt": "implement, self-review and open PR"}
        return self.transact(lambda state: journal.reserve(state, key, kind, task_id, request))

    def patch(self, task_id, changes):
        return self.transact(lambda state: journal.apply_patch(state, {"tasks": {task_id: changes}}))

    def ready(self, task_id="A"):
        # Stand-in facts for an offline fixture, not external evidence.
        head, base = "a" * 40, "b" * 40
        return self.patch(task_id, {
            "phase": "ready_to_merge",
            "session": {"thread_id": "worker-" + task_id, "model": journal.MODEL,
                        "thinking": journal.THINKING},
            "pr": {"url": "https://example.invalid/pr/1", "head": head,
                   "base_branch": "main", "base_commit": base},
            "review": {"head": head, "base_commit": base,
                       "comment_url": "https://example.invalid/comment/1",
                       "findings": [], "scope": "Spec/Plan/diff/affected callers"},
            "validation": {"head": head, "status": "passed", "commands": ["targeted test: passed"]},
            "merge_gate": {"head": head, "base_commit": base, "required_checks": "passed",
                           "mergeable": True, "checked_at": journal.now()}})

    def merged(self, task_id="A"):
        return self.patch(task_id, {"phase": "merged", "merge": {
            "head": "a" * 40, "commit": "c" * 40, "merged_at": journal.now()}})

    def test_unknown_creation_survives_restart_without_duplicate(self):
        self.reserve()
        resumed = self.load()
        journal.validate(resumed)
        self.assertEqual(resumed["actions"]["A:create"]["status"], "pending")
        with self.assertRaises(journal.StateError):
            self.reserve(key="A:create:retry")
        with self.assertRaises(journal.StateError):
            self.transact(lambda state: journal.settle(state, "A:create", "confirmed", {"ok": True}))
        self.patch("A", {"session": {"client_thread_id": "queued-client",
                                     "model": journal.MODEL, "thinking": journal.THINKING}})
        self.transact(lambda state: journal.settle(state, "A:create", "confirmed",
                                                  {"client_thread_id": "queued-client"}))
        # Settled creation still cannot be recreated, even with a new action key.
        with self.assertRaises(journal.StateError):
            self.reserve(key="A:create:another")
        self.assertEqual(self.load()["tasks"][0]["session"]["client_thread_id"], "queued-client")

    def test_prompt_sent_once_and_client_id_is_not_real_thread(self):
        self.patch("A", {"phase": "creating", "session": {
            "client_thread_id": "client", "model": journal.MODEL, "thinking": journal.THINKING}})
        with self.assertRaises(journal.StateError):
            self.reserve("A:self-review", "send_prompt")
        self.patch("A", {"phase": "running", "session": {
            "thread_id": "actual-thread", "model": journal.MODEL, "thinking": journal.THINKING}})
        self.reserve("A:self-review", "send_prompt")
        self.transact(lambda state: journal.settle(state, "A:self-review", "confirmed", {"turn": "1"}))
        with self.assertRaises(journal.StateError):
            self.reserve("A:self-review", "send_prompt")

    def test_fixed_execution_model_and_authorization(self):
        with self.assertRaises(journal.StateError):
            self.reserve(request={"model": "gpt-6-sol", "thinking": "xhigh"})
        with self.assertRaises(journal.StateError):
            self.patch("A", {"session": {"thread_id": "id", "model": journal.MODEL, "thinking": "low"}})
        state = self.load()
        state["authorization"]["create_sessions"] = False
        with self.assertRaises(journal.StateError):
            journal.reserve(state, "create", "create_session", "A",
                            {"model": journal.MODEL, "thinking": journal.THINKING})

    def test_start_next_only_after_platform_merge_evidence(self):
        self.ready()
        with self.assertRaises(journal.StateError):
            self.reserve("B:create", task_id="B")
        with self.assertRaises(journal.StateError):
            self.patch("A", {"phase": "merged"})
        self.merged()
        self.reserve("B:create", task_id="B")
        self.assertEqual(self.load()["tasks"][1]["phase"], "creating")

    def test_head_and_base_changes_invalidate_old_review(self):
        for key in ("head", "base_commit"):
            with self.subTest(key=key):
                fixture = copy.deepcopy(self.ready() if key == "head" else reviewed)
                reviewed = copy.deepcopy(fixture)
                new_pr = copy.deepcopy(fixture["tasks"][0]["pr"])
                new_pr[key] = "d" * 40
                journal.apply_patch(fixture, {"tasks": {"A": {"pr": new_pr}}})
                journal.validate(fixture)
                candidate = fixture["tasks"][0]
                self.assertEqual(candidate["phase"], "pr_ready")
                self.assertNotIn("review", candidate)
                self.assertTrue(candidate["review_history"][0]["review"]["comment_url"])
                candidate["phase"] = "ready_to_merge"
                with self.assertRaises(journal.StateError):
                    journal.validate(fixture)

    def test_merge_request_must_match_reviewed_head(self):
        self.ready()
        with self.assertRaises(journal.StateError):
            self.reserve("A:merge", "merge_pr", request={"expected_head": "wrong"})
        self.reserve("A:merge", "merge_pr", request={"expected_head": "a" * 40})
        with self.assertRaises(journal.StateError):
            self.transact(lambda state: journal.settle(state, "A:merge", "confirmed", {"ok": True}))
        self.merged()
        self.transact(lambda state: journal.settle(state, "A:merge", "confirmed", {"merged": True}))

    def test_required_failure_cannot_be_certified(self):
        reviewed = self.ready()
        for bad in ("failed", "not_run"):
            fixture = copy.deepcopy(reviewed)
            fixture["tasks"][0]["validation"]["status"] = bad
            with self.assertRaises(journal.StateError):
                journal.validate(fixture)
        fixture = copy.deepcopy(reviewed)
        fixture["tasks"][0]["validation"]["status"] = "accepted_exception"
        with self.assertRaises(journal.StateError):
            journal.validate(fixture)
        fixture["tasks"][0]["merge_gate"]["required_checks"] = "UNKNOWN"
        with self.assertRaises(journal.StateError):
            journal.validate(fixture)

    def manual_validation(self):
        verification = copy.deepcopy(self.load()["tasks"][0]["validation"])
        verification.update(status="passed_with_manual_pending", non_manual_status="passed",
                            manual_pending=[{"check_id": "manual-input",
                                             "reason": "awaits human IME interaction",
                                             "roadmap_path": self.config["roadmap"]}])
        return verification

    def test_registered_manual_pending_allows_merge_and_next_task(self):
        self.ready()
        verification = self.manual_validation()
        self.patch("A", {"validation": verification})
        self.reserve("A:merge", "merge_pr", request={"expected_head": "a" * 40})
        self.merged()
        self.transact(lambda state: journal.settle(state, "A:merge", "confirmed",
                                                  {"merged": True}))
        self.reserve("B:create", task_id="B")
        saved = self.load()
        self.assertEqual(saved["tasks"][0]["validation"]["manual_pending"],
                         verification["manual_pending"])
        self.assertEqual(saved["tasks"][1]["phase"], "creating")
        self.ready("B")
        self.transact(lambda state: journal.settle(state, "B:create", "confirmed",
                                                  {"thread_id": "worker-B"}))
        self.merged("B")
        self.transact(lambda state: journal.apply_patch(state, {"run": {"phase": "complete"}}))
        self.assertEqual(self.load()["phase"], "complete")
        self.assertEqual(self.load()["tasks"][0]["validation"]["manual_pending"],
                         verification["manual_pending"])

    def test_manual_pending_requires_passed_non_manual_checks_and_registration(self):
        reviewed = self.ready()
        valid = self.manual_validation()
        invalid = []
        for status in (None, "failed", "not_run", "unsupported"):
            invalid.append(dict(valid, non_manual_status=status))
        for pending in (None, [], "manual", [None]):
            invalid.append(dict(valid, manual_pending=pending))
        for field in ("check_id", "reason", "roadmap_path"):
            item = dict(valid["manual_pending"][0])
            item[field] = " "
            invalid.append(dict(valid, manual_pending=[item]))
        for verification in invalid:
            with self.subTest(verification=verification):
                fixture = copy.deepcopy(reviewed)
                fixture["tasks"][0]["validation"] = verification
                with self.assertRaises(journal.StateError):
                    journal.validate(fixture)

    def test_manual_pending_keeps_review_checks_and_invalidation_gates(self):
        self.ready()
        reviewed = self.patch("A", {"validation": self.manual_validation()})
        for section, field, value in (("review", "comment_url", None),
                                      ("validation", "head", "wrong"),
                                      ("merge_gate", "required_checks", "UNKNOWN"),
                                      ("merge_gate", "mergeable", False)):
            with self.subTest(section=section, field=field):
                fixture = copy.deepcopy(reviewed)
                fixture["tasks"][0][section][field] = value
                with self.assertRaises(journal.StateError):
                    journal.validate(fixture)
        fixture = copy.deepcopy(reviewed)
        new_pr = dict(fixture["tasks"][0]["pr"], head="d" * 40)
        journal.apply_patch(fixture, {"tasks": {"A": {"pr": new_pr}}})
        journal.validate(fixture)
        task = fixture["tasks"][0]
        self.assertEqual(task["phase"], "pr_ready")
        self.assertNotIn("validation", task)
        self.assertEqual(task["review_history"][0]["validation"]["manual_pending"],
                         reviewed["tasks"][0]["validation"]["manual_pending"])

    def test_concurrent_wakeups_cannot_both_reserve_creation(self):
        def attempt(index):
            try:
                journal.transact(self.path, 0, lambda state: journal.reserve(
                    state, f"A:create:{index}", "create_session", "A",
                    {"model": journal.MODEL, "thinking": journal.THINKING}))
                return True
            except journal.StateError:
                return False
        with ThreadPoolExecutor(max_workers=8) as pool:
            outcomes = list(pool.map(attempt, range(8)))
        self.assertEqual(sum(outcomes), 1)
        self.assertEqual(len(self.load()["actions"]), 1)
        self.assertEqual(self.load()["revision"], 1)

    def test_verified_non_creation_allows_new_attempt(self):
        self.reserve()
        self.transact(lambda state: journal.settle(state, "A:create", "not_applied",
                                                  {"verified": "request rejected before dispatch"}))
        self.reserve("A:create:2")
        self.assertEqual(len(self.load()["actions"]), 2)

    def test_user_can_pause_heartbeat_while_creation_is_unknown(self):
        self.reserve()
        self.transact(lambda state: journal.apply_patch(state, {"run": {
            "phase": "paused", "automation": {"id": "heartbeat", "status": "ACTIVE"}}}))
        self.reserve("stop", "pause_heartbeat", task_id=None, request={"id": "heartbeat"})
        self.transact(lambda state: journal.apply_patch(state, {"run": {
            "automation": {"id": "heartbeat", "status": "PAUSED"}}}))
        self.transact(lambda state: journal.settle(state, "stop", "confirmed", {"status": "PAUSED"}))
        resumed = self.load()
        self.assertEqual(resumed["phase"], "paused")
        self.assertEqual(resumed["actions"]["A:create"]["status"], "pending")
        self.assertEqual(resumed["automation"]["status"], "PAUSED")

    def test_adopted_workers_can_finish_but_cannot_skip_review_order(self):
        config = copy.deepcopy(self.config)
        config["tasks"][1].update(adopted=True, phase="running", session={
            "thread_id": "already-running", "model": "historical-model", "thinking": "high"})
        path = self.root / "adopted.json"
        adopted = journal.initialize(path, config)
        journal.reserve(adopted, "B:self-review", "send_prompt", "B",
                        {"model": journal.MODEL, "thinking": journal.THINKING, "prompt": "self-review"})
        journal.validate(adopted)
        reviewed = self.ready()["tasks"][0]
        changes = {key: copy.deepcopy(reviewed[key]) for key in journal.TASK_FIELDS if key in reviewed}
        with self.assertRaises(journal.StateError):
            journal.transact(path, 0, lambda state: journal.apply_patch(state, {"tasks": {"B": changes}}))

    def test_added_authorization_and_generated_plan_keep_history_and_scope(self):
        updated_auth = copy.deepcopy(self.config["authorization"])
        updated_auth.update(source="new human instruction", comment_prs=False)
        self.transact(lambda state: journal.apply_patch(state, {"run": {"authorization": updated_auth}}))
        self.assertEqual(self.load()["authorization_history"][0]["authorization"]["source"], "user message")
        with self.assertRaises(journal.StateError):
            self.transact(lambda state: journal.apply_patch(state, {"run": {"default_branch": "other"}}))
        config = copy.deepcopy(self.config)
        config["tasks"][0]["plan"] = None
        path = self.root / "generated-plan.json"
        journal.initialize(path, config)
        journal.transact(path, 0, lambda state: journal.apply_patch(state, {"tasks": {"A": {"plan": "generated.plan.md"}}}))
        with self.assertRaises(journal.StateError):
            journal.transact(path, 1, lambda state: journal.apply_patch(state, {"tasks": {"A": {"plan": "replacement.plan.md"}}}))

    def test_complete_then_pause_without_restarting_tasks(self):
        self.ready()
        self.merged()
        self.ready("B")
        self.merged("B")
        self.transact(lambda state: journal.apply_patch(state, {"run": {
            "phase": "complete", "automation": {"id": "heartbeat", "status": "ACTIVE"}}}))
        self.reserve("stop", "pause_heartbeat", task_id=None, request={"id": "heartbeat"})
        with self.assertRaises(journal.StateError):
            self.transact(lambda state: journal.settle(state, "stop", "confirmed", {"ok": True}))
        self.transact(lambda state: journal.apply_patch(state, {"run": {
            "automation": {"id": "heartbeat", "status": "PAUSED"}}}))
        self.transact(lambda state: journal.settle(state, "stop", "confirmed", {"status": "PAUSED"}))
        with self.assertRaises(journal.StateError):
            self.reserve("B:new-session", task_id="B")

    def test_write_rejects_repo_location_and_stale_revision(self):
        before = self.path.read_bytes()
        with self.assertRaises(journal.StateError):
            journal.transact(self.path, 100, lambda state: journal.apply_patch(state, {"run": {"phase": "paused"}}))
        self.assertEqual(self.path.read_bytes(), before)
        with self.assertRaises(journal.StateError):
            journal.initialize(self.root / "repo" / "state.json", self.config)
        with self.assertRaises(journal.StateError):
            journal.initialize(self.path, self.config)

    def test_cli_roundtrip_preserves_unicode(self):
        script = Path(journal.__file__)
        config = copy.deepcopy(self.config)
        config["authorization"]["source"] = "用户授权"
        input_path = self.root / "input.json"
        input_path.write_text(json.dumps(config, ensure_ascii=False), encoding="utf-8")
        path = self.root / "cli.json"
        command = [sys.executable, str(script), "init", "--state", str(path), "--input", str(input_path)]
        initialized = subprocess.run(command, capture_output=True, text=True, check=True)
        self.assertEqual(json.loads(initialized.stdout)["authorization"]["source"], "用户授权")
        shown = subprocess.run([sys.executable, str(script), "show", "--state", str(path)],
                               capture_output=True, text=True, check=True)
        self.assertEqual(json.loads(initialized.stdout), json.loads(shown.stdout))


if __name__ == "__main__":
    unittest.main()
