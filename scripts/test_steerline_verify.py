"""Check that the native verify receipt keeps the fixed gates and revision."""

import contextlib
import importlib.util
import io
import json
import pathlib
import sys
import unittest
from unittest import mock


SCRIPT = pathlib.Path(__file__).with_name("steerline-verify.py")


class VerifyReceiptTests(unittest.TestCase):
    def test_full_test_result_controls_receipt_without_changing_other_gates(self):
        spec = importlib.util.spec_from_file_location("steerline_verify", SCRIPT)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        context = {
            "schema_version": "native-task/v1", "current_step": "verify",
            "branch": "candidate", "project_id": "project", "base_commit": "base",
            "task_id": "1", "workflow_run_id": "workflow", "execution_id": "execution",
            "attempt_id": "attempt", "step_input": {
                "candidate_revision": "candidate-revision", "input_revision": "input-revision",
                "project_steps": {"verify": {"required_checks": ["go_test", "go_vet", "go_build"]}},
            },
        }

        def git(*args):
            return {("branch", "--show-current"): "candidate", ("rev-parse", "HEAD"): "candidate-revision", ("status", "--porcelain"): ""}[args]

        for test_exit in (0, 1):
            with self.subTest(test_exit=test_exit):
                commands = []

                def run(command, **kwargs):
                    commands.append(command)
                    return type("Result", (), {"returncode": test_exit if len(commands) == 1 else 0, "stdout": ""})()

                stdout = io.StringIO()
                with mock.patch.object(module, "git", side_effect=git), mock.patch.object(module.subprocess, "run", side_effect=run), mock.patch.object(sys, "stdin", io.StringIO(json.dumps(context))), contextlib.redirect_stdout(stdout):
                    module.main()
                receipt = json.loads(stdout.getvalue())
                result = receipt["result"]
                self.assertEqual(result["succeeded"], test_exit == 0)
                self.assertEqual([check["name"] for check in result["checks"]], ["go_test", "go_vet", "go_build"])
                self.assertEqual([check["status"] for check in result["checks"]], ["passed" if test_exit == 0 else "failed", "passed", "passed"])
                self.assertTrue(all(check["revision"] == "candidate-revision" for check in result["checks"]))
                self.assertEqual(commands[0][:4], ["go", "run", "./cmd/test-feedback", "-workers=3"])
                self.assertEqual(commands[1], ["go", "vet", "./..."])
                self.assertEqual(commands[2][:3], ["go", "build", "-o"])
                self.assertTrue(pathlib.Path(commands[0][-1]).is_absolute())


if __name__ == "__main__":
    unittest.main()
